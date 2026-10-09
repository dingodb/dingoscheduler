package dao

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"dingoscheduler/pkg/repository"
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
	var rows []SyncSource
	db := r.baseData.BizDB.WithContext(ctx).Table("upload_inventory_file f").
		Select("h.instance_id, e.management_url").
		Joins("JOIN upload_inventory_holding h ON h.file_id = f.id").
		Joins("JOIN dingospeed d ON d.id = (SELECT MAX(d2.id) FROM dingospeed d2 WHERE d2.instance_id = h.instance_id)").
		Joins("JOIN node_endpoints e ON e.node_id = d.id").
		Where("f.identity_hash = ? AND f.size = ? AND d.updated_at > ?", identity, q.Size, time.Now().Add(-5*time.Minute))
	if q.ExcludeInstanceID != "" {
		db = db.Where("h.instance_id <> ?", q.ExcludeInstanceID)
	}
	if err := db.Order("h.instance_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]SyncSource, 0, len(rows))
	for _, row := range rows {
		u, err := url.Parse(row.ManagementURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(row.ManagementURL) > 2048 {
			continue
		}
		result = append(result, row)
	}
	return result, nil
}
