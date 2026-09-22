package dao

import (
	"strings"
	"testing"
)

func TestPublishedManifestRejectsInvalidContent(t *testing.T) {
	good := PublishedFile{Path: "weights/a.bin", Size: 9, SHA256: strings.Repeat("a", 64)}
	for _, files := range [][]PublishedFile{{good, good}, {{Path: "../outside", Size: 9, SHA256: good.SHA256}}, {{Path: "a", Size: -1, SHA256: good.SHA256}}, {{Path: "a", Size: 9, SHA256: "not-a-hash"}}} {
		if _, e := validateSnapshot(PublishedSnapshot{Commit: "c", Files: files}); e == nil {
			t.Fatalf("accepted %+v", files)
		}
	}
	if n, e := validateSnapshot(PublishedSnapshot{Commit: "c", Files: []PublishedFile{good}}); e != nil || n != 9 {
		t.Fatal(n, e)
	}
}
