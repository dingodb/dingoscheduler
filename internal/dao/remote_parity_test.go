package dao

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"
	"dingoscheduler/pkg/repository"
	"github.com/glebarez/sqlite"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
)

func remoteTestDAO(t *testing.T, handler http.HandlerFunc) (*RepositoryDao, *CacheJobDao, *gorm.DB) {
	t.Helper()
	previous := config.SysConfig
	config.SysConfig = &config.Config{Retry: config.Retry{Attempts: 1}, Scheduler: config.Scheduler{ModelScopeToken: "ms-test"}}
	t.Cleanup(func() { config.SysConfig = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.Repository{}, &model.RepositoryTag{}, &model.Tag{}, &model.CacheJob{}, &model.Dingospeed{}, &model.ModelFileRecord{}, &model.ModelFileProcess{}, &model.HfToken{}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	p, _ := strconv.Atoi(port)
	if err := db.Create(&model.Dingospeed{InstanceID: "node", Host: host, Port: int32(p), Online: true}).Error; err != nil {
		t.Fatal(err)
	}
	d := &data.BaseData{BizDB: db, Cache: cache.New(time.Minute, time.Minute)}
	r := NewRepositoryDao(d, NewRepositoryTagDao(d), NewTagDao(d), NewDingospeedDao(d), NewOrganizationDao(d), NewHfTokenDao(d))
	return r, NewCacheJobDao(d, r), db
}

func putRemoteFile(t *testing.T, db *gorm.DB, org, path, etag string, size int64) {
	t.Helper()
	r := &model.ModelFileRecord{Datatype: "models", Org: org, Repo: "demo", Name: path, Etag: etag, FileSize: size}
	if err := db.Create(r).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ModelFileProcess{RecordID: r.ID, InstanceID: "node", OffsetNum: size, Status: 3}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestModelScopeCompletionRetriesPinnedSnapshotAndUpdatesProjection(t *testing.T) {
	fail, mismatch := true, false
	requests := 0
	_, jobs, db := remoteTestDAO(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/repositories/models/modelscope/metadata" || r.URL.Query().Get("repo") != "owner/demo" || r.Header.Get("Authorization") != "Bearer ms-test" {
			t.Errorf("unexpected request %s auth=%q", r.URL, r.Header.Get("Authorization"))
		}
		commit := r.URL.Query().Get("revision")
		if commit != "snapshot-1" && commit != "snapshot-2" {
			t.Errorf("queried mutable revision %s", commit)
		}
		if fail {
			http.Error(w, "unavailable", 503)
			return
		}
		if mismatch {
			commit = "different-head"
		}
		fmt.Fprintf(w, `{"sha":%q,"siblings":[{"rfilename":"weights.bin","blobId":%q,"size":8}],"usedStorage":8}`, commit, commit)
	})
	putRemoteFile(t, db, "modelscope/owner", "weights.bin", "snapshot-1", 8)
	job := &model.CacheJob{Type: consts.CacheTypePreheat, InstanceId: "node", Datatype: "models", Org: "modelscope/owner", Repo: "demo", Commit: "snapshot-1", Status: 1}
	if err := jobs.Save(job); err != nil {
		t.Fatal(err)
	}
	complete := func(id int64) error {
		return jobs.UpdateStatusAndRepo(&query.UpdateJobStatusReq{Id: id, InstanceId: "node", Status: consts.RunningStatusJobComplete, Process: 100})
	}
	if err := complete(job.ID); err == nil {
		t.Fatal("registration failure was hidden")
	}
	stored, _ := jobs.GetCacheJob(&query.CacheJobQuery{Id: job.ID})
	if stored.Status != 3 || !strings.Contains(stored.ErrorMsg, "registration pending") {
		t.Fatalf("lost completion or error: %+v", stored)
	}
	fail = false
	if err := complete(job.ID); err != nil {
		t.Fatal(err)
	}
	if err := complete(job.ID); err != nil {
		t.Fatal(err)
	}
	var repos []model.Repository
	db.Find(&repos)
	if len(repos) != 1 || repos[0].Sha != "snapshot-1" {
		t.Fatalf("retry not idempotent: %+v", repos)
	}
	firstID := repos[0].ID
	job2 := &model.CacheJob{Type: consts.CacheTypePreheat, InstanceId: "node", Datatype: "models", Org: "modelscope/owner", Repo: "demo", Commit: "snapshot-2", Status: 1}
	if err := jobs.Save(job2); err != nil {
		t.Fatal(err)
	}
	// An old complete file at the same path cannot satisfy the new snapshot.
	if err := complete(job2.ID); err == nil {
		t.Fatal("wrong content accepted")
	}
	putRemoteFile(t, db, "modelscope/owner", "weights.bin", "snapshot-2", 8)
	mismatch = true
	if err := complete(job2.ID); err == nil {
		t.Fatal("upstream commit drift accepted")
	}
	mismatch = false
	if err := complete(job2.ID); err != nil {
		t.Fatal(err)
	}
	before := requests
	if err := complete(job.ID); err != nil {
		t.Fatal(err)
	}
	if requests != before {
		t.Fatal("stale completion fetched an old snapshot")
	}
	repos = nil
	db.Find(&repos)
	if len(repos) != 1 || repos[0].ID != firstID || repos[0].Sha != "snapshot-2" {
		t.Fatalf("projection replaced or regressed: %+v", repos)
	}
	stored, _ = jobs.GetCacheJob(&query.CacheJobQuery{Id: job2.ID})
	if stored.ErrorMsg != "" {
		t.Fatal("successful retry retained registration error")
	}
}

func TestModelScopeDiscoveryUsesMasterAndExactFiles(t *testing.T) {
	repo, _, db := remoteTestDAO(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("revision") != "master" {
			t.Errorf("expected master: %s", r.URL)
		}
		fmt.Fprint(w, `{"sha":"snapshot","siblings":[{"rfilename":"weights.bin","blobId":"content","size":8}]}`)
	})
	putRemoteFile(t, db, "modelscope/owner", "other.bin", "content", 8)
	if err := repo.PersistRepo(&query.PersistRepoReq{InstanceIds: []string{"node"}}); err == nil {
		t.Fatal("same count with a different path accepted")
	}
	putRemoteFile(t, db, "modelscope/owner", "weights.bin", "content", 8)
	if err := repo.PersistRepo(&query.PersistRepoReq{InstanceIds: []string{"node"}}); err != nil {
		t.Fatal(err)
	}
	var result model.Repository
	if err := db.First(&result).Error; err != nil {
		t.Fatal(err)
	}
	if result.Sha != "snapshot" || result.UsedStorage != 8 {
		t.Fatalf("wrong projection: %+v", result)
	}
}

func TestProviderCredentialsAndNamespacePagination(t *testing.T) {
	repos, jobs, db := remoteTestDAO(t, func(http.ResponseWriter, *http.Request) {})
	if err := db.Create(&model.HfToken{Token: "hf-test", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ ns, want string }{{"huggingface", "Bearer hf-test"}, {"modelscope", "Bearer ms-test"}, {"alice", ""}} {
		if got := repos.hfTokenDao.ProviderHeaders(repository.Key{Namespace: tc.ns})["Authorization"]; got != tc.want {
			t.Fatalf("%s credential = %q", tc.ns, got)
		}
	}
	config.SysConfig.Scheduler.ModelScopeToken = ""
	if len(repos.hfTokenDao.ProviderHeaders(repository.Key{Namespace: "modelscope"})) != 0 {
		t.Fatal("fell back to HF credential")
	}
	for _, org := range []string{"owner", "modelscope/owner", "dingo-local/alice"} {
		if err := db.Create(&model.Repository{InstanceId: "node", Datatype: "models", Org: org, Repo: "demo"}).Error; err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := jobs.Save(&model.CacheJob{InstanceId: "node", Datatype: "models", Org: org, Repo: "demo"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, ns := range []string{"huggingface", "modelscope", "alice"} {
		r, total, err := repos.ModelList(&query.ModelQuery{Namespace: ns})
		if err != nil || total != 1 || len(r) != 1 || repositoryKey(r[0]).Namespace != ns {
			t.Fatalf("repository filter %s: %v %d %v", ns, r, total, err)
		}
		first, count, err := jobs.ListCacheJob(&query.CacheJobQuery{Namespace: ns, Page: 1, PageSize: 1})
		if err != nil || count != 2 || len(first) != 1 {
			t.Fatalf("job filter: %v %d %v", first, count, err)
		}
		second, _, err := jobs.ListCacheJob(&query.CacheJobQuery{Namespace: ns, Page: 2, PageSize: 1})
		if err != nil || len(second) != 1 || second[0].ID == first[0].ID {
			t.Fatal("pagination repeated first result")
		}
	}
}

func TestPinnedTreeResolvesHFFileIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Query().Get("recursive") != "true" || r.URL.Query().Get("revision") != "pinned" || r.Header.Get("Authorization") != "Bearer hf" {
			t.Errorf("unexpected tree: %s %s", r.Method, r.URL)
		}
		fmt.Fprint(w, `[{"type":"file","path":"weights.bin","oid":"git-pointer","size":120,"lfs":{"oid":"content","size":8}}]`)
	}))
	defer srv.Close()
	var meta dto.CommitHfSha
	json.Unmarshal([]byte(`{"sha":"pinned","siblings":[{"rfilename":"weights.bin"}]}`), &meta)
	if err := (&RepositoryDao{}).resolveFileIdentities(&meta, srv.URL, repository.Key{Namespace: "huggingface", RepoType: "models", Repo: "owner/demo"}, map[string]string{"Authorization": "Bearer hf"}); err != nil {
		t.Fatal(err)
	}
	if meta.Siblings[0].BlobID != "content" || meta.UsedStorage != 8 {
		t.Fatalf("unresolved metadata: %+v", meta)
	}
}
