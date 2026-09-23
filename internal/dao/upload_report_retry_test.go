package dao

import (
	"context"
	"dingoscheduler/internal/model"
	"testing"
)

func TestExplicitReconcileRestartsOnlyAttentionGeneration(t *testing.T) {
	d, db := inventoryTestDAO(t)
	if err := db.AutoMigrate(&model.UploadReportNode{}, &model.UploadReportRepo{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Dingospeed{InstanceID: "retry-node"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := d.UploadReportSession(ctx, "retry-node", true)
	if err != nil {
		t.Fatal(err)
	}
	same, err := d.UploadReportSession(ctx, "retry-node", true)
	if err != nil || same.PendingEpoch != first.PendingEpoch {
		t.Fatal("duplicate active request changed epoch", err)
	}
	if err := db.Model(&model.UploadReportNode{}).Where("instance_id = ?", "retry-node").Update("status", "needs_attention").Error; err != nil {
		t.Fatal(err)
	}
	same, err = d.UploadReportSession(ctx, "retry-node", false)
	if err != nil || same.PendingEpoch != first.PendingEpoch {
		t.Fatal("reconnect reset stopped task", err)
	}
	next, err := d.UploadReportSession(ctx, "retry-node", true)
	if err != nil || next.PendingEpoch == first.PendingEpoch || next.Status != "pending" {
		t.Fatal("explicit retry did not reset", err)
	}
}
