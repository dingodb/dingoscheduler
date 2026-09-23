package service

import (
	"bytes"
	"context"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/inventory"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (s *SchedulerService) UploadReport(ctx context.Context, p *inventory.Report) (inventory.Ack, error) {
	return s.repositoryDao.ApplyUploadReport(ctx, p)
}
func (s *SchedulerService) UploadReportSession(ctx context.Context, id string) (model.UploadReportNode, error) {
	return s.repositoryDao.UploadReportSession(ctx, id, false)
}
func (s *SchedulerService) UploadReportStatus(ctx context.Context, id string) (model.UploadReportNode, error) {
	return s.repositoryDao.UploadReportStatus(ctx, id)
}

func (s *SchedulerService) UploadReconcileProgress(ctx context.Context, id, epoch, status, message string) error {
	if status != "scanning" && status != "retrying" && status != "needs_attention" {
		return fmt.Errorf("invalid reconciliation status")
	}
	if len(message) > 4096 {
		message = message[:4096]
	}
	return s.baseData.BizDB.WithContext(ctx).Model(&model.UploadReportNode{}).Where("instance_id = ? AND pending_epoch = ? AND pending_epoch <> ''", id, epoch).Updates(map[string]any{"status": status, "error": message}).Error
}
func (s *SchedulerService) ReconcileUploadInventory(ctx context.Context, id string) (model.UploadReportNode, error) {
	n, err := s.repositoryDao.UploadReportSession(ctx, id, true)
	if err != nil {
		return n, err
	}
	// Delivery is only a wake-up. The durable pending epoch is also returned on reconnection.
	err = s.deliverInventoryReconcile(ctx, n)
	if err != nil {
		n.Error = err.Error()
		s.retryInventoryDelivery(n)
	}
	return n, nil
}

var inventoryDeliveries sync.Map

func (s *SchedulerService) resumeInventoryReconciles() {
	if s.baseData == nil || s.baseData.BizDB == nil {
		return
	}
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var pending []model.UploadReportNode
		err := s.baseData.BizDB.WithContext(ctx).Where("pending_epoch <> ''").Find(&pending).Error
		cancel()
		if err == nil {
			for _, n := range pending {
				s.retryInventoryDelivery(n)
			}
			return
		}
		time.Sleep(10 * time.Second)
	}
}
func (s *SchedulerService) retryInventoryDelivery(n model.UploadReportNode) {
	key := n.InstanceID + "/" + n.PendingEpoch
	if _, loaded := inventoryDeliveries.LoadOrStore(key, true); loaded {
		return
	}
	go func() {
		defer inventoryDeliveries.Delete(key)
		for delay := time.Second; ; {
			time.Sleep(delay)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			current, err := s.repositoryDao.UploadReportStatus(ctx, n.InstanceID)
			if err == nil && current.PendingEpoch != n.PendingEpoch {
				cancel()
				return
			}
			if err == nil {
				err = s.deliverInventoryReconcile(ctx, n)
			}
			cancel()
			if err == nil {
				return
			}
			if delay < time.Minute {
				delay *= 2
			}
		}
	}()
}
func (s *SchedulerService) deliverInventoryReconcile(ctx context.Context, n model.UploadReportNode) error {
	var endpoint model.NodeEndpoint
	err := s.baseData.BizDB.WithContext(ctx).Where("node_id = (?)", s.baseData.BizDB.Model(&model.Dingospeed{}).Select("MAX(id)").Where("instance_id = ?", n.InstanceID)).Take(&endpoint).Error
	if err != nil {
		return err
	}
	if endpoint.ManagementURL == "" {
		return fmt.Errorf("node management URL unavailable; task retained until reconnect")
	}
	b, _ := json.Marshal(map[string]string{"epoch": n.PendingEpoch})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint.ManagementURL, "/")+"/api/upload-inventory/reconcile", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("node reconciliation returned HTTP %d", resp.StatusCode)
	}
	return nil
}
