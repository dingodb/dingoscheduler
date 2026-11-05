package dao

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	inv "dingoscheduler/pkg/inventory"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Explicit loopback opt-in. Only a new, uniquely named database is created/dropped.
func TestMySQLInventoryIdempotencyConcurrencyAndRollback(t *testing.T) {
	dsn := os.Getenv("INVENTORY_TEST_DSN")
	if dsn == "" {
		t.Skip("set INVENTORY_TEST_DSN for an isolated loopback MySQL")
	}
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || (host != "localhost" && host != "127.0.0.1") {
		t.Fatal("only loopback TCP MySQL is allowed")
	}
	cfg.DBName, cfg.ParseTime = "", true
	admin, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, _ := admin.DB()
	defer adminSQL.Close()
	name := fmt.Sprintf("inventory409_test_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE `" + name + "`").Error; err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP DATABASE `" + name + "`")
	cfg.DBName = name
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.AutoMigrate(&model.UploadReportNode{}, &model.UploadReportRepo{}, &model.UploadInventoryState{}, &model.UploadInventoryFile{}, &model.UploadInventoryHolding{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE dingospeed (id BIGINT AUTO_INCREMENT PRIMARY KEY, instance_id VARCHAR(191) NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	dao := &RepositoryDao{baseData: &data.BaseData{BizDB: db}}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	key := inv.Key{Namespace: "team", RepoType: "models", Repo: "acceptance"}
	file := inv.File{Key: key, Path: "sample.bin", SHA256: strings.Repeat("a", 64), Size: 65536}
	baseline := func(node string, bad bool) error {
		n, err := dao.UploadReportSession(ctx, node, true)
		if err != nil {
			return err
		}
		f := file
		if bad {
			f.Size++
		}
		p := &inv.Report{Version: 2, InstanceID: node, Epoch: n.PendingEpoch, Baseline: true, Files: []inv.File{f}}
		if _, err := dao.ApplyUploadReport(ctx, p); err != nil {
			return err
		}
		_, err = dao.ApplyUploadReport(ctx, p) // lost ACK replay
		return err
	}
	for _, node := range []string{"A", "B"} {
		if err := db.Exec("INSERT INTO dingospeed (instance_id) VALUES (?)", node).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := baseline("A", false); err != nil {
		t.Fatal(err)
	}
	if err := baseline("A", false); err != nil {
		t.Fatal("second generation:", err)
	}
	if err := baseline("B", false); err != nil {
		t.Fatal("cross-node reuse:", err)
	}
	if err := baseline("A", true); err == nil {
		t.Fatal("size conflict accepted")
	}
	var holdings, files int64
	db.Model(&model.UploadInventoryHolding{}).Count(&holdings)
	db.Model(&model.UploadInventoryFile{}).Count(&files)
	if holdings != 2 || files != 1 {
		t.Fatalf("failed transaction lost inventory: holdings=%d files=%d", holdings, files)
	}
	// Both transactions observe a missing identity before either can insert it.
	item := item("concurrent.bin", strings.Repeat("b", 64), 1)
	var arrivals atomic.Int32
	ready := make(chan struct{})
	if err := db.Callback().Create().Before("gorm:create").Register("test:simultaneous_identity", func(tx *gorm.DB) {
		if tx.Statement.Table != "upload_inventory_file" {
			return
		}
		if arrivals.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-ctx.Done():
			tx.AddError(ctx.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- inventoryTransaction(ctx, db, func(tx *gorm.DB) error { _, err := ensureInventoryFile(tx, &item); return err })
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent identity:", err)
		}
	}
	if arrivals.Load() != 2 {
		t.Fatalf("duplicate transaction did not reuse identity: creates=%d", arrivals.Load())
	}
	db.Model(&model.UploadInventoryFile{}).Count(&files)
	if files != 2 {
		t.Fatalf("concurrent inserts created %d identities", files)
	}
}
