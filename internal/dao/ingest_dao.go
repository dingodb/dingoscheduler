package dao

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dingoscheduler/internal/model"
	pb "dingoscheduler/pkg/proto/manager"
	"dingoscheduler/pkg/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UploadedInventoryItem struct {
	Namespace string `json:"namespace"`
	RepoType  string `json:"repoType"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
}

type UploadedInventorySnapshot struct {
	Version        int                     `json:"version"`
	InstanceID     string                  `json:"instanceId"`
	Epoch          string                  `json:"epoch"`
	EpochStartedAt time.Time               `json:"epochStartedAt"`
	Sequence       uint64                  `json:"sequence"`
	GeneratedAt    time.Time               `json:"generatedAt"`
	Complete       bool                    `json:"complete"`
	Error          string                  `json:"error,omitempty"`
	Items          []UploadedInventoryItem `json:"items"`
}

// IngestPublished retains the new RPC's wire name but now pulls one durable
// complete inventory. revision/commit are wake-up hints, never inventory facts.
func (r *RepositoryDao) IngestPublished(ctx context.Context, req *pb.IngestRepositoryRequest) (*pb.IngestRepositoryResponse, error) {
	if req.InstanceId == "" || req.Commit == "" {
		return nil, fmt.Errorf("instance and inventory report token are required")
	}
	speed, err := r.dingospeedDao.GetEntity(req.InstanceId, req.Online)
	if err != nil {
		return nil, err
	}
	if speed == nil {
		return nil, fmt.Errorf("registered Speed not found")
	}
	endpoint := fmt.Sprintf("http://%s:%d/api/upload-inventory?token=%s", speed.Host, speed.Port, url.QueryEscape(req.Commit))
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload inventory returned %d", resp.StatusCode)
	}
	var snap UploadedInventorySnapshot
	decoder := json.NewDecoder(resp.Body)
	if err = decoder.Decode(&snap); err != nil {
		return nil, err
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("upload inventory contains trailing data")
	}
	if snap.InstanceID != req.InstanceId {
		return nil, fmt.Errorf("upload inventory instance mismatch")
	}
	if err = validateUploadedInventory(&snap); err != nil {
		return nil, err
	}
	accepted, count, total, err := r.ApplyUploadedInventory(ctx, &snap)
	if err != nil {
		return nil, err
	}
	commit := fmt.Sprintf("inventory:%s:%d", snap.Epoch, snap.Sequence)
	if !accepted {
		commit = "ignored:" + commit
	}
	return &pb.IngestRepositoryResponse{Commit: commit, FileCount: count, UsedStorage: total}, nil
}

func validateUploadedInventory(s *UploadedInventorySnapshot) error {
	if s.Version != 1 || s.InstanceID == "" || s.Epoch == "" || s.Sequence == 0 || s.EpochStartedAt.IsZero() || s.GeneratedAt.IsZero() {
		return fmt.Errorf("invalid upload inventory envelope")
	}
	seen := make(map[string]struct{}, len(s.Items))
	for i := range s.Items {
		item := &s.Items[i]
		key := repository.Key{Namespace: item.Namespace, RepoType: item.RepoType, Repo: item.Repo}
		if err := key.Validate(); err != nil {
			return err
		}
		if key.Namespace == "huggingface" || key.Namespace == "modelscope" {
			return fmt.Errorf("remote namespaces are not uploaded inventory")
		}
		if err := repository.ValidatePath(item.Path, true); err != nil || len(item.Path) > 1000 {
			return fmt.Errorf("invalid uploaded path %q", item.Path)
		}
		hash, err := hex.DecodeString(item.SHA256)
		if err != nil || len(hash) != 32 || strings.ToLower(item.SHA256) != item.SHA256 || item.Size < 0 {
			return fmt.Errorf("invalid uploaded content for %q", item.Path)
		}
		identity := uploadIdentity(item)
		if _, ok := seen[identity]; ok {
			return fmt.Errorf("duplicate uploaded inventory item")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func uploadIdentity(item *UploadedInventoryItem) string {
	raw := strings.Join([]string{item.Namespace, item.RepoType, item.Repo, item.Path, item.SHA256}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ApplyUploadedInventory is transactional and monotonic. Incomplete scans only
// mark confirmation state and never mutate holdings. Complete snapshots replace
// exactly one node's upload holdings and cannot touch remote-domain tables.
func (r *RepositoryDao) ApplyUploadedInventory(ctx context.Context, snap *UploadedInventorySnapshot) (accepted bool, count, total int64, err error) {
	err = r.baseData.BizDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state model.UploadInventoryState
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id = ?", snap.InstanceID).Take(&state).Error
		if findErr != nil && findErr != gorm.ErrRecordNotFound {
			return findErr
		}
		if findErr == nil {
			if snap.Epoch == state.Epoch && snap.Sequence <= state.LastSequence {
				return nil
			}
			if snap.Epoch != state.Epoch && !snap.EpochStartedAt.After(state.EpochStartedAt) {
				return nil
			}
		}
		accepted = true
		previousConfirmedAt := state.LastConfirmedAt
		now := time.Now().UTC()
		state = model.UploadInventoryState{InstanceID: snap.InstanceID, Epoch: snap.Epoch, EpochStartedAt: snap.EpochStartedAt.UTC(), LastSequence: snap.Sequence, InventoryComplete: snap.Complete, LastAttemptAt: now, ErrorMessage: snap.Error}
		if snap.Complete {
			confirmed := snap.GeneratedAt.UTC()
			state.LastConfirmedAt = &confirmed
		} else {
			state.LastConfirmedAt = previousConfirmedAt
		}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "instance_id"}}, DoUpdates: clause.AssignmentColumns([]string{"epoch", "epoch_started_at", "last_sequence", "inventory_complete", "last_attempt_at", "last_confirmed_at", "error_message"})}).Create(&state).Error; err != nil {
			return err
		}
		if !snap.Complete {
			return nil
		}
		fileIDs := make([]int64, 0, len(snap.Items))
		for i := range snap.Items {
			item := &snap.Items[i]
			identity := uploadIdentity(item)
			file := model.UploadInventoryFile{IdentityHash: identity, Namespace: item.Namespace, RepoType: item.RepoType, Repo: item.Repo, Path: item.Path, SHA256: item.SHA256, Size: item.Size}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "identity_hash"}}, DoNothing: true}).Create(&file).Error; err != nil {
				return err
			}
			if err := tx.Where("identity_hash = ?", identity).Take(&file).Error; err != nil {
				return err
			}
			if file.Size != item.Size || file.Namespace != item.Namespace || file.RepoType != item.RepoType || file.Repo != item.Repo || file.Path != item.Path || file.SHA256 != item.SHA256 {
				return fmt.Errorf("uploaded inventory identity collision or size conflict")
			}
			fileIDs = append(fileIDs, file.ID)
			holding := model.UploadInventoryHolding{FileID: file.ID, InstanceID: snap.InstanceID, Sequence: snap.Sequence, ConfirmedAt: snap.GeneratedAt.UTC()}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "file_id"}, {Name: "instance_id"}}, DoUpdates: clause.AssignmentColumns([]string{"sequence", "confirmed_at"})}).Create(&holding).Error; err != nil {
				return err
			}
			count++
			if total > math.MaxInt64-item.Size {
				return fmt.Errorf("uploaded inventory size overflow")
			}
			total += item.Size
		}
		remove := tx.Where("instance_id = ?", snap.InstanceID)
		if len(fileIDs) > 0 {
			remove = remove.Where("file_id NOT IN ?", fileIDs)
		}
		if err := remove.Delete(&model.UploadInventoryHolding{}).Error; err != nil {
			return err
		}
		return tx.Where("NOT EXISTS (SELECT 1 FROM upload_inventory_holding h WHERE h.file_id = upload_inventory_file.id)").Delete(&model.UploadInventoryFile{}).Error
	})
	return
}

type UploadedHoldingView struct {
	FileID            int64      `json:"fileId,string"`
	Namespace         string     `json:"namespace"`
	RepoType          string     `json:"repoType"`
	Repo              string     `json:"repo"`
	Path              string     `json:"path"`
	SHA256            string     `json:"sha256"`
	Size              int64      `json:"size"`
	InstanceID        string     `json:"instanceId"`
	ConfirmedAt       time.Time  `json:"confirmedAt"`
	NodeOnline        bool       `json:"nodeOnline"`
	NodeAvailable     bool       `json:"nodeAvailable"`
	HeartbeatAt       time.Time  `json:"heartbeatAt"`
	InventoryComplete bool       `json:"inventoryComplete"`
	LastConfirmedAt   *time.Time `json:"lastInventoryConfirmedAt,omitempty"`
}

type UploadedNodeInventoryStateView struct {
	InstanceID        string     `json:"instanceId"`
	Epoch             string     `json:"epoch"`
	LastSequence      uint64     `json:"lastSequence"`
	InventoryComplete bool       `json:"inventoryComplete"`
	LastAttemptAt     time.Time  `json:"lastAttemptAt"`
	LastConfirmedAt   *time.Time `json:"lastConfirmedAt,omitempty"`
	ErrorMessage      string     `json:"errorMessage,omitempty"`
	NodeAvailable     bool       `json:"nodeAvailable"`
	HeartbeatAt       time.Time  `json:"heartbeatAt"`
}

func (r *RepositoryDao) GetUploadedNodeInventoryState(ctx context.Context, instanceID string) (*UploadedNodeInventoryStateView, error) {
	var result UploadedNodeInventoryStateView
	err := r.baseData.BizDB.WithContext(ctx).Table("upload_inventory_state s").
		Select("s.instance_id, s.epoch, s.last_sequence, s.inventory_complete, s.last_attempt_at, s.last_confirmed_at, s.error_message, d.updated_at AS heartbeat_at").
		Joins("LEFT JOIN dingospeed d ON d.id = (SELECT MAX(d2.id) FROM dingospeed d2 WHERE d2.instance_id = s.instance_id)").
		Where("s.instance_id = ?", instanceID).Take(&result).Error
	if err != nil {
		return nil, err
	}
	result.NodeAvailable = !result.HeartbeatAt.IsZero() && result.HeartbeatAt.After(time.Now().Add(-5*time.Minute))
	return &result, nil
}

func (r *RepositoryDao) ListUploadedHoldings(ctx context.Context, instanceID, namespace, repoType, repo, path, sha string) ([]UploadedHoldingView, error) {
	var rows []UploadedHoldingView
	db := r.baseData.BizDB.WithContext(ctx).Table("upload_inventory_holding h").Select("f.id AS file_id, f.namespace, f.repo_type, f.repo, f.path, f.sha256, f.size, h.instance_id, h.confirmed_at, d.online AS node_online, d.updated_at AS heartbeat_at, s.inventory_complete, s.last_confirmed_at").Joins("JOIN upload_inventory_file f ON f.id = h.file_id").Joins("LEFT JOIN dingospeed d ON d.id = (SELECT MAX(d2.id) FROM dingospeed d2 WHERE d2.instance_id = h.instance_id)").Joins("LEFT JOIN upload_inventory_state s ON s.instance_id = h.instance_id")
	filters := [][2]string{{"h.instance_id", instanceID}, {"f.namespace", namespace}, {"f.repo_type", repoType}, {"f.repo", repo}, {"f.path", path}, {"f.sha256", sha}}
	for _, filter := range filters {
		if filter[1] != "" {
			db = db.Where(filter[0]+" = ?", filter[1])
		}
	}
	if err := db.Order("f.namespace, f.repo_type, f.repo, f.path, f.sha256, h.instance_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-5 * time.Minute)
	for i := range rows {
		// Current availability is a heartbeat observation. The legacy `online`
		// flag is retained in the response for compatibility but Speed currently
		// also uses it for its upstream-network mode, so it is not an availability
		// gate for uploaded local files.
		rows[i].NodeAvailable = !rows[i].HeartbeatAt.IsZero() && rows[i].HeartbeatAt.After(cutoff)
	}
	return rows, nil
}

type UploadedRepositoryView struct {
	Namespace    string `json:"namespace"`
	RepoType     string `json:"repoType"`
	Repo         string `json:"repo"`
	FileCount    int64  `json:"fileCount"`
	HoldingCount int64  `json:"holdingCount"`
}

func (r *RepositoryDao) ListUploadedRepositories(ctx context.Context) ([]UploadedRepositoryView, error) {
	var rows []UploadedRepositoryView
	err := r.baseData.BizDB.WithContext(ctx).Table("upload_inventory_file f").Select("f.namespace, f.repo_type, f.repo, COUNT(DISTINCT f.id) AS file_count, COUNT(h.id) AS holding_count").Joins("JOIN upload_inventory_holding h ON h.file_id = f.id").Group("f.namespace, f.repo_type, f.repo").Order("f.namespace, f.repo_type, f.repo").Find(&rows).Error
	return rows, err
}

// Legacy validation names are retained for the existing publication contract
// tests. They are no longer persisted or used as Scheduler inventory facts.
type PublishedFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type PublishedSnapshot struct {
	Commit string          `json:"commit"`
	Files  []PublishedFile `json:"files"`
}

func validateSnapshot(s PublishedSnapshot) (int64, error) {
	seen := map[string]struct{}{}
	var total int64
	for _, file := range s.Files {
		if err := repository.ValidatePath(file.Path, true); err != nil || len(file.Path) > 1000 || file.Size < 0 || total > math.MaxInt64-file.Size {
			return 0, fmt.Errorf("invalid published file")
		}
		hash, err := hex.DecodeString(file.SHA256)
		if err != nil || len(hash) != 32 || strings.ToLower(file.SHA256) != file.SHA256 {
			return 0, fmt.Errorf("invalid sha256")
		}
		if _, ok := seen[file.Path]; ok {
			return 0, fmt.Errorf("duplicate manifest path")
		}
		seen[file.Path] = struct{}{}
		total += file.Size
	}
	return total, nil
}
