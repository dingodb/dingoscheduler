package dao

import (
	"encoding/json"
	"net/http"
	"testing"

	"dingoscheduler/internal/model/dto"
)

// 2026-10-14: CASIA-LM/ChineseWebText2.0 contains an empty .gitkeep. Speed has
// no record for it, so the fully cached dataset was never registered.
func TestVerifyRepoCompleteSkipsEmptyFiles(t *testing.T) {
	r, _, db := remoteTestDAO(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	var meta dto.CommitHfSha
	if err := json.Unmarshal([]byte(`{"sha":"c1","siblings":[
		{"rfilename":"a.bin","blobId":"oid-a","size":3},
		{"rfilename":"d/.gitkeep","blobId":"e69de29bb2d1d6434b8b29ae775ad8c2e48c5391","size":0}]}`), &meta); err != nil {
		t.Fatal(err)
	}
	if ok, err := r.verifyRepoComplete(&meta, "node", "models", "org", "demo"); err != nil || ok {
		t.Fatalf("uncached a.bin accepted: %v %v", ok, err)
	}
	putRemoteFile(t, db, "org", "a.bin", "oid-a", 3)
	if ok, err := r.verifyRepoComplete(&meta, "node", "models", "org", "demo"); err != nil || !ok {
		t.Fatalf("empty file blocked registration: %v %v", ok, err)
	}
}
