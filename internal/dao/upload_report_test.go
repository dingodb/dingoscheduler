package dao

import (
	"context"
	"dingoscheduler/internal/model"
	inv "dingoscheduler/pkg/inventory"
	"strings"
	"testing"
)

func TestRepositoryReportsAndManualResetAreFencedPerNode(t *testing.T) {
	dbDAO, db := inventoryTestDAO(t)
	if err := db.AutoMigrate(&model.UploadReportNode{}, &model.UploadReportRepo{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B"} {
		if err := db.Create(&model.Dingospeed{InstanceID: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	key := inv.Key{Namespace: "team", RepoType: "models", Repo: "full/name"}
	other := inv.Key{Namespace: "team", RepoType: "models", Repo: "other"}
	file := inv.File{Key: key, Path: "weights.bin", SHA256: strings.Repeat("a", 64), Size: 5}
	baseline := func(id string) *inv.Report {
		t.Helper()
		n, e := dbDAO.UploadReportSession(ctx, id, true)
		if e != nil {
			t.Fatal(e)
		}
		p := &inv.Report{Version: 2, InstanceID: id, Epoch: n.PendingEpoch, Baseline: true, Files: []inv.File{file}}
		if _, e = dbDAO.ApplyUploadReport(ctx, p); e != nil {
			t.Fatal(e)
		}
		return p
	}
	a := baseline("A")
	b := baseline("B")
	apply := func(p *inv.Report) {
		t.Helper()
		if _, e := dbDAO.ApplyUploadReport(ctx, p); e != nil {
			t.Fatal(e)
		}
	}
	one := inv.Report{Version: 2, InstanceID: "A", Epoch: a.Epoch, Sequence: 10, Key: other, Files: []inv.File{{Key: other, Path: "x", SHA256: strings.Repeat("b", 64), Size: 1}}}
	apply(&one)
	del := inv.Report{Version: 2, InstanceID: "A", Epoch: a.Epoch, Sequence: 11, Key: key, Deleted: true}
	apply(&del)
	apply(&del)
	old := del
	old.Sequence = 9
	old.Deleted = false
	old.Files = []inv.File{file}
	apply(&old)
	var count int64
	db.Model(&model.UploadInventoryHolding{}).Count(&count)
	if count != 2 {
		t.Fatalf("delete affected another node or repository: %d", count)
	}
	conflict := del
	conflict.Deleted = false
	conflict.Files = []inv.File{file}
	if _, e := dbDAO.ApplyUploadReport(ctx, &conflict); e == nil {
		t.Fatal("same sequence different bytes accepted")
	}
	// One failed identity validation must roll back deletion and inserted files.
	bad := one
	bad.Sequence = 12
	bad.Files = []inv.File{{Key: other, Path: "x", SHA256: strings.Repeat("b", 64), Size: 999}}
	if _, e := dbDAO.ApplyUploadReport(ctx, &bad); e == nil {
		t.Fatal("conflicting size accepted")
	}
	db.Model(&model.UploadInventoryHolding{}).Count(&count)
	if count != 2 {
		t.Fatal("failed transaction changed holdings")
	}
	newA := baseline("A")
	if newA.Epoch == a.Epoch {
		t.Fatal("manual reset reused epoch")
	}
	if _, e := dbDAO.ApplyUploadReport(ctx, &one); e == nil {
		t.Fatal("old epoch revived inventory")
	}
	// Lost baseline response replay cannot erase later repository reports.
	one.Epoch = newA.Epoch
	one.Sequence = 1
	apply(&one)
	apply(newA)
	db.Model(&model.UploadInventoryHolding{}).Count(&count)
	if count != 3 {
		t.Fatalf("baseline replay changed inventory: %d", count)
	}
	n, e := dbDAO.UploadReportStatus(ctx, "B")
	if e != nil || n.Epoch != b.Epoch {
		t.Fatal("reset changed another node")
	}
	var waters []model.UploadReportRepo
	db.Where("instance_id = ?", "A").Find(&waters)
	if len(waters) != 1 || waters[0].Sequence != 1 {
		t.Fatalf("reset watermark: %+v", waters)
	}
}
