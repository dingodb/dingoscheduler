package handler

import (
	"dingoscheduler/internal/dao"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/model"
	"dingoscheduler/internal/service"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func TestSyncSourcesAvailableWithoutKey(t *testing.T) {
	t.Setenv("DINGO_SYNC_KEY", "")
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&model.UploadInventoryFile{}, &model.UploadInventoryHolding{}, &model.Dingospeed{}, &model.NodeEndpoint{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	repo := dao.NewRepositoryDao(&data.BaseData{BizDB: db}, nil, nil, nil, nil, nil)
	h := NewManagerHandler(service.NewSchedulerService(nil, nil, nil, nil, repo, nil), nil, nil, nil)
	r := httptest.NewRequest("GET", "/api/v1/upload-inventory/sources?namespace=team&repoType=models&repo=demo&path=a&sha256="+strings.Repeat("a", 64)+"&size=0", nil)
	w := httptest.NewRecorder()
	if err = h.UploadedSyncSources(echo.New().NewContext(r, w)); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestSyncSourcesRejectsIncompleteRequestsWithoutKey(t *testing.T) {
	t.Setenv("DINGO_SYNC_KEY", "")
	valid := "?namespace=team&repoType=models&repo=demo&path=a&sha256=" + strings.Repeat("a", 64) + "&size=0"
	for _, tc := range []struct {
		name, query string
		want        int
	}{
		{"no size", strings.TrimSuffix(valid, "&size=0"), 400},
		{"duplicate", valid + "&repo=other", 400},
		{"remote", strings.Replace(valid, "namespace=team", "namespace=huggingface", 1), 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/api/v1/upload-inventory/sources"+tc.query, nil)
			w := httptest.NewRecorder()
			// Nil service proves rejected calls never reach the inventory store.
			if err := (&ManagerHandler{}).UploadedSyncSources(echo.New().NewContext(r, w)); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.want {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
