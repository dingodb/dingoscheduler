package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"dingoscheduler/internal/authority"
	"dingoscheduler/internal/data"
	"dingoscheduler/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

func TestOfficialRevisionWithoutSharedKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&authority.Definition{}); err != nil {
		t.Fatal(err)
	}
	h := NewManagerHandler(service.NewSchedulerService(&data.BaseData{BizDB: db}, nil, nil, nil, nil, nil), nil, nil, nil)
	for _, key := range []string{"", "obsolete-key"} {
		t.Setenv("DINGO_AUTHORITY_KEY", key)
		for _, tc := range []struct {
			body   string
			status int
		}{
			{`{"action":"list","namespace":"datacanvas","repoType":"models","repo":"demo","actor":"test-user"}`, 200},
			{`{"action":"list","namespace":"datacanvas","repoType":"models","repo":"demo"}`, 403},
			{`{"action":`, 400},
		} {
			rec := httptest.NewRecorder()
			ctx := echo.New().NewContext(httptest.NewRequest("POST", "/internal/v1/official-revisions", strings.NewReader(tc.body)), rec)
			if err := h.OfficialRevision(ctx); err != nil || rec.Code != tc.status {
				t.Fatalf("got %d %s err=%v, want %d", rec.Code, rec.Body.String(), err, tc.status)
			}
		}
	}
}
