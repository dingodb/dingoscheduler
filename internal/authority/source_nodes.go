package authority

import (
	"context"
	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// sourceNodes discovers repository versions, including versions with no files.
// A failed read is not proof of absence; fail the listing so the UI retains its draft.
func (s *Service) sourceNodes(ctx context.Context, key repository.Key) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var nodes []model.Dingospeed
	if err := s.DB.WithContext(ctx).Order("instance_id, updated_at DESC, id").Find(&nodes).Error; err != nil {
		return nil, err
	}
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, n := range nodes {
		if seen[n.InstanceID] {
			continue
		}
		if time.Since(n.UpdatedAt) >= 5*time.Minute {
			var count int64
			err := s.DB.WithContext(ctx).Table("upload_inventory_holding h").Joins("JOIN upload_inventory_file f ON f.id=h.file_id").Where("h.instance_id=? AND f.namespace=? AND f.repo_type=? AND f.repo=?", n.InstanceID, key.Namespace, key.RepoType, key.Repo).Count(&count).Error
			if err != nil {
				return nil, err
			}
			if count > 0 {
				out = append(out, map[string]any{"node": n.InstanceID, "online": false})
				seen[n.InstanceID] = true
			}
			continue
		}
		endpoint := "http://" + net.JoinHostPort(n.Host, strconv.Itoa(int(n.Port))) + "/api/repositories/" + url.PathEscape(key.RepoType) + "/" + url.PathEscape(key.Namespace) + "/revisions?" + url.Values{"repo": {key.Repo}}.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.Client.Do(req)
		if err != nil {
			return nil, fail(503, fmt.Sprintf("source node %s revision listing unavailable: %v", n.InstanceID, err))
		}
		if resp.StatusCode == 404 {
			resp.Body.Close()
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fail(503, fmt.Sprintf("source node %s revision listing failed (HTTP %d)", n.InstanceID, resp.StatusCode))
		}
		var versions []struct {
			Name   string `json:"name"`
			Commit string `json:"commit"`
		}
		dec := json.NewDecoder(io.LimitReader(resp.Body, 32<<20))
		err = dec.Decode(&versions)
		if err == nil && dec.Decode(&struct{}{}) != io.EOF {
			err = fmt.Errorf("incomplete revision listing")
		}
		resp.Body.Close()
		if err != nil || versions == nil {
			return nil, fail(502, "incomplete source revision listing: "+n.InstanceID)
		}
		for _, v := range versions {
			if v.Name == "" || v.Commit == "" {
				return nil, fail(502, "invalid source revision listing: "+n.InstanceID)
			}
		}
		if len(versions) > 0 {
			out = append(out, map[string]any{"node": n.InstanceID, "online": true})
			seen[n.InstanceID] = true
		}
	}
	return map[string]any{"items": out}, nil
}
