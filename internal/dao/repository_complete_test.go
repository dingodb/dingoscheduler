package dao

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"dingoscheduler/internal/model"
	"dingoscheduler/internal/model/dto"

	"gorm.io/gorm"
)

// 2026-10-14: CASIA-LM/ChineseWebText2.0 contains an empty .gitkeep. Speed has
// no record for it, so the fully cached dataset was never registered.
func TestVerifyRepoCompleteSkipsEmptyFiles(t *testing.T) {
	r, _, db := remoteTestDAO(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	var meta dto.CommitHfSha
	if err := json.Unmarshal([]byte(`{"sha":"c1","siblings":[
		{"rfilename":"a.bin","blobId":"oid-a","size":3},
		{"rfilename":"d/.gitkeep","blobId":"e69de29bb2d1d6434b8b29ae775ad8c2e48c5391","size":0}]}`), &meta); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.verifyRepoComplete(&meta, "node", "models", "org", "demo"); err != nil || ok {
		t.Fatalf("uncached a.bin accepted: %v %v", ok, err)
	}
	putRemoteFile(t, db, "org", "a.bin", "oid-a", 3)
	if ok, err := r.verifyRepoComplete(&meta, "node", "models", "org", "demo"); err != nil || !ok {
		t.Fatalf("empty file blocked registration: %v %v", ok, err)
	}
}

func TestVerifyRepoCompleteUsesOneQuery(t *testing.T) {
	r, _, db := remoteTestDAO(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	const n = 50
	siblings := make([]string, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("part-%03d.bin", i)
		siblings[i] = fmt.Sprintf(`{"rfilename":%q,"blobId":"oid-%d","size":%d}`, name, i, i+1)
		putRemoteFile(t, db, "org", name, fmt.Sprintf("oid-%d", i), int64(i+1))
	}
	var meta dto.CommitHfSha
	if err := json.Unmarshal([]byte(`{"sha":"c1","siblings":[`+strings.Join(siblings, ",")+`]}`), &meta); err != nil {
		t.Fatal(err)
	}
	var queries int
	if err := db.Callback().Query().After("gorm:query").Register("test:count", func(*gorm.DB) { queries++ }); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.verifyRepoComplete(&meta, "node", "models", "org", "demo"); err != nil || !ok {
		t.Fatalf("complete repository rejected: %v %v", ok, err)
	}
	if queries != 1 {
		t.Fatalf("%d queries for %d files", queries, n)
	}
}

func TestVerifyRepoCompleteRejectsMismatchedRecords(t *testing.T) {
	meta := func(t *testing.T) *dto.CommitHfSha {
		var m dto.CommitHfSha
		if err := json.Unmarshal([]byte(`{"sha":"c1","siblings":[{"rfilename":"a.bin","blobId":"oid-a","size":3}]}`), &m); err != nil {
			t.Fatal(err)
		}
		return &m
	}
	for name, put := range map[string]func(*testing.T, *gorm.DB){
		"other etag": func(t *testing.T, db *gorm.DB) { putRemoteFile(t, db, "org", "a.bin", "oid-old", 3) },
		"other size": func(t *testing.T, db *gorm.DB) { putRemoteFile(t, db, "org", "a.bin", "oid-a", 4) },
		"other repo": func(t *testing.T, db *gorm.DB) { putRemoteFile(t, db, "org2", "a.bin", "oid-a", 3) },
		"other instance": func(t *testing.T, db *gorm.DB) {
			rec := &model.ModelFileRecord{Datatype: "models", Org: "org", Repo: "demo", Name: "a.bin", Etag: "oid-a", FileSize: 3}
			db.Create(rec)
			db.Create(&model.ModelFileProcess{RecordID: rec.ID, InstanceID: "elsewhere", OffsetNum: 3, Status: 3})
		},
		"partial": func(t *testing.T, db *gorm.DB) {
			rec := &model.ModelFileRecord{Datatype: "models", Org: "org", Repo: "demo", Name: "a.bin", Etag: "oid-a", FileSize: 3}
			db.Create(rec)
			db.Create(&model.ModelFileProcess{RecordID: rec.ID, InstanceID: "node", OffsetNum: 2, Status: 1})
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, _, db := remoteTestDAO(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
			put(t, db)
			if ok, err := r.verifyRepoComplete(meta(t), "node", "models", "org", "demo"); err != nil || ok {
				t.Fatalf("accepted: %v %v", ok, err)
			}
		})
	}
}
