package service

import (
	"context"
	"testing"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInventoryAttentionProgressFencesIncrementalAndPendingEpochs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UploadReportNode{}); err != nil {
		t.Fatal(err)
	}
	n := model.UploadReportNode{InstanceID: "node", Epoch: "active", Status: "completed"}
	if err := db.Create(&n).Error; err != nil {
		t.Fatal(err)
	}
	s := &SchedulerService{baseData: &data.BaseData{BizDB: db}}
	ctx := context.Background()
	if err := s.UploadReconcileProgress(ctx, "node", "obsolete", "needs_attention", "stale"); err != nil {
		t.Fatal(err)
	}
	db.Where("instance_id = ?", "node").Take(&n)
	if n.Status != "completed" {
		t.Fatal("stale report changed state")
	}
	if err := s.UploadReconcileProgress(ctx, "node", "active", "needs_attention", "conflict"); err != nil {
		t.Fatal(err)
	}
	db.Where("instance_id = ?", "node").Take(&n)
	if n.Status != "needs_attention" || n.Error != "conflict" {
		t.Fatalf("incremental stop hidden: %+v", n)
	}
	if err := db.Model(&n).Updates(map[string]any{"pending_epoch": "next", "status": "pending", "error": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.UploadReconcileProgress(ctx, "node", "active", "needs_attention", "late"); err != nil {
		t.Fatal(err)
	}
	db.Where("instance_id = ?", "node").Take(&n)
	if n.Status != "pending" {
		t.Fatal("old active epoch overwrote new reconciliation")
	}
	if err := s.UploadReconcileProgress(ctx, "node", "next", "needs_attention", "new conflict"); err != nil {
		t.Fatal(err)
	}
	db.Where("instance_id = ?", "node").Take(&n)
	if n.Status != "needs_attention" || n.Error != "new conflict" {
		t.Fatal("pending generation attention not stored")
	}
}
