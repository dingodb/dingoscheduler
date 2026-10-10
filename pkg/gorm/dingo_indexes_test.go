package gorm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type dingoIndexModel struct {
	ID       int    `gorm:"primaryKey"`
	Identity string `gorm:"uniqueIndex:idx_identity"`
	Group    string `gorm:"index:idx_group_path"`
	Path     string `gorm:"index:idx_group_path"`
}

type dingoIndexConnector struct{}

func (dingoIndexConnector) Connect(context.Context) (driver.Conn, error) {
	return dingoIndexConn{}, nil
}
func (dingoIndexConnector) Driver() driver.Driver { return dingoIndexDriver{} }

type dingoIndexDriver struct{}

func (dingoIndexDriver) Open(string) (driver.Conn, error) { return dingoIndexConn{}, nil }

type dingoIndexConn struct{}

func (dingoIndexConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepared statement")
}
func (dingoIndexConn) Close() error { return nil }
func (dingoIndexConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (dingoIndexConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.HasPrefix(query, "SHOW INDEX FROM ") {
		return nil, errors.New("unexpected metadata query: " + query)
	}
	// These are the lower-case column labels returned by DingoDB v3.0.0.
	return &dingoIndexRows{values: [][]driver.Value{
		{"idx_group_path", "path", int64(1), int64(2)},
		{"idx_identity", "identity", int64(0), int64(1)},
		{"PRIMARY", "id", int64(0), int64(1)},
		{"idx_group_path", "group", int64(1), int64(1)},
	}}, nil
}

type dingoIndexRows struct {
	values [][]driver.Value
	pos    int
}

func (*dingoIndexRows) Columns() []string {
	return []string{"key_name", "column_name", "non_unique", "seq_in_index"}
}
func (*dingoIndexRows) Close() error { return nil }
func (r *dingoIndexRows) Next(dst []driver.Value) error {
	if r.pos == len(r.values) {
		return io.EOF
	}
	copy(dst, r.values[r.pos])
	r.pos++
	return nil
}

func TestDingoExistingIndexesRemainDiscoverable(t *testing.T) {
	conn := sql.OpenDB(dingoIndexConnector{})
	t.Cleanup(func() { _ = conn.Close() })
	d := mysqlCompatibilityDialector{&mysql.Dialector{Config: &mysql.Config{
		Conn: conn, ServerVersion: "8.0.41-DingoDB-v3.0.0", SkipInitializeWithVersion: true,
	}}}
	db, err := gorm.Open(d, &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	indexes, err := db.Migrator().GetIndexes(&dingoIndexModel{})
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 3 {
		t.Fatalf("distinct existing indexes lost: %+v", indexes)
	}
	wantColumns := map[string][]string{
		"PRIMARY": {"id"}, "idx_identity": {"identity"}, "idx_group_path": {"group", "path"},
	}
	for _, index := range indexes {
		if !reflect.DeepEqual(index.Columns(), wantColumns[index.Name()]) {
			t.Fatalf("wrong identity or composite order: name=%q columns=%v", index.Name(), index.Columns())
		}
		unique, known := index.Unique()
		primary, primaryKnown := index.PrimaryKey()
		if !known || unique != (index.Name() != "idx_group_path") || !primaryKnown || primary != (index.Name() == "PRIMARY") {
			t.Fatalf("wrong index constraints: name=%q unique=%v primary=%v", index.Name(), unique, primary)
		}
	}
	for _, name := range []string{"PRIMARY", "idx_identity", "idx_group_path"} {
		if !db.Migrator().HasIndex(&dingoIndexModel{}, name) {
			t.Fatalf("existing index would be recreated: %s", name)
		}
	}
	if db.Migrator().HasIndex(&dingoIndexModel{}, "missing") {
		t.Fatal("missing index reported as existing")
	}
}
