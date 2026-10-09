package dao

import (
	"context"
	"strings"
	"testing"
	"time"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
)

func TestSyncSourcesExactIdentityAndRegisteredReachableEndpoints(t *testing.T) {
	r, db := inventoryTestDAO(t)
	if err := db.AutoMigrate(&model.NodeEndpoint{}); err != nil {
		t.Fatal(err)
	}
	q := SyncSourceQuery{Key: repository.Key{Namespace: "team", RepoType: "models", Repo: "demo/full"}, Path: "weights.bin", SHA256: strings.Repeat("a", 64), Size: 10, ExcludeInstanceID: "target"}
	f := model.UploadInventoryFile{IdentityHash: uploadIdentity(&UploadedInventoryItem{Namespace: q.Namespace, RepoType: q.RepoType, Repo: q.Repo, Path: q.Path, SHA256: q.SHA256}), Namespace: q.Namespace, RepoType: q.RepoType, Repo: q.Repo, Path: q.Path, SHA256: q.SHA256, Size: q.Size}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i, name := range []string{"good", "target", "stale", "bad-url", "missing-endpoint", "reregistered"} {
		node := model.Dingospeed{ID: int32(i + 1), InstanceID: name, Host: "127.0.0.1", Port: 1, UpdatedAt: now, Online: false}
		if name == "stale" {
			node.UpdatedAt = now.Add(-10 * time.Minute)
		}
		if err := db.Create(&node).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.UploadInventoryHolding{FileID: f.ID, InstanceID: name, ConfirmedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		endpoint := "http://127.0.0.1:19000"
		if name == "bad-url" {
			endpoint = "http://user:password@127.0.0.1:19000"
		}
		if name != "missing-endpoint" {
			if err := db.Create(&model.NodeEndpoint{NodeID: node.ID, ManagementURL: endpoint}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// The most recent registration wins; a stale new registration must not be
	// replaced by an old healthy endpoint for the same instance.
	if err := db.Create(&model.Dingospeed{ID: 99, InstanceID: "reregistered", Host: "127.0.0.1", Port: 1, UpdatedAt: now.Add(-10 * time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.NodeEndpoint{NodeID: 99, ManagementURL: "http://127.0.0.1:19999"}).Error; err != nil {
		t.Fatal(err)
	}
	rows, err := r.UploadedSyncSources(context.Background(), q)
	if err != nil || len(rows) != 1 || rows[0].InstanceID != "good" {
		t.Fatalf("sources=%+v err=%v", rows, err)
	}
	for _, change := range []func(*SyncSourceQuery){func(q *SyncSourceQuery) { q.Size++ }, func(q *SyncSourceQuery) { q.Path = "other.bin" }, func(q *SyncSourceQuery) { q.Repo = "other" }, func(q *SyncSourceQuery) { q.Namespace = "another" }, func(q *SyncSourceQuery) { q.SHA256 = strings.Repeat("b", 64) }} {
		other := q
		change(&other)
		rows, err := r.UploadedSyncSources(context.Background(), other)
		if err != nil || len(rows) != 0 {
			t.Fatalf("identity leaked: %+v rows=%+v err=%v", other, rows, err)
		}
	}
}

func TestSyncSourceQueryRejectsRemoteOrIncompleteIdentity(t *testing.T) {
	base := SyncSourceQuery{Key: repository.Key{Namespace: "team", RepoType: "models", Repo: "demo"}, Path: "empty.bin", SHA256: strings.Repeat("a", 64), Size: 0}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*SyncSourceQuery){func(q *SyncSourceQuery) { q.Namespace = "huggingface" }, func(q *SyncSourceQuery) { q.Namespace = "modelscope" }, func(q *SyncSourceQuery) { q.Path = "../a" }, func(q *SyncSourceQuery) { q.Size = -1 }, func(q *SyncSourceQuery) { q.SHA256 = strings.Repeat("A", 64) }, func(q *SyncSourceQuery) { q.Repo = "" }} {
		q := base
		change(&q)
		if q.Validate() == nil {
			t.Fatalf("accepted %+v", q)
		}
	}
}
