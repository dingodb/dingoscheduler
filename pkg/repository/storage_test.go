package repository

import (
	"strings"
	"testing"
)

func TestLegacyAndHostedStorage(t *testing.T) {
	for _, c := range []struct{ ns, api, org, repo string }{
		{"huggingface", "Qwen/demo", "Qwen", "demo"}, {"huggingface", "gpt2", "", "gpt2"},
		{"huggingface", "alice/demo", "alice", "demo"}, {"alice", "demo", "dingo-local/alice", "demo"},
		{"datacanvas", "team/demo", "dingo-local/datacanvas", "team/demo"},
		{"modelscope", "Qwen/demo", "modelscope/Qwen", "demo"},
	} {
		k := Key{Namespace: c.ns, RepoType: "models", Repo: c.api}
		org, repo, e := k.Storage()
		if e != nil || org != c.org || repo != c.repo {
			t.Fatalf("%+v => %s/%s %v", c, org, repo, e)
		}
		back, e := FromWire("models", org, repo)
		if e != nil || back != k {
			t.Fatalf("round trip %v %v", back, e)
		}
	}
}
func TestUnchangedSQLCapacity(t *testing.T) {
	k := Key{"alice", "models", strings.Repeat("a", 82)}
	if _, _, e := k.Storage(); e != nil {
		t.Fatal(e)
	}
	k.Repo += "a"
	if _, _, e := k.Storage(); e == nil {
		t.Fatal("accepted org_repo >100 bytes")
	}
}
