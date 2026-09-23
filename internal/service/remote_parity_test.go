package service

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/config"
	"dingoscheduler/pkg/consts"
	"github.com/glebarez/sqlite"
	"github.com/patrickmn/go-cache"
	"gorm.io/gorm"
)

func TestRemoteCreationDelegatesCommitReuseAndSelectsProviderCredentials(t *testing.T) {
	previous := config.SysConfig
	config.SysConfig = &config.Config{Scheduler: config.Scheduler{ModelScopeToken: "ms-test"}}
	t.Cleanup(func() { config.SysConfig = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.CacheJob{}, &model.Dingospeed{}, &model.HfToken{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.HfToken{Token: "hf-test", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	count := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		var req query.CreateCacheJobReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		want := "Bearer hf-test"
		if req.Org == "modelscope/owner" {
			want = "Bearer ms-test"
		}
		if r.URL.Path != "/api/cacheJob/create" || r.Header.Get("Authorization") != want {
			t.Errorf("wrong provider request: %s %s", r.URL, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":42,"disposition":"cached","commit":"pinned"}`))
	}))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	p, _ := strconv.Atoi(port)
	if err := db.Create(&model.Dingospeed{InstanceID: "node", Host: host, Port: int32(p), Online: true}).Error; err != nil {
		t.Fatal(err)
	}
	d := &data.BaseData{BizDB: db, Cache: cache.New(time.Minute, time.Minute)}
	jobs := dao.NewCacheJobDao(d, nil)
	s := NewCacheJobService(dao.NewDingospeedDao(d), nil, jobs, dao.NewHfTokenDao(d), dao.NewLockDao(d))
	for _, org := range []string{"owner", "modelscope/owner"} {
		if err := jobs.Save(&model.CacheJob{Type: consts.CacheTypePreheat, InstanceId: "node", Datatype: "models", Org: org, Repo: "demo", Commit: "previous", Status: consts.RunningStatusJobComplete}); err != nil {
			t.Fatal(err)
		}
		resp, err := s.CreateCacheJob(&query.CreateCacheJobReq{Type: consts.CacheTypePreheat, InstanceId: "node", Datatype: "models", Org: org, Repo: "demo"})
		if err != nil || resp.StatusCode != 200 || !strings.Contains(string(resp.Body), `"disposition":"cached"`) {
			t.Fatalf("prior job blocked reuse: %+v %v", resp, err)
		}
	}
	if count != 2 {
		t.Fatalf("Speed admission called %d times", count)
	}
	items, total, err := s.ListCacheJob("node", "models", 1, 10, "modelscope")
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list: %v %d %v", items, total, err)
	}
	if items[0].Namespace != "modelscope" || items[0].FullRepo != "owner/demo" || items[0].RepositoryID != "modelscope/owner/demo" || items[0].Org != "modelscope/owner" {
		t.Fatalf("identity contract: %+v", items[0])
	}
}

func TestRepositoryIdentityAddsCanonicalFieldsWithoutChangingLegacyHF(t *testing.T) {
	for _, tc := range []struct{ org, namespace, full string }{{"owner", "huggingface", "owner/demo"}, {"modelscope/owner", "modelscope", "owner/demo"}, {"dingo-local/alice", "alice", "demo"}} {
		r := &dto.Repository{Datatype: "models", Org: tc.org, Repo: "demo"}
		setRepositoryIdentity(r)
		if r.Namespace != tc.namespace || r.FullRepo != tc.full || r.RepositoryID != tc.namespace+"/"+tc.full {
			t.Fatalf("identity: %+v", r)
		}
		if tc.namespace == "huggingface" && (r.Org != "owner" || r.Repo != "demo") {
			t.Fatal("changed legacy HF fields")
		}
	}
}
