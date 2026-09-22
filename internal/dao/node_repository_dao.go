package dao

import (
	"context"
	"time"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
)

type NodeRepositoryView struct {
	ID            int64     `json:"id,string"`
	Namespace     string    `json:"namespace"`
	Repo          string    `json:"repo"`
	Datatype      string    `json:"datatype"`
	IdentityValid bool      `json:"identityValid"`
	Commit        string    `json:"commit"`
	UsedStorage   int64     `json:"usedStorage"`
	MountStatus   int32     `json:"mountStatus"`
	ErrorMessage  string    `json:"errorMessage"`
	LastModified  string    `json:"lastModified"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type NodeRepositoryPage struct {
	NodeID      int32                `json:"nodeId"`
	InstanceID  string               `json:"instanceId"`
	Items       []NodeRepositoryView `json:"items"`
	Total       int64                `json:"total"`
	UsedStorage int64                `json:"usedStorage"`
	NextAfter   int64                `json:"nextAfter,string"`
	ObservedAt  time.Time            `json:"observedAt"`
}

// Read the existing instance projection only. This is not a disk inventory and
// does not trigger persistence, contact Speed, or alter existing records.
func (d *DingospeedDao) ListNodeRepositories(ctx context.Context, nodeID int32, after int64, limit int) (*NodeRepositoryPage, error) {
	db := d.baseData.BizDB.WithContext(ctx)
	var node model.Dingospeed
	if err := db.Select("id, instance_id").Where("id = ?", nodeID).Take(&node).Error; err != nil {
		return nil, err
	}
	page := &NodeRepositoryPage{NodeID: node.ID, InstanceID: node.InstanceID, Items: make([]NodeRepositoryView, 0)}
	var totals struct {
		Total       int64
		UsedStorage int64
	}
	if err := db.Model(&model.Repository{}).Select("COUNT(*) AS total, COALESCE(SUM(used_storage), 0) AS used_storage").Where("instance_id = ?", node.InstanceID).Scan(&totals).Error; err != nil {
		return nil, err
	}
	page.Total, page.UsedStorage = totals.Total, totals.UsedStorage
	var rows []model.Repository
	if err := db.Select("id, datatype, org, repo, sha, used_storage, status, error_msg, last_modified, updated_at").Where("instance_id = ? AND id > ?", node.InstanceID, after).Order("id ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		page.NextAfter = rows[len(rows)-1].ID
	}
	for _, row := range rows {
		key, err := repository.FromWire(row.Datatype, row.Org, row.Repo)
		item := NodeRepositoryView{ID: row.ID, Namespace: key.Namespace, Repo: key.Repo, Datatype: row.Datatype, IdentityValid: err == nil, Commit: row.Sha, UsedStorage: row.UsedStorage, MountStatus: row.Status, ErrorMessage: row.ErrorMsg, LastModified: row.LastModified, UpdatedAt: row.UpdatedAt}
		if err != nil {
			item.Namespace, item.Repo = row.Org, row.Repo
		}
		page.Items = append(page.Items, item)
	}
	page.ObservedAt = time.Now().UTC()
	return page, nil
}
