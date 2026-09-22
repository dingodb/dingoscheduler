package query

import (
	"encoding/json"
	"testing"
)

func TestLegacyPersistRequest(t *testing.T) {
	var r PersistRepoReq
	if e := json.Unmarshal([]byte(`{"instanceIds":["node-1"],"org":"Qwen","repo":"Qwen2.5-3B-Instruct","offVerify":false}`), &r); e != nil {
		t.Fatal(e)
	}
	if r.Org != "Qwen" || r.Repo != "Qwen2.5-3B-Instruct" || len(r.InstanceIds) != 1 || r.OffVerify {
		t.Fatalf("legacy request changed: %+v", r)
	}
}
func TestLegacyTaskRequest(t *testing.T) {
	var r CreateCacheJobReq
	json.Unmarshal([]byte(`{"instanceId":"node-1","datatype":"models","orgRepo":"Qwen/demo","org":"Qwen","repo":"demo","type":1}`), &r)
	if r.Org != "Qwen" || r.Repo != "demo" || r.OrgRepo != "Qwen/demo" {
		t.Fatalf("legacy task changed: %+v", r)
	}
}
