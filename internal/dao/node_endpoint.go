package dao

import (
	"context"
	"dingoscheduler/internal/model"
	"gorm.io/gorm/clause"
)

func (d *DingospeedDao) SaveNodeEndpoint(ctx context.Context, id int32, management, download string) error {
	return d.baseData.BizDB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "node_id"}}, DoUpdates: clause.AssignmentColumns([]string{"management_url", "download_url"})}).Create(&model.NodeEndpoint{NodeID: id, ManagementURL: management, DownloadURL: download}).Error
}

func (d *DingospeedDao) NodeEndpoints(ctx context.Context, ids []int32) (map[int32]model.NodeEndpoint, error) {
	out := map[int32]model.NodeEndpoint{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []model.NodeEndpoint
	if err := d.baseData.BizDB.WithContext(ctx).Where("node_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.NodeID] = row
	}
	return out, nil
}
