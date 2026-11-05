package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/inventory"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func inventoryDeliveryFixture(t *testing.T, managementURL string) (*SchedulerService, *data.BaseData) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Dingospeed{}, &model.NodeEndpoint{}, &model.UploadReportNode{}, &model.UploadReportRepo{}, &model.UploadInventoryFile{}, &model.UploadInventoryHolding{}, &model.UploadInventoryState{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	node := model.Dingospeed{InstanceID: "node"}
	if err := db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.NodeEndpoint{NodeID: node.ID, ManagementURL: managementURL}).Error; err != nil {
		t.Fatal(err)
	}
	b := &data.BaseData{BizDB: db}
	repo := dao.NewRepositoryDao(b, nil, nil, nil, nil, nil)
	return NewSchedulerService(b, nil, nil, nil, repo, nil), b
}

func TestInventoryDeliveryFailureWaitsForNextExplicitReconciliation(t *testing.T) {
	var available atomic.Bool
	var attempts atomic.Int32
	notified := make(chan struct{}, 16)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		notified <- struct{}{}
		if !available.Load() {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(202)
	}))
	defer server.Close()
	s, b := inventoryDeliveryFixture(t, server.URL)
	ctx := context.Background()
	first, err := s.ReconcileUploadInventory(ctx, "node")
	if err != nil || first.Status != "needs_attention" || first.Error == "" || first.PendingEpoch == "" {
		t.Fatalf("failed delivery not retained: %+v %v", first, err)
	}
	<-notified
	stored, err := s.UploadReportSession(ctx, "node")
	if err != nil || stored.PendingEpoch != first.PendingEpoch || stored.Status != "needs_attention" {
		t.Fatalf("unfinished task lost: %+v %v", stored, err)
	}
	// Restarting Scheduler must not spawn a second delivery loop for the failed task.
	restarted := NewSchedulerService(b, nil, nil, nil, dao.NewRepositoryDao(b, nil, nil, nil, nil, nil), nil)
	select {
	case <-notified:
		t.Fatal("failed notification was automatically re-sent")
	case <-time.After(1200 * time.Millisecond):
	}
	if attempts.Load() != 1 {
		t.Fatalf("attempts=%d", attempts.Load())
	}
	available.Store(true)
	next, err := restarted.ReconcileUploadInventory(ctx, "node")
	if err != nil || next.PendingEpoch == first.PendingEpoch || next.Status != "pending" {
		t.Fatalf("next explicit task did not replace failed task: %+v %v", next, err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("explicit retry attempts=%d", attempts.Load())
	}
	// Rejecting a report must not push a new reconciliation to Speed either.
	_, err = restarted.UploadReport(ctx, &inventory.Report{Version: 2, InstanceID: "node", Epoch: next.PendingEpoch, Sequence: 1, Baseline: true})
	if err == nil {
		t.Fatal("invalid report accepted")
	}
	if attempts.Load() != 2 {
		t.Fatal("report rejection notified Speed to retry")
	}
}

func TestInventoryInitialSessionCreatesOnlyFirstBaseline(t *testing.T) {
	s, _ := inventoryDeliveryFixture(t, "")
	ctx := context.Background()
	first, err := s.UploadReportSession(ctx, "node")
	if err != nil || first.PendingEpoch == "" {
		t.Fatal("first join did not authorize baseline", err)
	}
	same, err := s.UploadReportSession(ctx, "node")
	if err != nil || same.PendingEpoch != first.PendingEpoch {
		t.Fatal("reconnect replaced unfinished task", err)
	}
	if _, err := s.UploadReport(ctx, &inventory.Report{Version: 2, InstanceID: "node", Epoch: first.PendingEpoch, Baseline: true, Files: []inventory.File{}}); err != nil {
		t.Fatal(err)
	}
	completed, err := s.UploadReportSession(ctx, "node")
	if err != nil || completed.PendingEpoch != "" || completed.Epoch != first.PendingEpoch {
		t.Fatalf("reconnect created another full task: %+v %v", completed, err)
	}
}
