package authority

import (
	"context"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestSourceNodesWithoutFileHoldings(t *testing.T) {
	for _, tc := range []struct {
		name, body   string
		status, want int
		failure      bool
	}{
		{"empty-version", `[{"name":"main","commit":"empty-manifest"}]`, 200, 1, false},
		{"missing-repository", `{}`, 404, 0, false},
		{"no-versions", `[]`, 200, 0, false},
		{"unavailable", `{}`, 503, 0, true},
		{"malformed", `[`, 200, 0, true},
		{"null", `null`, 200, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err = db.AutoMigrate(&model.Dingospeed{}, &model.UploadInventoryFile{}, &model.UploadInventoryHolding{}); err != nil {
				t.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/repositories/models/datacanvas/revisions" || r.URL.Query().Get("repo") != "empty" {
					t.Error(r.URL.String())
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			port, _ := strconv.Atoi(u.Port())
			if err = db.Create(&model.Dingospeed{InstanceID: "local-a", Host: u.Hostname(), Port: int32(port), UpdatedAt: time.Now()}).Error; err != nil {
				t.Fatal(err)
			}
			got, err := New(db).Do(context.Background(), Request{Key: repository.Key{Namespace: "datacanvas", RepoType: "models", Repo: "empty"}, Action: "nodes"})
			if tc.failure {
				if err == nil {
					t.Fatal("failed read accepted as absence")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			items := got.(map[string]any)["items"].([]map[string]any)
			if len(items) != tc.want {
				t.Fatalf("got %v", items)
			}
		})
	}
}
