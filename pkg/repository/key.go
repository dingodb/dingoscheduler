// Package repository defines the namespace identity shared by scheduler boundaries.
package repository

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Key is the HTTP identity; SQL and RPC retain legacy org/repo coordinates.
type Key struct {
	Namespace string `json:"namespace"`
	RepoType  string `json:"repoType"`
	Repo      string `json:"repo"`
}

// FromWire decodes unchanged remote identities and explicitly encoded hosted ones.
func FromWire(repoType, org, repo string) (Key, error) {
	k := Key{Namespace: "huggingface", RepoType: repoType, Repo: repo}
	if strings.HasPrefix(org, "dingo-local/") {
		k.Namespace = strings.TrimPrefix(org, "dingo-local/")
	} else if strings.HasPrefix(org, "modelscope/") {
		k.Namespace = "modelscope"
		k.Repo = strings.TrimPrefix(org, "modelscope/") + "/" + repo
	} else if org != "" {
		k.Repo = org + "/" + repo
	}
	return k, k.Validate()
}

// Storage leaves HF owner/repo exactly as old SQL and old peers expect.
func (k Key) Storage() (org, repo string, err error) {
	if err = k.Validate(); err != nil {
		return
	}
	repo = k.Repo
	switch k.Namespace {
	case "huggingface":
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 {
			org, repo = parts[0], parts[1]
		}
	case "modelscope":
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) != 2 {
			return "", "", fmt.Errorf("ModelScope needs owner/repo")
		}
		org, repo = "modelscope/"+parts[0], parts[1]
	default:
		org = "dingo-local/" + k.Namespace
	}
	if k.Namespace != "huggingface" && (len(org) > 100 || len(repo) > 100 || len(org)+1+len(repo) > 100) {
		err = fmt.Errorf("repository identity exceeds unchanged SQL VARCHAR(100) capacity")
	}
	return
}

func (k Key) Validate() error {
	if k.RepoType != "models" && k.RepoType != "datasets" && k.RepoType != "spaces" {
		return fmt.Errorf("invalid repository type")
	}
	if err := ValidatePath(k.Namespace, false); err != nil {
		return fmt.Errorf("invalid namespace: %w", err)
	}
	if len(k.Namespace) > 255 || len(k.Repo) > 1024 {
		return fmt.Errorf("namespace or repository exceeds storage identity limit")
	}
	return ValidatePath(k.Repo, true)
}

func ValidatePath(value string, multiple bool) error {
	if value == "" || (!multiple && strings.Contains(value, "/")) || (multiple && len(value) > 1024) {
		return fmt.Errorf("empty or invalid path")
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." || !utf8.ValidString(part) || len(part) > 255 || strings.TrimSpace(part) != part || strings.HasSuffix(part, ".") {
			return fmt.Errorf("invalid path segment")
		}
		for _, ch := range part {
			if unicode.IsControl(ch) || strings.ContainsRune(`\<>:"|?*`, ch) {
				return fmt.Errorf("unsafe path character")
			}
		}
		base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
			return fmt.Errorf("reserved platform path segment")
		}
	}
	return nil
}

// LockKey uses a structured encoding: multi-level repositories cannot collide
// with later identity components such as file paths, job types or content IDs.
func (k Key) LockKey(operation string, fields ...string) string {
	parts := append([]string{operation, k.Namespace, k.RepoType, k.Repo}, fields...)
	encoded, _ := json.Marshal(parts)
	return string(encoded)
}

func (k Key) ID() string { return k.Namespace + "/" + k.Repo }

func (k Key) DefaultRevision() string {
	if k.Namespace == "modelscope" {
		return "master"
	}
	return "main"
}

// OperationURI addresses DingoSpeed's own API, never the upstream HF protocol.
func (k Key) OperationURI(operation, revision, path string) (string, error) {
	if err := k.Validate(); err != nil {
		return "", err
	}
	switch operation {
	case "metadata", "snapshot", "file", "files", "tree", "archive":
	default:
		return "", fmt.Errorf("unsupported repository operation")
	}
	if revision == "" {
		revision = k.DefaultRevision()
	}
	if err := ValidatePath(revision, false); err != nil {
		return "", err
	}
	if path != "" {
		if err := ValidatePath(path, true); err != nil {
			return "", err
		}
	}
	q := url.Values{"repo": {k.Repo}, "revision": {revision}}
	if path != "" {
		q.Set("path", path)
	}
	return "/api/repositories/" + url.PathEscape(k.RepoType) + "/" + url.PathEscape(k.Namespace) + "/" + operation + "?" + q.Encode(), nil
}
