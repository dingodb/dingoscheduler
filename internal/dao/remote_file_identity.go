package dao

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"dingoscheduler/internal/model/dto"
	"dingoscheduler/pkg/repository"
)

type remoteTreeFile struct {
	Type string `json:"type"`
	Path string `json:"path"`
	OID  string `json:"oid"`
	Size *int64 `json:"size"`
	LFS  *struct {
		OID  string `json:"oid"`
		Size int64  `json:"size"`
	} `json:"lfs"`
}

// Resolve absent file identities in one recursive tree request at the pinned
// commit. Speed handles provider pagination; all metadata paths must be present.
// This avoids per-file HEAD requests (which can create cache links in Speed).
func (r *RepositoryDao) resolveFileIdentities(meta *dto.CommitHfSha, domain string, key repository.Key, headers map[string]string) error {
	needsTree := false
	for _, file := range meta.Siblings {
		if file.LFS == nil && (file.BlobID == "" || file.Size == nil) {
			needsTree = true
		}
	}
	tree := map[string]remoteTreeFile{}
	if needsTree {
		uri, err := key.OperationURI("tree", meta.Sha, "")
		if err != nil {
			return err
		}
		req, err := http.NewRequest(http.MethodGet, domain+uri+"&recursive=true", nil)
		if err != nil {
			return err
		}
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("snapshot tree returned HTTP %d", resp.StatusCode)
		}
		var files []remoteTreeFile
		if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
			return err
		}
		for _, file := range files {
			if file.Type != "file" {
				continue
			}
			if _, exists := tree[file.Path]; exists {
				return fmt.Errorf("duplicate snapshot tree path")
			}
			tree[file.Path] = file
		}
	}
	seen := make(map[string]bool)
	var total int64
	for i := range meta.Siblings {
		file := &meta.Siblings[i]
		if repository.ValidatePath(file.Rfilename, true) != nil || seen[file.Rfilename] {
			return fmt.Errorf("invalid or duplicate snapshot path")
		}
		seen[file.Rfilename] = true
		if file.LFS != nil {
			file.BlobID, file.Size = file.LFS.OID, &file.LFS.Size
		}
		if file.BlobID == "" || file.Size == nil {
			item, ok := tree[file.Rfilename]
			if !ok {
				return fmt.Errorf("snapshot tree is missing %s", file.Rfilename)
			}
			// HF blobId can identify a Git LFS pointer. The LFS OID identifies
			// the downloadable content used by Scheduler's file records.
			if item.LFS != nil {
				item.OID, item.Size = item.LFS.OID, &item.LFS.Size
			}
			file.BlobID, file.Size = item.OID, item.Size
		}
		if repository.ValidatePath(file.BlobID, false) != nil || file.Size == nil || *file.Size < 0 || *file.Size > math.MaxInt64-total {
			return fmt.Errorf("invalid snapshot content identity")
		}
		total += *file.Size
	}
	meta.UsedStorage = total
	return nil
}
