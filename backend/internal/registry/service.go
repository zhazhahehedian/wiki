package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var versionPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._+-]{0,63}$`)
var toolPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

type Service struct {
	repo    Repository
	storage ports.ObjectStorage
}

func New(repo Repository, storage ports.ObjectStorage) *Service { return &Service{repo, storage} }
func (s *Service) List(ctx context.Context, user string, f Filter) (Page, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 100000 || len(f.Search) > 200 || utf8.RuneCountInString(f.Department) > 120 || (f.Type != "" && f.Type != "mcp" && f.Type != "skill") || (f.Status != "" && f.Status != "draft" && f.Status != "in_review" && f.Status != "published" && f.Status != "offline") {
		return Page{}, ErrInvalid
	}
	owner, e := s.repo.Owner(ctx, user)
	if e != nil {
		return Page{}, e
	}
	return s.repo.List(ctx, owner, f)
}
func (s *Service) Get(ctx context.Context, user, slug string) (Detail, error) {
	return s.repo.Read(ctx, user, slug, false)
}
func validate(r *SaveRequest) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.Department = strings.TrimSpace(r.Department)
	if (r.Slug == "new" || r.Slug == "create-skill") || !slugPattern.MatchString(r.Slug) || len(r.Slug) > 80 || (r.Type != "mcp" && r.Type != "skill") || r.Name == "" || utf8.RuneCountInString(r.Name) > 120 || r.Description == "" || utf8.RuneCountInString(r.Description) > 10000 || utf8.RuneCountInString(r.Department) > 120 || !versionPattern.MatchString(r.Version) || utf8.RuneCountInString(r.Changelog) > 10000 || r.Revision < 0 {
		return ErrInvalid
	}
	switch r.Visibility {
	case "org":
	case "department":
		if r.Department == "" {
			return ErrInvalid
		}
	case "allowlist":
		if len(r.Allowlist) == 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if len(r.Allowlist) > 200 || (r.Visibility != "allowlist" && len(r.Allowlist) > 0) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range r.Allowlist {
		if id == "" || len(id) > 128 || strings.ContainsAny(id, " \t\r\n") || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	if r.Type == "skill" {
		if r.MCPEndpoint != "" || r.MCPTransport != "" || r.MCPAuthScheme != "" || len(r.Tools) > 0 {
			return ErrInvalid
		}
		return nil
	}
	u, e := url.Parse(r.MCPEndpoint)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" || len(r.MCPEndpoint) > 2048 {
		return ErrInvalid
	}
	if r.MCPTransport != "streamable-http" || (r.MCPAuthScheme != "none" && r.MCPAuthScheme != "bearer") || len(r.Tools) > 100 {
		return ErrInvalid
	}
	seen = map[string]bool{}
	for _, tool := range r.Tools {
		var schema map[string]any
		if !toolPattern.MatchString(tool.Name) || seen[tool.Name] || len(tool.Description) > 4000 || len(tool.InputSchema) > 32000 || json.Unmarshal(tool.InputSchema, &schema) != nil || schema == nil || schema["type"] != "object" {
			return ErrInvalid
		}
		seen[tool.Name] = true
	}
	return nil
}
func (s *Service) Save(ctx context.Context, user, slug string, r SaveRequest, files []File) (Detail, error) {
	if e := validate(&r); e != nil {
		return Detail{}, e
	}
	owner, e := s.repo.Owner(ctx, user)
	if e != nil {
		return Detail{}, e
	}
	v := Version{Version: r.Version, Changelog: r.Changelog, MCPEndpoint: r.MCPEndpoint, MCPTransport: r.MCPTransport, MCPAuthScheme: r.MCPAuthScheme, Tools: r.Tools}
	var old Detail
	if slug != "" {
		old, e = s.repo.Get(ctx, owner, slug)
		if e != nil {
			return Detail{}, e
		}
		if old.Revision != r.Revision || old.Status != "draft" || old.Type != r.Type || old.Slug != r.Slug {
			return Detail{}, ErrConflict
		}
		for _, previous := range old.Versions {
			if previous.Version == r.Version {
				if previous.ID != old.DraftVersionID || previous.PublishedAt != nil {
					return Detail{}, ErrConflict
				}
				v.BundleKey = previous.BundleKey
			}
		}
	} else if r.Revision != 0 {
		return Detail{}, ErrConflict
	}
	uploaded := ""
	if r.Type == "skill" {
		if len(files) > 0 {
			bundle, err := PackSkill(r.Slug, files)
			if err != nil {
				return Detail{}, err
			}
			uploaded = "registry/" + uuid.NewString() + ".zip"
			v.BundleKey = uploaded
			if err = s.storage.Put(ctx, uploaded, bytes.NewReader(bundle), int64(len(bundle)), "application/zip"); err != nil {
				s.cleanupBundle(uploaded)
				return Detail{}, fmt.Errorf("store skill bundle: %w", err)
			}
		}
		if v.BundleKey == "" {
			return Detail{}, ErrInvalid
		}
		v.SkillManifest, _ = json.Marshal(map[string]string{"name": r.Name, "description": r.Description})
	} else if len(files) > 0 {
		return Detail{}, ErrInvalid
	}
	saved, e := s.repo.Save(ctx, owner, slug, r, v)
	if e != nil {
		if uploaded != "" && !errors.Is(e, ErrCommitUnknown) {
			s.cleanupBundle(uploaded)
		}
		return Detail{}, e
	}
	if uploaded != "" {
		for _, previous := range old.Versions {
			if previous.ID == old.DraftVersionID && previous.Version == r.Version && previous.BundleKey != "" {
				s.cleanupBundle(previous.BundleKey)
			}
		}
	}
	return saved, nil
}

// A commit error may be ambiguous. Never delete an object unless a fresh query
// proves that no committed version references it. Failures leave it for recovery.
func (s *Service) cleanupBundle(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	used, e := s.repo.BundleReferenced(ctx, key)
	if e != nil {
		log.Print("registry: bundle reference check failed; cleanup deferred")
		return
	}
	if !used {
		if e = s.storage.Delete(ctx, key); e != nil {
			log.Print("registry: unreferenced bundle cleanup failed")
		}
	}
}
func (s *Service) Download(ctx context.Context, user, slug, versionID string) (io.ReadCloser, error) {
	detail, e := s.Get(ctx, user, slug)
	if e != nil {
		return nil, e
	}
	for _, v := range detail.Versions {
		if v.ID == versionID && v.BundleKey != "" {
			return s.storage.Get(ctx, v.BundleKey)
		}
	}
	return nil, ErrNotFound
}
