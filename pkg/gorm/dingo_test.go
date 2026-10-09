package gorm

import (
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestMysqlDialectSelectionAndTypes(t *testing.T) {
	for _, version := range []string{"8.0.46", "5.7.44", "10.11.6-MariaDB", "8.0.11-TiDB-v8.1.0", "5.7.41-DingoDB-f7c8a95", "8.0.41-DingoDB-v3.0.0"} {
		t.Run(version, func(t *testing.T) {
			precision := 3
			d := mysqlCompatibilityDialector{&mysql.Dialector{Config: &mysql.Config{ServerVersion: version, DefaultDatetimePrecision: &precision}}}
			s, err := schema.Parse(&struct {
				ID       int64     `gorm:"primaryKey"`
				At       time.Time `gorm:"not null"`
				Optional *time.Time
				Text     string `gorm:"type:text"`
				Long     string `gorm:"type:longtext"`
				Label    string `gorm:"size:64"`
			}{}, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatal(err)
			}
			isDingo := strings.Contains(version, "DingoDB")
			_, custom := d.Migrator(&gorm.DB{}).(dingoMigrator)
			if custom != isDingo {
				t.Fatal("wrong migrator selected")
			}
			expected := map[string]string{"At": "timestamp(3)", "Optional": "timestamp(3) NULL", "Text": "varchar(65535)", "Long": "varchar(65536)", "Label": "varchar(64)"}
			for _, f := range s.Fields {
				want := d.Dialector.DataTypeOf(f)
				if isDingo {
					if mapped, ok := expected[f.Name]; ok {
						want = mapped
					}
				}
				if got := d.DataTypeOf(f); got != want {
					t.Fatalf("%s type=%q want=%q", f.Name, got, want)
				}
			}
		})
	}
}

func TestDingoIndexInspectionFailureIsReturned(t *testing.T) {
	// A closed connection makes metadata unavailable without network access.
	conn, err := sql.Open("mysql", "unused:unused@tcp(127.0.0.1:1)/unused")
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	d := mysqlCompatibilityDialector{&mysql.Dialector{Config: &mysql.Config{Conn: conn, ServerVersion: "8.0.41-DingoDB-v3.0.0", SkipInitializeWithVersion: true}}}
	db, err := gorm.Open(d, &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		ID    int    `gorm:"primaryKey"`
		Label string `gorm:"index:idx_label;size:32"`
	}
	if _, err := db.Migrator().GetIndexes(&value); err == nil {
		t.Fatal("metadata error swallowed")
	}
	if err := db.Migrator().CreateIndex(&value, "idx_label"); err == nil || !strings.Contains(err.Error(), "inspect DingoDB indexes") {
		t.Fatalf("expected inspection error, got %v", err)
	}
	if err := db.AutoMigrate(&value); err == nil {
		t.Fatal("failed migration reported success")
	}
}
