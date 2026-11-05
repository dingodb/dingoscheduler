package dao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"dingoscheduler/internal/model"
	inv "dingoscheduler/pkg/inventory"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func TestInventoryIdentityReuseNeverAttemptsInsert(t *testing.T) {
	_, db := inventoryTestDAO(t)
	f := item("sample.bin", strings.Repeat("a", 64), 65536)
	var first model.UploadInventoryFile
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		first, err = ensureInventoryFile(tx, &f)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Model the deployed engine rejecting even a no-op upsert on a duplicate.
	if err := db.Callback().Create().Before("gorm:create").Register("test:reject_duplicate", func(tx *gorm.DB) {
		if tx.Statement.Table == "upload_inventory_file" {
			tx.AddError(&driver.MySQLError{Number: 1062, Message: "duplicate PRIMARY"})
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		got, err := ensureInventoryFile(tx, &f)
		if got.ID != first.ID {
			t.Fatalf("identity changed: %d -> %d", first.ID, got.ID)
		}
		return err
	}); err != nil {
		t.Fatal("existing file attempted an INSERT:", err)
	}
	f.Size++
	err := db.Transaction(func(tx *gorm.DB) error { _, err := ensureInventoryFile(tx, &f); return err })
	var reject *inv.Rejection
	if !errors.As(err, &reject) {
		t.Fatalf("size mismatch not rejected: %v", err)
	}
}

func TestInventoryInsertRaceRollsBackWholeTransaction(t *testing.T) {
	_, db := inventoryTestDAO(t)
	f := item("sample.bin", strings.Repeat("a", 64), 1)
	attempts := 0
	err := inventoryTransaction(context.Background(), db, func(tx *gorm.DB) error {
		attempts++
		var count int64
		tx.Model(&model.UploadInventoryFile{}).Count(&count)
		if count != 0 {
			t.Fatal("failed transaction leaked inserted file")
		}
		if _, err := ensureInventoryFile(tx, &f); err != nil {
			return err
		}
		if attempts == 1 {
			return inventoryIdentityRace{&driver.MySQLError{Number: 1062}}
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	var count int64
	db.Model(&model.UploadInventoryFile{}).Count(&count)
	if count != 1 {
		t.Fatalf("committed files=%d", count)
	}
	attempts = 0
	err = inventoryTransaction(context.Background(), db, func(tx *gorm.DB) error {
		attempts++
		return inventoryIdentityRace{&driver.MySQLError{Number: 1062}}
	})
	if err == nil || attempts != 3 {
		t.Fatalf("unbounded race retry: %d %v", attempts, err)
	}
}
