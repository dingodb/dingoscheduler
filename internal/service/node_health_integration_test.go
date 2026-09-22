package service_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/handler"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/router"
	"dingoscheduler/internal/service"
	"dingoscheduler/pkg/config"
	pb "dingoscheduler/pkg/proto/manager"
	"dingoscheduler/pkg/util"
	"github.com/labstack/echo/v4"
	"github.com/patrickmn/go-cache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Exercise the real gRPC -> service -> DAO -> SQL and HTTP handler chain using
// an in-memory SQL connector. It cannot open a network/database connection.
type healthStore struct {
	sync.Mutex
	rows map[int64]*healthRow
	fail bool
}
type healthRow struct {
	instance string
	online   bool
	received time.Time
	report   driver.Value
}
type healthConnector struct{ store *healthStore }

func (c healthConnector) Connect(context.Context) (driver.Conn, error) {
	return &healthConn{c.store}, nil
}
func (c healthConnector) Driver() driver.Driver { return healthDriver{} }

type healthDriver struct{}

func (healthDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("network databases forbidden in health test")
}

type healthConn struct{ store *healthStore }

func (c *healthConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (c *healthConn) Close() error              { return nil }
func (c *healthConn) Begin() (driver.Tx, error) { return nil, errors.New("unexpected transaction") }
func (c *healthConn) ExecContext(_ context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	c.store.Lock()
	defer c.store.Unlock()
	if c.store.fail {
		return nil, errors.New("simulated database unavailable")
	}
	if q != "UPDATE dingospeed SET updated_at = ? WHERE id = ? AND instance_id = ? AND online = ?" {
		return nil, fmt.Errorf("unexpected SQL: %s", q)
	}
	id := a[1].Value.(int64)
	r := c.store.rows[id]
	if r == nil || r.instance != a[2].Value || r.online != a[3].Value {
		return driver.RowsAffected(0), nil
	}
	r.received = a[0].Value.(time.Time)
	return driver.RowsAffected(1), nil
}

type healthRows struct {
	columns []string
	values  [][]driver.Value
	pos     int
}

func (r *healthRows) Columns() []string { return r.columns }
func (r *healthRows) Close() error      { return nil }
func (r *healthRows) Next(v []driver.Value) error {
	if r.pos >= len(r.values) {
		return io.EOF
	}
	copy(v, r.values[r.pos])
	r.pos++
	return nil
}
func (c *healthConn) QueryContext(_ context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	c.store.Lock()
	defer c.store.Unlock()
	if c.store.fail {
		return nil, errors.New("simulated database unavailable")
	}
	if strings.Contains(q, "count(*)") {
		var n int64
		r := c.store.rows[a[0].Value.(int64)]
		if r != nil && r.instance == a[1].Value && r.online == a[2].Value {
			n = 1
		}
		return &healthRows{columns: []string{"count(*)"}, values: [][]driver.Value{{n}}}, nil
	}
	if strings.Contains(q, "health_snapshot") || !strings.Contains(q, "ORDER BY id ASC LIMIT ?") {
		return nil, fmt.Errorf("unexpected SQL: %s", q)
	}
	rows := &healthRows{columns: []string{"id", "instance_id", "online", "updated_at"}}
	after, limit := a[0].Value.(int64), a[1].Value.(int64)
	for id := int64(1); id <= 3; id++ {
		if r := c.store.rows[id]; r != nil && id > after && int64(len(rows.values)) < limit {
			rows.values = append(rows.values, []driver.Value{id, r.instance, r.online, r.received})
		}
	}
	return rows, nil
}

func TestNodeHealthHeartbeatToHTTP(t *testing.T) {
	old := config.SysConfig
	config.SysConfig = &config.Config{}
	t.Cleanup(func() { config.SysConfig = old })
	now := time.Now().Truncate(time.Second)
	store := &healthStore{rows: map[int64]*healthRow{
		1: {instance: "hd-05", online: true, received: now},
		2: {instance: "hd-05", online: false, received: now},
		3: {instance: "legacy", online: true, received: now.Add(-10 * time.Minute)},
	}}
	sqlDB := sql.OpenDB(healthConnector{store})
	defer sqlDB.Close()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	base := &data.BaseData{BizDB: db, Cache: cache.New(time.Hour, time.Hour)}
	for id, r := range store.rows {
		base.Cache.Set(util.GetSpeedKey(r.instance, r.online), &model.Dingospeed{ID: int32(id), InstanceID: r.instance, Online: r.online}, cache.NoExpiration)
	}
	svc := service.NewSchedulerService(base, dao.NewDingospeedDao(base), nil, nil, nil, nil)
	lis := bufconn.Listen(64 * 1024)
	gs := grpc.NewServer()
	pb.RegisterManagerServer(gs, svc)
	go gs.Serve(lis)
	defer gs.Stop()
	conn, err := grpc.NewClient("passthrough:///local-health", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewManagerClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h := &pb.NodeHealthSnapshot{Version: 1, ProcessId: "boot-1", StartedAt: now.Unix() - 10, CollectedAt: now.Unix(), HeartbeatPeriodSeconds: 5,
		Capabilities: []*pb.CapabilityObservation{{Capability: "metadata_read", State: 2, Unresolved: true, LastObservation: now.Unix(), LastFailure: now.Unix()}, {Capability: "metadata_write"}},
		Errors:       []*pb.StorageErrorCount{{Operation: "read", Kind: "mount_disconnected", Count: 12}},
	}
	req := &pb.HeartbeatRequest{Id: 1, InstanceId: "hd-05", Online: true, Health: h}
	if _, err = client.Heartbeat(ctx, req); err != nil {
		t.Fatal(err)
	}
	fresh := dao.NewDingospeedDao(base)
	views, err := fresh.ListNodeHealth(ctx, 0, 10, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if views[0].Reported {
		t.Fatal("health unexpectedly survived DAO restart")
	}
	e := echo.New()
	manager := handler.NewManagerHandler(svc, nil, nil, nil)
	router.NewHttpRouter(e, manager, &handler.SysHandler{}, &handler.RepositoryHandler{}, &handler.TagHandler{}, &handler.CacheJobHandler{})
	get := func(query string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/health"+query, nil)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, r)
		return w
	}
	if w := get(""); w.Code != 200 {
		t.Fatalf("status endpoint without token: %d", w.Code)
	}
	w := get("?limit=1")
	if w.Code != 200 {
		t.Fatalf("status=%d body=%s", w.Code, w.Body)
	}
	var page struct {
		Items     []dao.NodeHealthView `json:"items"`
		NextAfter int32                `json:"nextAfter"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextAfter != 1 || page.Items[0].Communication != "connected" || page.Items[0].Capabilities[0].Status != "unresolved" {
		t.Fatalf("bad page: %+v", page)
	}
	if !strings.Contains(string(page.Items[0].Snapshot), `"count":"12"`) {
		t.Fatalf("counter not exposed losslessly: %s", page.Items[0].Snapshot)
	}
	if w = get("?after=1"); w.Code != 200 || strings.Contains(w.Body.String(), "boot-1") {
		t.Fatalf("pagination mixed node snapshots: %s", w.Body)
	}
	if w = get("?limit=201"); w.Code != 400 {
		t.Fatal("unbounded list accepted")
	}
	// Same instance online/offline processes remain independent.
	req.Online = false
	if _, err = client.Heartbeat(ctx, req); status.Code(err) != codes.NotFound {
		t.Fatalf("wrong mode accepted: %v", err)
	}
	req.Online = true
	req.Health.Version = 99
	if _, err = client.Heartbeat(ctx, req); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid version accepted: %v", err)
	}
	req.Health.Version = 1
	// Storage failure does not update communication timestamp or snapshot.
	store.Lock()
	store.fail = true
	before := store.rows[1].received
	store.Unlock()
	if _, err = client.Heartbeat(ctx, req); err == nil {
		t.Fatal("database failure acknowledged")
	}
	if w = get(""); w.Code != 503 {
		t.Fatal("failed query displayed healthy")
	}
	store.Lock()
	store.fail = false
	if store.rows[1].received != before {
		t.Fatal("failed write changed heartbeat time")
	}
	store.Unlock()
	// A fresh DAO loses health by design; no health_snapshot SQL column.
	items, err := dao.NewDingospeedDao(base).ListNodeHealth(ctx, 0, 100, time.Now())
	if err != nil || items[0].Reported {
		t.Fatalf("snapshot unexpectedly persisted through DAO: %v", err)
	}
	// A legacy heartbeat clears stale health instead of inheriting old success/failure.
	req.Health = nil
	if _, err = client.Heartbeat(ctx, req); err != nil {
		t.Fatal(err)
	}
	items, err = svc.NodeHealth(ctx, 0, 100)
	if err != nil || items[0].Reported || items[2].Communication != "disconnected" {
		t.Fatalf("legacy/disconnect handling: %+v %v", items, err)
	}
}
