package dao

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"dingoscheduler/internal/model"
	inv "dingoscheduler/pkg/inventory"
	"dingoscheduler/pkg/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockReportNode(tx *gorm.DB, id string) (model.UploadReportNode, error) {
	n := model.UploadReportNode{InstanceID: id, Status: "uninitialized"}
	if id == "" || len(id) > 191 {
		return n, fmt.Errorf("invalid node identity")
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&n).Error; err != nil {
		return n, err
	}
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("instance_id = ?", id).Take(&n).Error
	return n, err
}

func (r *RepositoryDao) UploadReportSession(ctx context.Context, id string, explicit bool) (out model.UploadReportNode, err error) {
	err = r.baseData.BizDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.Dingospeed{}).Where("instance_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("node is not registered")
		}
		var e error
		out, e = lockReportNode(tx, id)
		if e != nil {
			return e
		}
		if ((explicit || out.Epoch == "") && out.PendingEpoch == "") || (explicit && out.Status == "needs_attention") {
			out.PendingEpoch, out.Status, out.Error = uuid.NewString(), "pending", ""
			return tx.Save(&out).Error
		}
		return nil
	})
	return
}

func (r *RepositoryDao) UploadReportStatus(ctx context.Context, id string) (model.UploadReportNode, error) {
	var n model.UploadReportNode
	err := r.baseData.BizDB.WithContext(ctx).Where("instance_id = ?", id).Take(&n).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UploadReportNode{InstanceID: id, Status: "uninitialized"}, nil
	}
	return n, err
}

func validateReport(p *inv.Report) error {
	if p.Version != 2 || p.InstanceID == "" || p.Epoch == "" || len(p.Epoch) > 64 {
		return fmt.Errorf("invalid report envelope")
	}
	if p.Baseline {
		if p.Sequence != 0 || p.Deleted || p.Key != (inv.Key{}) {
			return fmt.Errorf("invalid baseline")
		}
	} else {
		if p.Sequence == 0 {
			return fmt.Errorf("repository sequence must be positive")
		}
		if err := validateReportKey(p.Key); err != nil {
			return err
		}
		if p.Deleted && len(p.Files) > 0 {
			return fmt.Errorf("deleted repository has files")
		}
	}
	seen := map[string]bool{}
	for _, f := range p.Files {
		if err := validateReportKey(f.Key); err != nil {
			return err
		}
		if !p.Baseline && f.Key != p.Key {
			return fmt.Errorf("report contains another repository")
		}
		if err := repository.ValidatePath(f.Path, true); err != nil || len(f.Path) > 1000 {
			return fmt.Errorf("invalid file path")
		}
		h, e := hex.DecodeString(f.SHA256)
		if e != nil || len(h) != 32 || hex.EncodeToString(h) != f.SHA256 || f.Size < 0 {
			return fmt.Errorf("invalid file content")
		}
		id := f.Key.ID() + "\x00" + f.Path + "\x00" + f.SHA256
		if seen[id] {
			return fmt.Errorf("duplicate file")
		}
		seen[id] = true
	}
	return nil
}
func validateReportKey(k inv.Key) error {
	if k.Namespace == "huggingface" || k.Namespace == "modelscope" {
		return fmt.Errorf("remote namespace is not uploaded inventory")
	}
	return (repository.Key{Namespace: k.Namespace, RepoType: k.RepoType, Repo: k.Repo}).Validate()
}

// ApplyUploadReport serializes on this node's row, never on a global service mutex.
// Epoch, watermark and holdings commit together; a lost response can be retried verbatim.
func (r *RepositoryDao) ApplyUploadReport(ctx context.Context, p *inv.Report) (ack inv.Ack, err error) {
	if err = validateReport(p); err != nil {
		return
	}
	ack = inv.Ack{Epoch: p.Epoch, Sequence: p.Sequence, Digest: p.Digest(), Status: "accepted"}
	err = r.baseData.BizDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		n, e := lockReportNode(tx, p.InstanceID)
		if e != nil {
			return e
		}
		var state model.UploadReportRepo
		if p.Baseline {
			if n.Epoch == p.Epoch {
				if n.BaselineDigest != ack.Digest {
					return fmt.Errorf("baseline content conflict")
				}
				ack.Status = "confirmed"
				return nil
			}
			if n.PendingEpoch != p.Epoch {
				return fmt.Errorf("baseline epoch is not authorized")
			}
		} else {
			if n.Epoch != p.Epoch {
				return fmt.Errorf("report epoch is obsolete; explicit reconciliation required")
			}
			h := sha256.Sum256([]byte(p.InstanceID + "\x00" + p.Key.ID()))
			state.RepoHash = hex.EncodeToString(h[:])
			e = tx.Where("repo_hash = ?", state.RepoHash).Take(&state).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
			if e == nil && state.Sequence >= p.Sequence {
				if state.Sequence == p.Sequence && state.Digest != ack.Digest {
					return fmt.Errorf("sequence content conflict")
				}
				ack.Status = "confirmed"
				if state.Sequence > p.Sequence {
					ack.Status = "obsolete"
				}
				return nil
			}
		}
		// Delete only the selected scope, then insert the replacement in the same transaction.
		remove := tx.Where("instance_id = ?", p.InstanceID)
		if !p.Baseline {
			ids := tx.Model(&model.UploadInventoryFile{}).Select("id").Where("namespace = ? AND repo_type = ? AND repo = ?", p.Namespace, p.RepoType, p.Repo)
			remove = remove.Where("file_id IN (?)", ids)
		}
		if e = remove.Delete(&model.UploadInventoryHolding{}).Error; e != nil {
			return e
		}
		for _, f := range p.Files {
			item := UploadedInventoryItem{Namespace: f.Namespace, RepoType: f.RepoType, Repo: f.Repo, Path: f.Path, SHA256: f.SHA256, Size: f.Size}
			file := model.UploadInventoryFile{IdentityHash: uploadIdentity(&item), Namespace: f.Namespace, RepoType: f.RepoType, Repo: f.Repo, Path: f.Path, SHA256: f.SHA256, Size: f.Size}
			if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&file).Error; e != nil {
				return e
			}
			if e = tx.Where("identity_hash = ?", file.IdentityHash).Take(&file).Error; e != nil {
				return e
			}
			if file.Size != f.Size || file.Namespace != f.Namespace || file.RepoType != f.RepoType || file.Repo != f.Repo || file.Path != f.Path || file.SHA256 != f.SHA256 {
				return fmt.Errorf("file identity conflict")
			}
			if e = tx.Create(&model.UploadInventoryHolding{FileID: file.ID, InstanceID: p.InstanceID, Sequence: p.Sequence, ConfirmedAt: time.Now().UTC()}).Error; e != nil {
				return e
			}
		}
		if p.Baseline {
			if e = tx.Where("instance_id = ?", p.InstanceID).Delete(&model.UploadReportRepo{}).Error; e != nil {
				return e
			}
			n.Epoch, n.PendingEpoch, n.BaselineDigest, n.Status, n.Error = p.Epoch, "", ack.Digest, "completed", ""
			if e = tx.Save(&n).Error; e != nil {
				return e
			}
		} else {
			state.InstanceID, state.Epoch, state.Sequence, state.Digest = p.InstanceID, p.Epoch, p.Sequence, ack.Digest
			if e = tx.Save(&state).Error; e != nil {
				return e
			}
		}
		now := time.Now().UTC()
		view := model.UploadInventoryState{InstanceID: p.InstanceID, Epoch: p.Epoch, EpochStartedAt: now, LastSequence: p.Sequence, InventoryComplete: true, LastAttemptAt: now, LastConfirmedAt: &now}
		if e = tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&view).Error; e != nil {
			return e
		}
		// Orphan identities are retained: concurrent reports from another node may be inserting holdings.
		return nil
	})
	return
}
