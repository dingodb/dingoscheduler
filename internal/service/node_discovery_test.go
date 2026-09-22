package service_test

import (
	"context"
	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/handler"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/service"
	pb "dingoscheduler/pkg/proto/manager"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"github.com/labstack/echo/v4"
	"github.com/patrickmn/go-cache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gorm.io/gorm"
	"net"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegisteredEndpointsSurviveDAORecreationAndAppearInDiscovery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.Dingospeed{}, &model.NodeEndpoint{}); err != nil {
		t.Fatal(err)
	}
	d := &data.BaseData{BizDB: db, Cache: cache.New(time.Minute, time.Minute)}
	newService := func() *service.SchedulerService {
		return service.NewSchedulerService(d, dao.NewDingospeedDao(d), nil, nil, nil, nil)
	}
	s := newService()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rpcServer := grpc.NewServer()
	pb.RegisterManagerServer(rpcServer, s)
	go func() { _ = rpcServer.Serve(listener) }()
	defer rpcServer.Stop()
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewManagerClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := &pb.RegisterRequest{InstanceId: "external-node", Host: "speed", Port: 8090, ManagementUrl: "http://speed:8091", DownloadUrl: "http://speed:8090"}
	first, err := client.Register(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	req.ManagementUrl = "http://new-entry:18091"
	second, err := client.Register(ctx, req)
	if err != nil || first.Id != second.Id {
		t.Fatalf("idempotent registration: %v %v", second, err)
	}
	s = newService()
	h := handler.NewManagerHandler(s, nil, nil, nil)
	rec := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/api/v1/nodes/health?endpoints=true", nil)
	if err = h.NodeHealth(echo.New().NewContext(request, rec)); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Items []struct {
			InstanceID    string `json:"instanceId"`
			ManagementURL string `json:"managementUrl"`
			Communication string `json:"communication"`
		}
	}
	if err = json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].ManagementURL != req.ManagementUrl || out.Items[0].Communication != "connected" {
		t.Fatalf("discovery: %s", rec.Body.String())
	}
}
