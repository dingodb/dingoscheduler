package dao

import (
	"context"
	"testing"
	"time"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func inventoryTestDAO(t *testing.T) (*RepositoryDao, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.UploadInventoryState{}, &model.UploadInventoryFile{}, &model.UploadInventoryHolding{}, &model.Dingospeed{}, &model.ModelFileRecord{}, &model.ModelFileProcess{}, &model.Repository{}); err != nil {
		t.Fatal(err)
	}
	return &RepositoryDao{baseData: &data.BaseData{BizDB: db}}, db
}

func inventory(epoch string, started time.Time, sequence uint64, instance string, complete bool, items ...UploadedInventoryItem) *UploadedInventorySnapshot {
	return &UploadedInventorySnapshot{Version: 1, InstanceID: instance, Epoch: epoch, EpochStartedAt: started, Sequence: sequence, GeneratedAt: started.Add(time.Duration(sequence) * time.Second), Complete: complete, Items: items}
}

func item(path, hash string, size int64) UploadedInventoryItem {
	return UploadedInventoryItem{Namespace: "team", RepoType: "models", Repo: "demo/full", Path: path, SHA256: hash, Size: size}
}

func TestUploadedInventoryMultiNodeIdentityOrderingAndCompleteness(t *testing.T) {
	dao, db := inventoryTestDAO(t)
	ctx := context.Background()
	started := time.Now().Add(-time.Hour).UTC()
	h1 := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	h2 := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	base := item("same/path.bin", h1, 10)
	if accepted, _, _, err := dao.ApplyUploadedInventory(ctx, inventory("e1", started, 1, "A", true, base, item("other/path.bin", h1, 10), item("same/path.bin", h2, 11))); err != nil || !accepted {
		t.Fatalf("A apply: accepted=%v err=%v", accepted, err)
	}
	if accepted, _, _, err := dao.ApplyUploadedInventory(ctx, inventory("e2", started, 1, "B", true, base)); err != nil || !accepted {
		t.Fatalf("B apply: accepted=%v err=%v", accepted, err)
	}
	var holdings, files int64
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	db.Model(&model.UploadInventoryFile{}).Count(&files)
	if holdings != 4 || files != 3 {
		t.Fatalf("holdings=%d files=%d, want 4/3", holdings, files)
	}

	// A's complete empty inventory removes only A; B keeps the shared file.
	if _, _, _, err := dao.ApplyUploadedInventory(ctx, inventory("e1", started, 2, "A", true)); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	db.Model(&model.UploadInventoryFile{}).Count(&files)
	if holdings != 1 || files != 1 {
		t.Fatalf("after A delete holdings=%d files=%d, want 1/1", holdings, files)
	}

	// Late pre-delete inventory cannot revive A.
	accepted, _, _, err := dao.ApplyUploadedInventory(ctx, inventory("e1", started, 1, "A", true, base))
	if err != nil || accepted {
		t.Fatalf("late snapshot accepted=%v err=%v", accepted, err)
	}
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	if holdings != 1 {
		t.Fatalf("late snapshot revived holding: %d", holdings)
	}

	// Incomplete higher sequence marks uncertainty but never interprets absence.
	if _, _, _, err = dao.ApplyUploadedInventory(ctx, inventory("e2", started, 2, "B", false)); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	if holdings != 1 {
		t.Fatalf("incomplete scan removed holding: %d", holdings)
	}
	var state model.UploadInventoryState
	if err = db.Where("instance_id = ?", "B").Take(&state).Error; err != nil || state.InventoryComplete {
		t.Fatalf("state=%+v err=%v", state, err)
	}
	if state.LastConfirmedAt == nil {
		t.Fatal("incomplete scan erased the last complete confirmation time")
	}

	// A genuinely newer epoch may restore the file; an older epoch cannot.
	accepted, _, _, err = dao.ApplyUploadedInventory(ctx, inventory("old", started.Add(-time.Minute), 99, "A", true, base))
	if err != nil || accepted {
		t.Fatalf("older epoch accepted=%v err=%v", accepted, err)
	}
	accepted, _, _, err = dao.ApplyUploadedInventory(ctx, inventory("new", started.Add(time.Minute), 1, "A", true, base))
	if err != nil || !accepted {
		t.Fatalf("new epoch accepted=%v err=%v", accepted, err)
	}
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	if holdings != 2 {
		t.Fatalf("restore holdings=%d, want 2", holdings)
	}
	if err = db.Create(&model.Dingospeed{InstanceID: "A", Host: "127.0.0.1", Port: 1, Online: true, UpdatedAt: time.Now().Add(-10 * time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	views, err := dao.ListUploadedHoldings(ctx, "A", "", "", "", "", "")
	if err != nil || len(views) != 1 || views[0].NodeAvailable {
		t.Fatalf("offline view=%+v err=%v", views, err)
	}
	nodeState, err := dao.GetUploadedNodeInventoryState(ctx, "A")
	if err != nil || nodeState.NodeAvailable || !nodeState.InventoryComplete || nodeState.LastConfirmedAt == nil {
		t.Fatalf("node state=%+v err=%v", nodeState, err)
	}
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	if holdings != 2 {
		t.Fatalf("offline observation deleted holdings: %d", holdings)
	}

	// Removing both confirmed copies finally removes the global uploaded-file row.
	if _, _, _, err = dao.ApplyUploadedInventory(ctx, inventory("new", started.Add(time.Minute), 2, "A", true)); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = dao.ApplyUploadedInventory(ctx, inventory("e2", started, 3, "B", true)); err != nil {
		t.Fatal(err)
	}
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	db.Model(&model.UploadInventoryFile{}).Count(&files)
	if holdings != 0 || files != 0 {
		t.Fatalf("last confirmed removal left holdings=%d files=%d", holdings, files)
	}
}

func TestUploadedInventoryDoesNotTouchRemoteTables(t *testing.T) {
	dao, db := inventoryTestDAO(t)
	legacyRecord := model.ModelFileRecord{Datatype: "models", Org: "remote-org", Repo: "remote-repo", Name: "x", Etag: "etag", FileSize: 7}
	if err := db.Create(&legacyRecord).Error; err != nil {
		t.Fatal(err)
	}
	legacyProcess := model.ModelFileProcess{RecordID: legacyRecord.ID, InstanceID: "A", OffsetNum: 7, Status: 3}
	if err := db.Create(&legacyProcess).Error; err != nil {
		t.Fatal(err)
	}
	legacyRepo := model.Repository{InstanceId: "A", Datatype: "models", Org: "remote-org", Repo: "remote-repo", OrgRepo: "remote-org/remote-repo", Sha: "remote"}
	if err := db.Create(&legacyRepo).Error; err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	if _, _, _, err := dao.ApplyUploadedInventory(context.Background(), inventory("e", started, 1, "A", true, item("x", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 7))); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := dao.ApplyUploadedInventory(context.Background(), inventory("e", started, 2, "A", true)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		model interface{}
	}{
		{"record", &model.ModelFileRecord{}}, {"process", &model.ModelFileProcess{}}, {"repository", &model.Repository{}},
	} {
		var count int64
		if err := db.Model(tc.model).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("legacy %s count=%d err=%v", tc.name, count, err)
		}
	}
}
