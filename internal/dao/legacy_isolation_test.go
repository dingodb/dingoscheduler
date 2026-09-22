package dao

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model/query"
	"dingoscheduler/pkg/config"
	pb "dingoscheduler/pkg/proto/manager"
	"dingoscheduler/pkg/repository"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// A deterministic SQL fixture executes selection over otherwise identical
// records. No local database service or configured production DSN is accessed.
type fixtureRow struct {
	id                                    int64
	repoType, namespace, repo, name, etag string
}
type fixtureConnector struct {
	records []fixtureRow
	queries *[]string
}

func (c fixtureConnector) Connect(context.Context) (driver.Conn, error) { return fixtureConn{c}, nil }
func (c fixtureConnector) Driver() driver.Driver                        { return fixtureDriver{} }

type fixtureDriver struct{}

func (fixtureDriver) Open(string) (driver.Conn, error) { return nil, fmt.Errorf("use connector") }

type fixtureConn struct{ fixtureConnector }

func (fixtureConn) Prepare(string) (driver.Stmt, error) { return nil, fmt.Errorf("unexpected prepare") }
func (fixtureConn) Close() error                        { return nil }
func (fixtureConn) Begin() (driver.Tx, error)           { return nil, fmt.Errorf("unexpected write transaction") }

type fixtureRows struct {
	ids []int64
	pos int
}

func (fixtureRows) Columns() []string { return []string{"id"} }
func (fixtureRows) Close() error      { return nil }
func (r *fixtureRows) Next(dest []driver.Value) error {
	if r.pos == len(r.ids) {
		return io.EOF
	}
	dest[0] = r.ids[r.pos]
	r.pos++
	return nil
}
func (c fixtureConn) QueryContext(_ context.Context, statement string, args []driver.NamedValue) (driver.Rows, error) {
	*c.queries = append(*c.queries, statement)
	if strings.Contains(statement, "model_file_process p") {
		var count int64
		if len(args) != 7 {
			return nil, fmt.Errorf("incomplete process identity")
		}
		for _, r := range c.records {
			if r.id == args[0].Value && args[1].Value == "node" && r.repoType == args[2].Value && r.namespace == args[3].Value && r.repo == args[4].Value && r.name == args[5].Value && r.etag == args[6].Value {
				count++
			}
		}
		return &fixtureRows{ids: []int64{count}}, nil
	}
	if !strings.Contains(statement, "datatype = ? AND org = ? AND repo = ?") || strings.Contains(statement, " OR ") {
		return nil, fmt.Errorf("query did not scope content selection to a complete identity: %s", statement)
	}
	if len(args) < 3 {
		return nil, fmt.Errorf("missing identity arguments")
	}
	ids := []int64{}
	for _, r := range c.records {
		if r.repoType != args[0].Value || r.namespace != args[1].Value || r.repo != args[2].Value {
			continue
		}
		index := 3
		if strings.Contains(statement, "etag = ?") {
			if r.etag != args[index].Value {
				continue
			}
			index++
		}
		if strings.Contains(statement, "name = ?") && r.name != args[index].Value {
			continue
		}
		ids = append(ids, r.id)
	}
	return &fixtureRows{ids: ids}, nil
}

func TestQueuedProgressCannotReuseAnotherNamespaceProcessID(t *testing.T) {
	db, _ := fixtureDB(t, []fixtureRow{{1, "models", "dingo-local/alice", "team/model", "nested/a.bin", "same"}})
	d := NewModelFileProcessDao(&data.BaseData{BizDB: db})
	entry := &pb.FileProcessEntry{ProcessId: 1, InstanceId: "node", DataType: "models", Org: "dingo-local/alice", Repo: "team/model", Name: "nested/a.bin", Etag: "same"}
	if err := d.ValidateProcessIdentity(entry); err != nil {
		t.Fatal(err)
	}
	entry.Org = "dingo-local/bob"
	if err := d.ValidateProcessIdentity(entry); err == nil {
		t.Fatal("accepted progress ID belonging to alice for bob")
	}
	entry.Org = "dingo-local/alice"
	entry.Repo = "team/other"
	if err := d.ValidateProcessIdentity(entry); err == nil {
		t.Fatal("accepted progress ID for another repository")
	}
}
func fixtureDB(t *testing.T, rows []fixtureRow) (*gorm.DB, *[]string) {
	t.Helper()
	statements := []string{}
	sqldb := sql.OpenDB(fixtureConnector{rows, &statements})
	t.Cleanup(func() { _ = sqldb.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqldb, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db, &statements
}

func TestContentDeletionNeverCrossesNamespaceRepoOrType(t *testing.T) {
	db, _ := fixtureDB(t, []fixtureRow{
		{1, "models", "dingo-local/alice", "team/model", "weights/a.bin", "same"},
		{2, "models", "dingo-local/bob", "team/model", "weights/a.bin", "same"},
		{3, "models", "dingo-local/alice", "other/model", "weights/a.bin", "same"},
		{4, "datasets", "dingo-local/alice", "team/model", "weights/a.bin", "same"},
		{5, "models", "dingo-local/alice", "team/model", "weights/a.bin", "different"},
	})
	d := NewModelFileRecordDao(&data.BaseData{BizDB: db})
	for _, name := range []string{"", "weights/a.bin"} {
		ids, err := d.GetIDsByEtagsOrFields("same", "models", "dingo-local/alice", "team/model", name)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(ids, []int64{1}) {
			t.Fatalf("deleted other identity: %v", ids)
		}
	}
	if _, err := d.GetIDsByEtagsOrFields("same", "", "", "", ""); err == nil {
		t.Fatal("accepted unscoped etag deletion")
	}
}

func TestRepositoryEnumerationUsesWholeIdentity(t *testing.T) {
	db, _ := fixtureDB(t, nil)
	statement := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		d := NewRepositoryDaoForTest(tx)
		_, _ = d.GetFreeRepository("node", "models", "dingo-local/alice", "team/model")
		return tx
	})
	for _, predicate := range []string{"r.datatype = t1.datatype", "r.org = t1.org", "r.repo = t1.repo", "NOT EXISTS"} {
		if !strings.Contains(statement, predicate) {
			t.Fatalf("missing %s: %s", predicate, statement)
		}
	}
}
func NewRepositoryDaoForTest(db *gorm.DB) *RepositoryDao {
	return &RepositoryDao{baseData: &data.BaseData{BizDB: db}}
}

func TestSpeedMetadataUsesIndependentIdentityAndCredentials(t *testing.T) {
	previous := config.SysConfig
	config.SysConfig = &config.Config{Retry: config.Retry{Attempts: 1}}
	t.Cleanup(func() { config.SysConfig = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/models/Qwen/Qwen3/revision/v1" {
			t.Errorf("wrong metadata address: %s", r.URL)
		}
		if r.Header.Get("X-Dingo-Service-Token") != "" || r.Header.Get("Authorization") != "Bearer upstream" {
			t.Error("credentials lost or merged")
		}
		_, _ = io.WriteString(w, `{"sha":"commit","siblings":[{"rfilename":"nested/model.bin"}],"usedStorage":9}`)
	}))
	defer server.Close()
	_, err := (&DingospeedDao{}).RemoteRequestMeta(server.URL, repository.Key{"huggingface", "models", "Qwen/Qwen3"}, "v1", map[string]string{"Authorization": "Bearer upstream"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecordLookupAcceptsEmptyLegacyOwner(t *testing.T) {
	db, _ := fixtureDB(t, []fixtureRow{{1, "models", "", "gpt2", "config.json", "sha"}, {2, "models", "other", "gpt2", "config.json", "sha"}})
	d := NewModelFileRecordDao(&data.BaseData{BizDB: db})
	r, e := d.FirstModelFileRecord(&query.ModelFileRecordQuery{Datatype: "models", Repo: "gpt2", FileName: "config.json", Etag: "sha"})
	if e != nil || r == nil || r.ID != 1 {
		t.Fatalf("legacy unowned: %+v %v", r, e)
	}
}
