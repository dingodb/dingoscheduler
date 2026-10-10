package dao

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
	"gorm.io/gorm"
)

// SyncSourceQuery identifies one uploaded file, never a mutable revision or a
// remote-provider object. Size is a consistency check, not a new identity.
type SyncSourceQuery struct {
	repository.Key
	Path              string
	SHA256            string
	Size              int64
	ExcludeInstanceID string
}

func (q SyncSourceQuery) Validate() error {
	if err := q.Key.Validate(); err != nil {
		return err
	}
	if q.Namespace == "huggingface" || q.Namespace == "modelscope" {
		return fmt.Errorf("uploaded sources require a hosted namespace")
	}
	if err := repository.ValidatePath(q.Path, true); err != nil {
		return err
	}
	digest, err := hex.DecodeString(q.SHA256)
	if err != nil || len(digest) != 32 || q.SHA256 != strings.ToLower(q.SHA256) {
		return fmt.Errorf("canonical SHA256 required")
	}
	if q.Size < 0 || len(q.ExcludeInstanceID) > 191 {
		return fmt.Errorf("invalid size or instance ID")
	}
	return nil
}

type SyncSource struct {
	InstanceID    string `json:"instanceId"`
	ManagementURL string `json:"managementUrl"`
}

// UploadedSyncSources reuses inventory identity and registered endpoints. Its
// indexed file lookup never enumerates repositories or scans node filesystems.
// Heartbeats select candidates; the target must still verify actual content.
func (r *RepositoryDao) UploadedSyncSources(ctx context.Context, q SyncSourceQuery) ([]SyncSource, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	identity := uploadIdentity(&UploadedInventoryItem{Namespace: q.Namespace, RepoType: q.RepoType, Repo: q.Repo, Path: q.Path, SHA256: q.SHA256})
	result := make([]SyncSource, 0)
	db := r.baseData.BizDB.WithContext(ctx)
	var file struct{ ID int64 }
	if err := db.Model(&model.UploadInventoryFile{}).Select("id").Where("identity_hash = ? AND size = ?", identity, q.Size).Take(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, nil
		}
		return nil, err
	}
	// Keep file/holding lookups indexed and query only related registrations.
	// DingoDB v3 cannot reliably plan the equivalent correlated multi-table join.
	holdingQuery := db.Model(&model.UploadInventoryHolding{}).Where("file_id = ?", file.ID)
	if q.ExcludeInstanceID != "" {
		holdingQuery = holdingQuery.Where("instance_id <> ?", q.ExcludeInstanceID)
	}
	var instances []string
	if err := holdingQuery.Order("instance_id").Pluck("instance_id", &instances).Error; err != nil {
		return nil, err
	}
	if len(instances) == 0 {
		return result, nil
	}
	var nodes []struct {
		ID         int32
		InstanceID string
		UpdatedAt  time.Time
	}
	if err := db.Model(&model.Dingospeed{}).Select("id, instance_id, updated_at").Where("instance_id IN ?", instances).Find(&nodes).Error; err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-5 * time.Minute)
	nodeIDs := make([]int32, 0, len(nodes))
	for i := range nodes {
		if nodes[i].UpdatedAt.After(cutoff) {
			nodeIDs = append(nodeIDs, nodes[i].ID)
		}
	}
	if len(nodeIDs) == 0 {
		return result, nil
	}
	var endpoints []model.NodeEndpoint
	if err := db.Where("node_id IN ?", nodeIDs).Find(&endpoints).Error; err != nil {
		return nil, err
	}
	byNode := make(map[int32]string, len(endpoints))
	for i := range endpoints {
		row := &endpoints[i]
		u, err := url.Parse(row.ManagementURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(row.ManagementURL) > 2048 {
			continue
		}
		byNode[row.NodeID] = row.ManagementURL
	}
	selected := make(map[string]int, len(instances))
	for i := range nodes {
		node := &nodes[i]
		if !node.UpdatedAt.After(cutoff) || byNode[node.ID] == "" {
			continue
		}
		previous, exists := selected[node.InstanceID]
		if !exists || node.UpdatedAt.After(nodes[previous].UpdatedAt) || (node.UpdatedAt.Equal(nodes[previous].UpdatedAt) && node.ID > nodes[previous].ID) {
			selected[node.InstanceID] = i
		}
	}
	result = make([]SyncSource, 0, len(selected))
	// Registration reuses IDs when a node switches running mode; heartbeat
	// recency, not MAX(id), identifies a current registered endpoint.
	for _, instance := range instances {
		if i, exists := selected[instance]; exists {
			result = append(result, SyncSource{InstanceID: instance, ManagementURL: byNode[nodes[i].ID]})
		}
	}
	return result, nil
}
