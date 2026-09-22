package authority

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAtomicConfirmedDefinitions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&Definition{}, &Receipt{}, &model.Dingospeed{}, &model.UploadInventoryFile{}, &model.UploadInventoryHolding{}); err != nil {
		t.Fatal(err)
	}
	key := repository.Key{Namespace: "datacanvas", RepoType: "models", Repo: "demo"}
	a := File{Path: "folder/a", SHA256: strings.Repeat("a", 64), Size: 1}
	b := File{Path: "folder/b", SHA256: strings.Repeat("b", 64), Size: 2}
	snap := Snapshot{Key: key, Revision: "commit", Commit: "commit", Files: []File{a, b}, Complete: true, Verified: true, ContentVerified: true, FileCount: 2}
	broken := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(snap)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	n := model.Dingospeed{InstanceID: "A", Host: u.Hostname(), Port: int32(port), UpdatedAt: time.Now()}
	db.Create(&n)
	s := New(db)
	ctx := context.Background()
	source := &Source{Node: "A", Revision: "main", Commit: "commit"}
	r := Request{Key: key, Action: "commit", Revision: "release", Actor: "admin", RequestID: "create", Changes: []Change{{Path: a.Path, File: &a, Source: source}, {Path: b.Path, File: &b, Source: source}}}
	if _, err = s.Do(ctx, r); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Do(ctx, r); err != nil {
		t.Fatal("idempotent retry", err)
	}
	get := r
	get.Action = "get"
	v, err := s.get(ctx, get)
	if err != nil || v.Version != 1 || len(v.Files) != 2 {
		t.Fatalf("%+v %v", v, err)
	}
	r.RequestID = "stale"
	if _, err = s.Do(ctx, r); err == nil {
		t.Fatal("stale revision overwrote")
	}
	emptyRequest := r
	emptyRequest.Revision = "empty-revision"
	emptyRequest.BaseVersion = 0
	emptyRequest.RequestID = "create-empty"
	emptyRequest.Changes = nil
	if _, err = s.Do(ctx, emptyRequest); err != nil {
		t.Fatal("create empty revision", err)
	}
	emptyView, err := s.get(ctx, emptyRequest)
	if err != nil || emptyView.Version != 1 || len(emptyView.Files) != 0 {
		t.Fatalf("empty revision readback: %+v %v", emptyView, err)
	}
	deleteRequest := r
	deleteRequest.Revision = "delete-all"
	deleteRequest.RequestID = "create-before-delete"
	if _, err = s.Do(ctx, deleteRequest); err != nil {
		t.Fatal(err)
	}
	deleteRequest.BaseVersion = 1
	deleteRequest.RequestID = "delete-all"
	deleteRequest.Changes = []Change{{Path: a.Path}, {Path: b.Path}}
	if _, err = s.Do(ctx, deleteRequest); err != nil {
		t.Fatal("delete all files", err)
	}
	emptyView, err = s.get(ctx, deleteRequest)
	if err != nil || emptyView.Version != 2 || len(emptyView.Files) != 0 {
		t.Fatalf("delete all readback: %+v %v", emptyView, err)
	}
	r.BaseVersion = 1
	c := File{Path: "c", SHA256: strings.Repeat("c", 64), Size: 3}
	r.RequestID = "bad-source"
	r.Changes = []Change{{Path: a.Path}, {Path: c.Path, File: &c, Source: source}}
	if _, err = s.Do(ctx, r); err == nil {
		t.Fatal("unverified content accepted")
	}
	v, _ = s.get(ctx, get)
	if v.Version != 1 || len(v.Files) != 2 {
		t.Fatal("partial commit")
	}
	snap.Complete = false
	if _, err = s.snapshot(ctx, key, "A", "commit"); err == nil {
		t.Fatal("incomplete accepted")
	}
	snap.Complete = true
	snap.ContentVerified = false
	if _, err = s.snapshot(ctx, key, "A", "commit"); err == nil {
		t.Fatal("lightweight completion check accepted as SHA256 verification")
	}
	snap.ContentVerified = true
	snap.FileCount = 3
	if _, err = s.snapshot(ctx, key, "A", "commit"); err == nil {
		t.Fatal("truncated accepted")
	}
	snap.FileCount = 2
	broken = true
	r.RequestID = "unreadable"
	r.Changes = []Change{{Path: a.Path, File: &a, Source: source}}
	if _, err = s.Do(ctx, r); err == nil {
		t.Fatal("unreadable accepted")
	}
	// A retained official file needs no source. Deletion changes only definition.
	r.RequestID = "delete-one"
	r.Changes = []Change{{Path: a.Path}}
	if _, err = s.Do(ctx, r); err != nil {
		t.Fatal(err)
	}
	v, _ = s.get(ctx, get)
	if v.Version != 2 || len(v.Files) != 1 || v.Files[0] != b {
		t.Fatal("retained unavailable file lost")
	}
	// A successful receipt survives a new service instance and source outage.
	if _, err = New(db).Do(ctx, r); err != nil {
		t.Fatal("restart retry", err)
	}
	r.Changes = []Change{{Path: b.Path}}
	if _, err = s.Do(ctx, r); err == nil {
		t.Fatal("idempotency identity reused")
	}
	db.Model(&n).Update("updated_at", time.Now().Add(-time.Hour))
	if _, err = s.snapshot(ctx, key, "A", "commit"); err == nil {
		t.Fatal("offline accepted")
	}
}
