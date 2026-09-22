// Package authority owns explicitly confirmed official definitions, independently
// of inventory lifetime. It never invokes a Speed mutation.
package authority

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"dingoscheduler/internal/model"
	"dingoscheduler/pkg/repository"
	dbmysql "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Definition struct {
	ID         string    `gorm:"primaryKey;size:64" json:"-"`
	Repository string    `gorm:"size:64;index;not null" json:"-"`
	Revision   string    `gorm:"size:255;not null" json:"revision"`
	Version    uint64    `gorm:"not null" json:"version"`
	Manifest   string    `gorm:"type:longtext;not null" json:"-"`
	Actor      string    `gorm:"size:255" json:"actor"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (Definition) TableName() string { return "official_revision" }

type Receipt struct {
	ID     string `gorm:"primaryKey;size:64"`
	Digest string `gorm:"size:64;not null"`
	Result string `gorm:"type:longtext;not null"`
}

func (Receipt) TableName() string { return "official_revision_receipt" }

type View struct {
	Definition
	Files     []File          `json:"files"`
	Available map[string]bool `json:"available"`
}
type Source struct {
	Node     string `json:"node"`
	Revision string `json:"revision"`
	Commit   string `json:"commit"`
}
type Change struct {
	Path   string  `json:"path"`
	File   *File   `json:"file"`
	Source *Source `json:"source"`
}
type Request struct {
	repository.Key
	Action      string   `json:"action"`
	Revision    string   `json:"revision"`
	Node        string   `json:"node"`
	BaseVersion uint64   `json:"baseVersion"`
	RequestID   string   `json:"requestId"`
	Actor       string   `json:"actor"`
	Changes     []Change `json:"changes"`
}
type Failure struct {
	Status  int
	Message string
}

func (e *Failure) Error() string            { return e.Message }
func fail(status int, message string) error { return &Failure{status, message} }

type Service struct {
	DB     *gorm.DB
	Client *http.Client
}

func New(db *gorm.DB) *Service {
	return &Service{db, &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Service) Do(ctx context.Context, r Request) (any, error) {
	if err := r.Key.Validate(); err != nil {
		return nil, fail(400, err.Error())
	}
	if r.Namespace == "huggingface" || r.Namespace == "modelscope" {
		return nil, fail(403, "remote repositories are read only")
	}
	switch r.Action {
	case "list":
		rows := []Definition{}
		if err := s.DB.WithContext(ctx).Where("repository = ?", hash(r.Key)).Order("revision").Find(&rows).Error; err != nil {
			return nil, err
		}
		return map[string]any{"items": rows}, nil
	case "get":
		return s.get(ctx, r)
	case "nodes":
		var nodes []model.Dingospeed
		if err := s.DB.WithContext(ctx).Where("instance_id IN (?)", s.DB.Model(&model.UploadInventoryHolding{}).Select("instance_id").Where("file_id IN (?)", s.DB.Model(&model.UploadInventoryFile{}).Select("id").Where("namespace = ? AND repo_type = ? AND repo = ?", r.Namespace, r.RepoType, r.Repo))).Find(&nodes).Error; err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, n := range nodes {
			out = append(out, map[string]any{"node": n.InstanceID, "online": time.Since(n.UpdatedAt) < 5*time.Minute})
		}
		return map[string]any{"items": out}, nil
	case "revisions":
		var out []struct {
			Name   string `json:"name"`
			Commit string `json:"commit"`
		}
		if err := s.read(ctx, r.Key, r.Node, "revisions", "", &out); err != nil {
			return nil, err
		}
		if out == nil {
			return nil, fail(502, "incomplete revision listing")
		}
		return map[string]any{"items": out}, nil
	case "snapshot":
		return s.snapshot(ctx, r.Key, r.Node, r.Revision)
	case "commit":
		return s.commit(ctx, r)
	default:
		return nil, fail(400, "unknown official operation")
	}
}
func (s *Service) get(ctx context.Context, r Request) (View, error) {
	v := View{Files: []File{}, Available: map[string]bool{}}
	err := s.DB.WithContext(ctx).Where("id = ?", hash([]string{hash(r.Key), r.Revision})).Take(&v.Definition).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return v, fail(404, "official revision not found")
	}
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal([]byte(v.Manifest), &v.Files); err != nil {
		return v, err
	}
	for _, f := range v.Files {
		var count int64
		err = s.DB.WithContext(ctx).Table("upload_inventory_holding h").Joins("JOIN upload_inventory_file f ON f.id=h.file_id").Joins("JOIN dingospeed n ON n.instance_id=h.instance_id").Where("f.namespace=? AND f.repo_type=? AND f.repo=? AND f.path=? AND f.sha256=? AND n.updated_at>?", r.Namespace, r.RepoType, r.Repo, f.Path, f.SHA256, time.Now().Add(-5*time.Minute)).Count(&count).Error
		if err != nil {
			return v, err
		}
		v.Available[f.Path] = count > 0
	}
	return v, nil
}
func (s *Service) read(ctx context.Context, k repository.Key, node, op, revision string, out any) error {
	var n model.Dingospeed
	if err := s.DB.WithContext(ctx).Where("instance_id = ?", node).Take(&n).Error; err != nil {
		return fail(503, "source node unavailable")
	}
	if time.Since(n.UpdatedAt) > 5*time.Minute {
		return fail(503, "source node offline; comparison unavailable")
	}
	q := url.Values{"repo": {k.Repo}}
	if revision != "" {
		q.Set("revision", revision)
		q.Set("verify", "sha256")
	}
	endpoint := "http://" + net.JoinHostPort(n.Host, strconv.Itoa(int(n.Port))) + "/api/repositories/" + url.PathEscape(k.RepoType) + "/" + url.PathEscape(k.Namespace) + "/" + op + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return fail(503, "source content unreadable: "+err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fail(503, fmt.Sprintf("source content unreadable (HTTP %d); cannot compare", resp.StatusCode))
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 32<<20))
	if err = dec.Decode(out); err != nil {
		return fail(502, "incomplete source response")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return fail(502, "incomplete or oversized source response")
	}
	return nil
}

type Snapshot struct {
	repository.Key
	Revision        string `json:"revision"`
	Commit          string `json:"commit"`
	Files           []File `json:"files"`
	Complete        bool   `json:"complete"`
	Verified        bool   `json:"verified"`
	ContentVerified bool   `json:"contentVerified"`
	FileCount       int    `json:"fileCount"`
}

func validateFiles(files []File) error {
	seen := map[string]bool{}
	for _, f := range files {
		k := repository.Key{Namespace: "local", RepoType: "models", Repo: f.Path}
		if k.Validate() != nil || len(f.SHA256) != 64 || f.Size < 0 {
			return fail(400, "invalid file identity")
		}
		if _, err := hex.DecodeString(f.SHA256); err != nil || f.SHA256 != strings.ToLower(f.SHA256) {
			return fail(400, "invalid SHA256")
		}
		if seen[f.Path] {
			return fail(400, "duplicate file path")
		}
		seen[f.Path] = true
	}
	for p := range seen {
		parts := strings.Split(p, "/")
		for i := 1; i < len(parts); i++ {
			if seen[strings.Join(parts[:i], "/")] {
				return fail(400, "file/folder path conflict")
			}
		}
	}
	return nil
}
func (s *Service) snapshot(ctx context.Context, k repository.Key, node, revision string) (Snapshot, error) {
	var snap Snapshot
	if revision == "" {
		return snap, fail(400, "source revision required")
	}
	if err := s.read(ctx, k, node, "snapshot", revision, &snap); err != nil {
		return snap, err
	}
	if snap.Key != k || snap.Revision != revision || !snap.Complete || !snap.Verified || !snap.ContentVerified || snap.Commit == "" || snap.Files == nil || snap.FileCount != len(snap.Files) {
		return snap, fail(502, "source manifest is incomplete or unverified; cannot compare")
	}
	if err := validateFiles(snap.Files); err != nil {
		return snap, fail(502, err.Error())
	}
	return snap, nil
}
func (s *Service) commit(ctx context.Context, r Request) (any, error) {
	// Revision names use the same single-segment rules as repository names.
	k := r.Key
	k.Repo = r.Revision
	if k.Validate() != nil || strings.Contains(r.Revision, "/") || r.RequestID == "" || len(r.RequestID) > 128 {
		return nil, fail(400, "revision and request ID are required")
	}
	receiptID := hash([]string{hash(r.Key), r.Revision, r.Actor, r.RequestID})
	digest := hash(r)
	lookup := func(db *gorm.DB) (any, error) {
		var row Receipt
		err := db.Where("id = ?", receiptID).Take(&row).Error
		if err != nil {
			return nil, err
		}
		if row.Digest != digest {
			return nil, fail(409, "request ID already used for a different preview")
		}
		var out any
		err = json.Unmarshal([]byte(row.Result), &out)
		return out, err
	}
	if out, err := lookup(s.DB.WithContext(ctx)); err == nil {
		return out, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	current, err := s.get(ctx, r)
	if err != nil {
		var e *Failure
		if !errors.As(err, &e) || e.Status != 404 {
			return nil, err
		}
		if r.BaseVersion != 0 {
			return nil, fail(409, "official revision changed; keep draft and compare again")
		}
	}
	if current.Version != r.BaseVersion {
		return nil, fail(409, "official revision changed; keep draft and compare again")
	}
	files := map[string]File{}
	for _, f := range current.Files {
		files[f.Path] = f
	}
	seen := map[string]bool{}
	sources := map[string]Snapshot{}
	for _, c := range r.Changes {
		if seen[c.Path] {
			return nil, fail(400, "duplicate change path")
		}
		seen[c.Path] = true
		if c.File == nil {
			if _, ok := files[c.Path]; !ok {
				return nil, fail(400, "delete path not in official definition")
			}
			delete(files, c.Path)
			continue
		}
		if c.Path != c.File.Path || c.Source == nil {
			return nil, fail(400, "selected file requires an explicit source")
		}
		sk := hash(c.Source)
		snap, ok := sources[sk]
		if !ok {
			snap, err = s.snapshot(ctx, r.Key, c.Source.Node, c.Source.Commit)
			if err != nil {
				return nil, err
			}
			if snap.Commit != c.Source.Commit {
				return nil, fail(409, "source changed; compare again")
			}
			sources[sk] = snap
		}
		found := false
		for _, f := range snap.Files {
			if f == *c.File {
				found = true
				break
			}
		}
		if !found {
			return nil, fail(409, "selected source content changed; compare again")
		}
		files[c.Path] = *c.File
	}
	final := []File{}
	for _, f := range files {
		final = append(final, f)
	}
	sort.Slice(final, func(i, j int) bool { return final[i].Path < final[j].Path })
	if err = validateFiles(final); err != nil {
		return nil, err
	}
	manifest, _ := json.Marshal(final)
	next := Definition{ID: hash([]string{hash(r.Key), r.Revision}), Repository: hash(r.Key), Revision: r.Revision, Version: r.BaseVersion + 1, Manifest: string(manifest), Actor: r.Actor, UpdatedAt: time.Now().UTC()}
	out := View{Definition: next, Files: final}
	encoded, _ := json.Marshal(out)
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if r.BaseVersion == 0 {
			if err := tx.Create(&next).Error; err != nil {
				var duplicate *dbmysql.MySQLError
				if errors.Is(err, gorm.ErrDuplicatedKey) || errors.As(err, &duplicate) && duplicate.Number == 1062 {
					return fail(409, "official revision already created; keep draft and compare again")
				}
				return err
			}
		} else {
			res := tx.Model(&Definition{}).Where("id = ? AND version = ?", next.ID, r.BaseVersion).Updates(map[string]any{"version": next.Version, "manifest": next.Manifest, "actor": next.Actor, "updated_at": next.UpdatedAt})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return fail(409, "official revision changed; keep draft and compare again")
			}
		}
		return tx.Create(&Receipt{ID: receiptID, Digest: digest, Result: string(encoded)}).Error
	})
	if err != nil {
		if prior, e := lookup(s.DB.WithContext(ctx)); e == nil {
			return prior, nil
		}
		return nil, err
	}
	return out, nil
}
