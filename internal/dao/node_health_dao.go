package dao

import (
	"context"
	"time"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/nodehealth"
	pb "dingoscheduler/pkg/proto/manager"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Update the report and communication timestamp atomically. A missing report
// clears the previous report, so downgraded clients never inherit old health.
func (d *DingospeedDao) HeartbeatHealth(ctx context.Context, req *pb.HeartbeatRequest, encoded *string, now time.Time) error {
	d.healthMu.Lock()
	defer d.healthMu.Unlock()
	result := d.baseData.BizDB.WithContext(ctx).Exec(
		"UPDATE dingospeed SET updated_at = ? WHERE id = ? AND instance_id = ? AND online = ?",
		now, req.Id, req.InstanceId, req.Online)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		// MySQL may report zero for an identical update in the same timestamp tick.
		var count int64
		err := d.baseData.BizDB.WithContext(ctx).Model(&model.Dingospeed{}).Where("id = ? AND instance_id = ? AND online = ?", req.Id, req.InstanceId, req.Online).Count(&count).Error
		if err != nil {
			return err
		}
		if count == 0 {
			return status.Error(codes.NotFound, "registered node not found")
		}
	}
	if d.health == nil {
		d.health = make(map[int32]*string)
	}
	d.health[req.Id] = encoded
	return nil
}

type nodeHealthRow struct {
	ID         int32
	InstanceID string
	Online     bool
	UpdatedAt  time.Time
}

type NodeHealthView struct {
	ID         int32  `json:"id"`
	InstanceID string `json:"instanceId"`
	OnlineMode bool   `json:"onlineMode"`
	nodehealth.View
}

func (d *DingospeedDao) ListNodeHealth(ctx context.Context, after int32, limit int, now time.Time) ([]NodeHealthView, error) {
	d.healthMu.Lock()
	defer d.healthMu.Unlock()
	var rows []nodeHealthRow
	err := d.baseData.BizDB.WithContext(ctx).Table("dingospeed").Select("id, instance_id, online, updated_at").Where("id > ?", after).Order("id ASC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]NodeHealthView, 0, len(rows))
	for _, r := range rows {
		result = append(result, NodeHealthView{r.ID, r.InstanceID, r.Online, nodehealth.Present(d.health[r.ID], r.UpdatedAt, now)})
	}
	return result, nil
}
