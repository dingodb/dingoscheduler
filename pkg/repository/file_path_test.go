package repository

import (
	"net/url"
	"strings"
	"testing"
)

func TestRemoteFilePathAndOperationURI(t *testing.T) {
	for _, ns := range []string{"huggingface", "modelscope"} {
		k := Key{Namespace: ns, RepoType: "models", Repo: "team/demo"}
		for _, name := range []string{"nested/a|b.bin", "a:b?c*<d>\"", "CON", "trailing.", " space ", "data/中文#100%+ok.bin", "%2e%2e/literal", `samples\image.webp`, `nested/name\piece.bin`, `..\literal.bin`} {
			uri, err := k.OperationURI("file", "main", name)
			if err != nil {
				t.Fatalf("valid remote file %q: %v", name, err)
			}
			u, err := url.Parse(uri)
			if err != nil || u.Query().Get("path") != name {
				t.Fatalf("file name changed: %q %v", uri, err)
			}
		}
		for _, name := range []string{"", "/a", "../a", "a/../b", "a/./b", "a//b", "a/", `nested/../name\piece.bin`, "a\x00b", "a\nb", string([]byte{255}), strings.Repeat("a", 256)} {
			if k.FilePath(name) == nil {
				t.Errorf("unsafe remote file accepted: %q", name)
			}
		}
	}
	k := Key{}
	for _, ns := range []string{"dingo-local", "datacanvas"} {
		k.Namespace = ns
		for _, name := range []string{"a|b", `a\b`} {
			if k.FilePath(name) == nil {
				t.Errorf("changed %s naming policy for %q", ns, name)
			}
		}
	}
}
