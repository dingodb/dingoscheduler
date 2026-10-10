package gorm_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"dingoscheduler/internal/authority"
	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/config"
	manager "dingoscheduler/pkg/proto/manager"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type migrationLog struct {
	logger.Interface
	mu  sync.Mutex
	ddl []string
}

func (l *migrationLog) LogMode(level logger.LogLevel) logger.Interface { return l }
func (l *migrationLog) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	s, n := fc()
	upper := strings.ToUpper(strings.TrimSpace(s))
	if strings.HasPrefix(upper, "CREATE ") || strings.HasPrefix(upper, "ALTER ") || strings.HasPrefix(upper, "DROP ") {
		l.mu.Lock()
		l.ddl = append(l.ddl, s)
		l.mu.Unlock()
	}
	l.Interface.Trace(ctx, begin, func() (string, int64) { return s, n }, err)
}
func (l *migrationLog) reset() { l.mu.Lock(); defer l.mu.Unlock(); l.ddl = nil }
func (l *migrationLog) assertNoDDL(t *testing.T) {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.ddl) > 0 {
		t.Fatalf("unchanged schema issued DDL: %v", l.ddl)
	}
}

func migrationModels() []interface{} {
	return []interface{}{&model.NodeEndpoint{}, &model.UploadInventoryState{}, &model.UploadInventoryFile{}, &model.UploadInventoryHolding{}, &model.UploadReportNode{}, &model.UploadReportRepo{}, &authority.Definition{}, &authority.Receipt{}}
}

// Opt in with a localhost admin DSN. Each case uses a new isolated database;
// neither the configured database nor an existing database is migrated.
func TestDatabaseMigrationCompatibility(t *testing.T) {
	dsn := os.Getenv("SCHEDULER_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set SCHEDULER_MIGRATION_TEST_DSN to an isolated localhost MySQL/DingoDB admin DSN")
	}
	dc, err := driver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	host, portString, err := net.SplitHostPort(dc.Addr)
	if err != nil {
		t.Fatal(err)
	}
	if dc.Net != "tcp" || (host != "127.0.0.1" && host != "localhost") {
		t.Fatal("integration tests require a localhost TCP database")
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, _ := admin.DB()
	defer adminSQL.Close()
	var version string
	if err = admin.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("server=%s", version)
	for _, baseline := range []bool{true, false} {
		t.Run(fmt.Sprintf("old_schema_%t", baseline), func(t *testing.T) {
			name := fmt.Sprintf("scheduler_migration_test_%d", time.Now().UnixNano())
			if err := admin.Exec("CREATE DATABASE `" + name + "`").Error; err != nil {
				t.Fatal(err)
			}
			t.Logf("isolated schema retained for inspection: %s", name)
			cfg := &config.Config{BizDBConfig: config.DBConfig{Type: "mysql", Host: host, Port: port, User: dc.User, Password: dc.Passwd, Database: name, Timeout: "10s", MaxConn: 3, MaxIdleConn: 1}}
			cfg.Server.Mode = "release"
			oldConfig := config.SysConfig
			config.SysConfig = cfg
			defer func() { config.SysConfig = oldConfig }()
			log := &migrationLog{Interface: logger.Default.LogMode(logger.Info)}
			oldLogger := logger.Default
			logger.Default = log
			defer func() { logger.Default = oldLogger }()
			copyDC := *dc
			copyDC.DBName = name
			copyDC.ParseTime = true
			copyDC.InterpolateParams = true
			copyDC.Loc, _ = time.LoadLocation("Asia/Shanghai")
			old, err := gorm.Open(mysql.Open(copyDC.FormatDSN()), &gorm.Config{Logger: log})
			if err != nil {
				t.Fatal(err)
			}
			oldSQL, _ := old.DB()
			defer oldSQL.Close()
			if baseline {
				if err := old.AutoMigrate(migrationModels()...); err != nil {
					t.Fatal(err)
				}
			}
			// Legacy-domain sentinels use their historical model schemas. The
			// production startup must not migrate or replace any of them.
			legacy := []interface{}{&model.Repository{}, &model.ModelFileRecord{}, &model.ModelFileProcess{}, &model.Dingospeed{}}
			// Historical tables use second-precision CURRENT_TIMESTAMP defaults.
			zeroPrecision := 0
			legacyDB, err := gorm.Open(mysql.New(mysql.Config{Conn: oldSQL, DefaultDatetimePrecision: &zeroPrecision}), &gorm.Config{Logger: log})
			if err != nil {
				t.Fatal(err)
			}
			if err := legacyDB.Migrator().CreateTable(legacy...); err != nil {
				t.Fatal(err)
			}
			legacyRows := []interface{}{
				&model.Repository{ID: 101, InstanceId: "legacy-node", Datatype: "model", Org: "org", Repo: "repo", OrgRepo: "org/repo", Sha: "old-sha"},
				&model.ModelFileRecord{ID: 102, Datatype: "model", Org: "org", Repo: "repo", Name: "config.json", Etag: "etag", FileSize: 42},
				&model.ModelFileProcess{ID: 103, RecordID: 102, InstanceID: "legacy-node", OffsetNum: 42, Status: 3},
				&model.Dingospeed{ID: 104, InstanceID: "legacy-node", Host: "127.0.0.1", Port: 18080, Online: true},
			}
			for _, row := range legacyRows {
				if err := legacyDB.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			legacyDataBefore := rowSnapshot(t, old, []string{"repository", "model_file_record", "model_file_process", "dingospeed"})
			legacyBefore := schemaSnapshot(t, old, []string{"repository", "model_file_record", "model_file_process", "dingospeed"})
			now := time.Date(2026, 10, 9, 12, 34, 56, 123000000, copyDC.Loc)
			state := model.UploadInventoryState{InstanceID: "compat-node", Epoch: "epoch", EpochStartedAt: now, LastSequence: 42, InventoryComplete: true, LastAttemptAt: now, LastConfirmedAt: &now, ErrorMessage: "兼容性检查"}
			if baseline {
				seedMigrationRows(t, old, state, now)
			}
			var before map[string]string
			if baseline {
				before = schemaSnapshot(t, old, migrationTables())
			}
			log.reset()
			base, cleanup, err := data.NewBaseData(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if baseline {
				log.assertNoDDL(t)
			}
			db := base.BizDB
			if !baseline {
				seedMigrationRows(t, db, state, now)
				before = schemaSnapshot(t, db, migrationTables())
			}
			dataBefore := rowSnapshot(t, db, migrationTables())
			cleanup()
			for i := 0; i < 2; i++ {
				log.reset()
				base, cleanup, err = data.NewBaseData(cfg)
				if err != nil {
					t.Fatal(err)
				}
				log.assertNoDDL(t)
				db = base.BizDB
				var got model.UploadInventoryState
				if err := db.First(&got, "instance_id = ?", state.InstanceID).Error; err != nil {
					t.Fatal(err)
				}
				if got.LastSequence != 42 || !got.EpochStartedAt.Equal(now) || got.LastConfirmedAt == nil || !got.LastConfirmedAt.Equal(now) || got.ErrorMessage != state.ErrorMessage {
					t.Fatalf("state/time changed: %+v want %+v", got, state)
				}
				var def authority.Definition
				var receipt authority.Receipt
				if err := db.First(&def, "id = ?", "compat").Error; err != nil {
					t.Fatal(err)
				}
				if err := db.First(&receipt, "id = ?", "compat").Error; err != nil {
					t.Fatal(err)
				}
				if def.Version != 7 || def.Manifest != strings.Repeat("x", 8192) || receipt.Result != `{"version":7}` {
					t.Fatal("authority records changed")
				}
				if !reflect.DeepEqual(before, schemaSnapshot(t, db, migrationTables())) {
					t.Fatal("schema changed on restart")
				}
				if !reflect.DeepEqual(dataBefore, rowSnapshot(t, db, migrationTables())) {
					t.Fatal("inventory/report/authority data changed on restart")
				}
				if !reflect.DeepEqual(legacyDataBefore, rowSnapshot(t, db, []string{"repository", "model_file_record", "model_file_process", "dingospeed"})) {
					t.Fatal("legacy data changed on restart")
				}
				if !reflect.DeepEqual(legacyBefore, schemaSnapshot(t, db, []string{"repository", "model_file_record", "model_file_process", "dingospeed"})) {
					t.Fatal("legacy schemas changed")
				}
				if i == 0 {
					cleanup()
				}
			}
			defer cleanup()
			// Continue reading and updating the legacy records through the new connection.
			if err := db.Model(&model.Repository{}).Where("id = ?", 101).Update("sha", "new-sha").Error; err != nil {
				t.Fatal(err)
			}
			var repo model.Repository
			if err := db.First(&repo, 101).Error; err != nil {
				t.Fatal(err)
			}
			if repo.Sha != "new-sha" {
				t.Fatal("legacy update was not persisted")
			}
			// Real heartbeat and inventory writes must not become NULL when
			// the clock supplies more precision than DingoDB can parse.
			heartbeatAt := now.Add(456789 * time.Nanosecond)
			health := dao.NewDingospeedDao(base)
			if err := health.HeartbeatHealth(context.Background(), &manager.HeartbeatRequest{Id: 104, InstanceId: "legacy-node", Online: true}, nil, heartbeatAt); err != nil {
				t.Fatal(err)
			}
			var speed model.Dingospeed
			if err := db.First(&speed, 104).Error; err != nil {
				t.Fatal(err)
			}
			wantHeartbeat := heartbeatAt.Truncate(time.Second)
			if strings.Contains(strings.ToLower(version), "dingodb") {
				wantHeartbeat = heartbeatAt.Truncate(time.Millisecond)
			}
			if !speed.UpdatedAt.Equal(wantHeartbeat) {
				t.Fatalf("heartbeat timestamp lost: got=%s want=%s", speed.UpdatedAt, wantHeartbeat)
			}
			if err := db.Model(&model.UploadInventoryState{}).Where("instance_id = ?", state.InstanceID).Updates(map[string]interface{}{"last_attempt_at": heartbeatAt, "last_confirmed_at": &heartbeatAt}).Error; err != nil {
				t.Fatal(err)
			}
			var confirmed model.UploadInventoryState
			if err := db.First(&confirmed, "instance_id = ?", state.InstanceID).Error; err != nil {
				t.Fatal(err)
			}
			if !confirmed.LastAttemptAt.Equal(heartbeatAt.Truncate(time.Millisecond)) || confirmed.LastConfirmedAt == nil || !confirmed.LastConfirmedAt.Equal(heartbeatAt.Truncate(time.Millisecond)) {
				t.Fatalf("inventory timestamp lost: %+v", confirmed)
			}
			verifyEvolution(t, db, log)
			verifyTimestampEvolution(t, db, log, strings.Contains(strings.ToLower(version), "dingodb"), now)
		})
	}
}

type timestampV1 struct {
	ID int       `gorm:"primaryKey"`
	At time.Time `gorm:"not null;precision:3"`
}

func (timestampV1) TableName() string { return "migration_timestamp" }

type timestampV2 struct {
	ID int       `gorm:"primaryKey"`
	At time.Time `gorm:"not null;precision:6"`
}

func (timestampV2) TableName() string { return "migration_timestamp" }

func verifyTimestampEvolution(t *testing.T, db *gorm.DB, log *migrationLog, isDingo bool, now time.Time) {
	t.Helper()
	if err := db.AutoMigrate(&timestampV1{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&timestampV1{ID: 1, At: now}).Error; err != nil {
		t.Fatal(err)
	}
	before := schemaSnapshot(t, db, []string{"migration_timestamp"})
	err := db.AutoMigrate(&timestampV2{})
	if isDingo && err != nil {
		// This DingoDB release rejects MODIFY of a NOT NULL timestamp with
		// no default. A real change must fail visibly, never be skipped or
		// "fixed" by adding a default/changing nullability behind the model.
		if !strings.Contains(err.Error(), "DingoDB cannot migrate column") {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, schemaSnapshot(t, db, []string{"migration_timestamp"})) {
			t.Fatal("failed migration modified existing schema")
		}
		log.reset()
		if err := db.AutoMigrate(&timestampV1{}); err != nil {
			t.Fatal(err)
		}
		log.assertNoDDL(t)
		t.Logf("unsupported timestamp alteration is explicit and preserves data: %v", err)
	} else {
		if err != nil {
			t.Fatal(err)
		}
		columns, err := db.Migrator().ColumnTypes(&timestampV2{})
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range columns {
			if c.Name() == "at" {
				p, _, ok := c.DecimalSize()
				if !ok || p != 6 {
					t.Fatal("real precision change was silently skipped")
				}
			}
		}
		log.reset()
		if err := db.AutoMigrate(&timestampV2{}); err != nil {
			t.Fatal(err)
		}
		log.assertNoDDL(t)
	}
	var row timestampV1
	if err := db.First(&row, 1).Error; err != nil {
		t.Fatal(err)
	}
	if !row.At.Equal(now) {
		t.Fatalf("timestamp changed: %s want %s", row.At, now)
	}
}

func migrationTables() []string {
	return []string{"upload_inventory_state", "upload_inventory_file", "upload_inventory_holding", "upload_report_nodes", "upload_report_repos", "node_endpoints", "official_revision", "official_revision_receipt"}
}

func seedMigrationRows(t *testing.T, db *gorm.DB, state model.UploadInventoryState, now time.Time) {
	t.Helper()
	rows := []interface{}{
		&state,
		&model.UploadInventoryState{InstanceID: "null-time", Epoch: "epoch", EpochStartedAt: now, LastAttemptAt: now},
		&model.NodeEndpoint{NodeID: 1, ManagementURL: "http://localhost:18080", DownloadURL: "http://localhost:18080"},
		&model.UploadInventoryFile{ID: 1, IdentityHash: "identity", Namespace: "org", RepoType: "model", Repo: "repo", Path: "config.json", SHA256: "sha", Size: 42, CreatedAt: now, UpdatedAt: now},
		&model.UploadInventoryHolding{ID: 1, FileID: 1, InstanceID: "compat-node", Sequence: 42, ConfirmedAt: now},
		&model.UploadReportNode{InstanceID: "compat-node", Epoch: "epoch", Status: "ready", UpdatedAt: now},
		&model.UploadReportRepo{RepoHash: "repo-hash", InstanceID: "compat-node", Epoch: "epoch", Sequence: 42, Digest: "digest"},
		&authority.Definition{ID: "compat", Repository: "repo", Revision: "main", Version: 7, Manifest: strings.Repeat("x", 8192), UpdatedAt: now},
		&authority.Receipt{ID: "compat", Digest: "digest", Result: `{"version":7}`},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func rowSnapshot(t *testing.T, db *gorm.DB, tables []string) map[string][]map[string]interface{} {
	t.Helper()
	out := map[string][]map[string]interface{}{}
	for _, table := range tables {
		var rows []map[string]interface{}
		if err := db.Table(table).Order("1").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		out[table] = rows
	}
	return out
}

func schemaSnapshot(t *testing.T, db *gorm.DB, tables []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range tables {
		var name, ddl string
		if err := db.Raw("SHOW CREATE TABLE `"+table+"`").Row().Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		out[table] = ddl
	}
	return out
}

type evolutionV1 struct {
	ID   int64  `gorm:"primaryKey"`
	Name string `gorm:"size:32"`
}

func (evolutionV1) TableName() string { return "migration_evolution" }

type evolutionV2 struct {
	ID   int64   `gorm:"primaryKey"`
	Name string  `gorm:"size:32;index:idx_evolution_name"`
	Note *string `gorm:"size:128"`
}

func (evolutionV2) TableName() string { return "migration_evolution" }

type evolutionV3 struct {
	ID   int64   `gorm:"primaryKey"`
	Name string  `gorm:"size:64;index:idx_evolution_name;uniqueIndex:uk_evolution_pair"`
	Note *string `gorm:"size:128;uniqueIndex:uk_evolution_pair"`
}

func (evolutionV3) TableName() string { return "migration_evolution" }
func verifyEvolution(t *testing.T, db *gorm.DB, log *migrationLog) {
	t.Helper()
	if err := db.AutoMigrate(&evolutionV1{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&evolutionV1{ID: 1, Name: "old row"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&evolutionV2{}); err != nil {
		t.Fatal(err)
	}
	var got evolutionV2
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.Name != "old row" || got.Note != nil {
		t.Fatalf("evolution lost data: %+v", got)
	}
	if !db.Migrator().HasIndex(&evolutionV2{}, "idx_evolution_name") {
		t.Fatal("missing new index")
	}
	log.reset()
	if err := db.AutoMigrate(&evolutionV2{}); err != nil {
		t.Fatal(err)
	}
	log.assertNoDDL(t)
	if err := db.AutoMigrate(&evolutionV3{}); err != nil {
		t.Fatal(err)
	}
	note := "unique"
	if err := db.Model(&evolutionV3{}).Where("id = ?", 1).Update("note", note).Error; err != nil {
		t.Fatal(err)
	}
	indexes, err := db.Migrator().GetIndexes(&evolutionV3{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, idx := range indexes {
		if idx.Name() == "uk_evolution_pair" {
			unique, ok := idx.Unique()
			if !ok || !unique || !reflect.DeepEqual(idx.Columns(), []string{"name", "note"}) {
				t.Fatalf("incorrect composite unique index: %+v", idx)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing composite unique index")
	}
	if err := db.Create(&evolutionV3{ID: 2, Name: "old row", Note: &note}).Error; err == nil {
		t.Fatal("unique index accepted duplicate")
	}
	if err := db.Create(&evolutionV3{ID: 3, Name: strings.Repeat("a", 60), Note: &note}).Error; err != nil {
		t.Fatal("widened column rejects valid value:", err)
	}
	log.reset()
	if err := db.AutoMigrate(&evolutionV3{}); err != nil {
		t.Fatal(err)
	}
	log.assertNoDDL(t)
	if err := db.Migrator().DropIndex(&evolutionV3{}, "idx_evolution_name"); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasIndex(&evolutionV3{}, "idx_evolution_name") {
		t.Fatal("index was not dropped")
	}
	if err := db.AutoMigrate(&evolutionV3{}); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasIndex(&evolutionV3{}, "idx_evolution_name") {
		t.Fatal("missing index was not restored")
	}
}
