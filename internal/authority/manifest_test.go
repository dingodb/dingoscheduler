package authority

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"dingoscheduler/pkg/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestManifestReadDoesNotDependOnInventoryTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately create no inventory/holding/node tables: this read must only
	// require the authoritative definition even when every replica is unavailable.
	if err := db.AutoMigrate(&Definition{}); err != nil {
		t.Fatal(err)
	}
	key := repository.Key{Namespace: "datacanvas", RepoType: "models", Repo: "demo"}
	files := []File{{Path: "a", SHA256: strings.Repeat("a", 64), Size: 2}}
	body, _ := json.Marshal(files)
	d := Definition{ID: hash([]string{hash(key), "release"}), Repository: hash(key), Revision: "release", Version: 3, Manifest: string(body)}
	if err := db.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	result, err := New(db).Do(context.Background(), Request{Key: key, Action: "manifest", Revision: "release"})
	if err != nil {
		t.Fatal(err)
	}
	out := result.(Manifest)
	if out.Version != 3 || len(out.Files) != 1 || out.Files[0] != files[0] {
		t.Fatalf("wrong manifest %+v", out)
	}
	if _, err := New(db).Do(context.Background(), Request{Key: key, Action: "manifest", Revision: "missing"}); err == nil {
		t.Fatal("missing definition accepted")
	}
	key.Namespace = "huggingface"
	if _, err := New(db).Do(context.Background(), Request{Key: key, Action: "manifest", Revision: "release"}); err == nil {
		t.Fatal("remote entered upload authority")
	}
}
