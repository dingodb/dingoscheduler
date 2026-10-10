package dao

import (
	"context"
	"strings"
	"testing"
	"time"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
)

func TestSyncSourcesFollowFreshRegistrationInsteadOfLargestID(t *testing.T) {
	r, db := inventoryTestDAO(t)
	if err := db.AutoMigrate(&model.NodeEndpoint{}); err != nil {
		t.Fatal(err)
	}
	q := SyncSourceQuery{Key: repository.Key{Namespace: "team", RepoType: "models", Repo: "demo"}, Path: "a.bin", SHA256: strings.Repeat("a", 64), Size: 7}
	f := model.UploadInventoryFile{IdentityHash: uploadIdentity(&UploadedInventoryItem{Namespace: q.Namespace, RepoType: q.RepoType, Repo: q.Repo, Path: q.Path, SHA256: q.SHA256}), Namespace: q.Namespace, RepoType: q.RepoType, Repo: q.Repo, Path: q.Path, SHA256: q.SHA256, Size: q.Size}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, node := range []model.Dingospeed{
		{ID: 1, InstanceID: "source", Online: false, UpdatedAt: now.Add(-time.Minute)},
		{ID: 2, InstanceID: "source", Online: true, UpdatedAt: now.Add(-10 * time.Minute)},
	} {
		if err := db.Create(&node).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range []model.NodeEndpoint{
		{NodeID: 1, ManagementURL: "http://current:8091"},
		{NodeID: 2, ManagementURL: "http://other-mode:8091"},
	} {
		if err := db.Create(&endpoint).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.UploadInventoryHolding{FileID: f.ID, InstanceID: "source", ConfirmedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		rows, err := r.UploadedSyncSources(context.Background(), q)
		if err != nil || len(rows) != 1 || rows[0].InstanceID != "source" || rows[0].ManagementURL != want {
			t.Fatalf("source discovery lost current registration: rows=%+v err=%v want=%s", rows, err, want)
		}
	}
	check("http://current:8091")
	// Both modes can be fresh; a larger ID still does not imply a newer heartbeat.
	if err := db.Model(&model.Dingospeed{}).Where("id = ?", 2).Update("updated_at", now.Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	check("http://current:8091")
	if err := db.Model(&model.Dingospeed{}).Where("id = ?", 2).Update("updated_at", now.Add(-30*time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	check("http://other-mode:8091")
}
