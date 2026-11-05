package dao

import (
	"context"
	"errors"
	"fmt"

	"dingoscheduler/internal/model"
	inv "dingoscheduler/pkg/inventory"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// A duplicate insert may invalidate the transaction or its read snapshot. Retry
// the entire transaction, never swallow the error and continue in that snapshot.
type inventoryIdentityRace struct{ error }

func inventoryTransaction(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		err := db.WithContext(ctx).Transaction(fn)
		var race inventoryIdentityRace
		if !errors.As(err, &race) || attempt == 2 || ctx.Err() != nil {
			return err
		}
	}
}

func (e inventoryIdentityRace) Unwrap() error { return e.error }

func inventoryDuplicate(err error) bool {
	var e *driver.MySQLError
	return errors.Is(err, gorm.ErrDuplicatedKey) || (errors.As(err, &e) && e.Number == 1062)
}

func ensureInventoryFile(tx *gorm.DB, item *UploadedInventoryItem) (model.UploadInventoryFile, error) {
	var file model.UploadInventoryFile
	identity := uploadIdentity(item)
	err := tx.Where("identity_hash = ?", identity).Take(&file).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		file = model.UploadInventoryFile{IdentityHash: identity, Namespace: item.Namespace, RepoType: item.RepoType, Repo: item.Repo, Path: item.Path, SHA256: item.SHA256, Size: item.Size}
		// Existing identities are read and reused; no no-op upsert is needed.
		err = tx.Create(&file).Error
		if inventoryDuplicate(err) {
			return file, inventoryIdentityRace{fmt.Errorf("insert uploaded file identity: %w", err)}
		}
		if err == nil {
			// Preserve the old path's authoritative ID read rather than relying
			// on LastInsertId behavior of MySQL-compatible servers.
			file = model.UploadInventoryFile{}
			err = tx.Where("identity_hash = ?", identity).Take(&file).Error
		}
	}
	if err != nil {
		return file, err
	}
	if file.IdentityHash != identity || file.Size != item.Size || file.Namespace != item.Namespace || file.RepoType != item.RepoType || file.Repo != item.Repo || file.Path != item.Path || file.SHA256 != item.SHA256 {
		return file, inv.Reject("inventory_identity_conflict", "uploaded inventory identity collision or size conflict")
	}
	return file, nil
}
