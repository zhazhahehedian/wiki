package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	repo      Repository
	storage   ports.ObjectStorage
	directory SelectionValidator
	previews  chan struct{}
	uploads   *uploads
}

type SelectionValidator interface {
	ValidateSelection(context.Context, string, string, []string) (string, error)
}

func New(repo Repository, storage ports.ObjectStorage) *Service {
	return &Service{repo: repo, storage: storage, previews: make(chan struct{}, maxConcurrentPreviews)}
}
func (s *Service) WithDirectory(d SelectionValidator) *Service { s.directory = d; return s }
func (s *Service) validateDirectory(ctx context.Context, user string, r *SaveRequest) error {
	if s.directory == nil {
		return nil
	}
	// Existing string-only departments remain readable. New saves must select an ID.
	if r.DepartmentID == "" && (r.Department != "" || r.Visibility == "department") {
		return ErrInvalid
	}
	name, e := s.directory.ValidateSelection(ctx, user, r.DepartmentID, r.Allowlist)
	if e != nil {
		return e
	}
	r.Department = name
	return nil
}
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
	if r.MCPEndpoint != "" {
		u, e := url.Parse(r.MCPEndpoint)
		if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" || len(r.MCPEndpoint) > 2048 {
			return ErrInvalid
		}
	}
	if (r.MCPTransport != "streamable-http" && r.MCPTransport != "stdio") || (r.MCPAuthScheme != "none" && r.MCPAuthScheme != "bearer") || len(r.Tools) > 100 {
		return ErrInvalid
	}
	if r.MCPTransport == "stdio" && (r.MCPEndpoint != "" || r.MCPAuthScheme != "none") {
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
	if e := s.validateDirectory(ctx, user, &r); e != nil {
		return Detail{}, e
	}
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
				v.BundleKey, v.BundleSHA256, v.BundleSize, v.BundleFindings = previous.BundleKey, previous.BundleSHA256, previous.BundleSize, previous.BundleFindings
			}
		}
	} else if r.Revision != 0 {
		return Detail{}, ErrConflict
	}
	uploaded := ""
	var upload *BundleUpload
	if r.BundleUploadID != "" {
		if r.Type != "mcp" || len(files) > 0 {
			return Detail{}, ErrInvalid
		}
		u, err := s.readyUpload(ctx, user, r.BundleUploadID)
		if err != nil {
			return Detail{}, err
		}
		upload = &u
		v.BundleKey, v.BundleSHA256, v.BundleSize, v.BundleFindings = u.ObjectKey, u.BundleSHA256, u.Size, u.BundleFindings
	} else if len(files) > 0 {
		var bundle []byte
		var err error
		if r.Type == "skill" {
			bundle, err = PackSkill(r.Slug, files)
		} else {
			bundle, v.BundleFindings, err = validateMCPBundle(ctx, files)
		}
		if err != nil {
			return Detail{}, err
		}
		uploaded = bundlePrefix + uuid.NewString() + ".zip"
		sum := sha256.Sum256(bundle)
		v.BundleKey, v.BundleSHA256, v.BundleSize = uploaded, hex.EncodeToString(sum[:]), int64(len(bundle))
		if err = s.storage.Put(ctx, uploaded, bytes.NewReader(bundle), int64(len(bundle)), "application/zip"); err != nil {
			s.cleanupBundle(uploaded)
			return Detail{}, fmt.Errorf("store capability bundle: %w", err)
		}
	}
	if r.Type == "skill" {
		if v.BundleKey == "" {
			return Detail{}, ErrInvalid
		}
		v.SkillManifest, _ = json.Marshal(map[string]string{"name": r.Name, "description": r.Description})
	} else if v.BundleKey == "" && v.MCPEndpoint == "" {
		return Detail{}, ErrMCPPackage
	}
	// A failed save keeps a part upload so the user can retry without re-sending it.
	saved, e := s.repo.Save(ctx, owner, slug, r, v)
	if e != nil {
		if uploaded != "" && !errors.Is(e, ErrCommitUnknown) {
			s.cleanupBundle(uploaded)
		}
		return Detail{}, e
	}
	if upload != nil {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := s.uploads.repo.DeleteUpload(c, user, upload.ID); err != nil {
			log.Print("registry: consumed bundle upload record cleanup failed; it expires later")
		}
		cancel()
	}
	if uploaded != "" || upload != nil {
		for _, previous := range old.Versions {
			if previous.ID == old.DraftVersionID && previous.Version == r.Version && previous.BundleKey != "" && previous.BundleKey != v.BundleKey {
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

func (s *Service) Downloaders(ctx context.Context, user, slug string, f Filter) (DownloaderPage, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 100000 {
		return DownloaderPage{}, ErrInvalid
	}
	return s.repo.Downloaders(ctx, user, slug, f)
}

// Download copies one bundle into dst. The download audit is written only after
// every byte has been copied. prepare runs after the object is opened and
// before the first byte is written, so an HTTP caller can set headers without
// committing a success status. A short copy or a read/write error returns
// before RecordDownload. An audit failure after a complete copy is ErrAudit.
func (s *Service) Download(ctx context.Context, user, slug, versionID string, dst io.Writer, prepare func(Version) error) (Version, error) {
	detail, e := s.Get(ctx, user, slug)
	if e != nil {
		return Version{}, e
	}
	for _, v := range detail.Versions {
		if v.ID != versionID || v.BundleKey == "" {
			continue
		}
		body, e := s.storage.Get(ctx, v.BundleKey)
		if e != nil {
			return Version{}, e
		}
		defer body.Close()
		if prepare != nil {
			if e = prepare(v); e != nil {
				return Version{}, e
			}
		}
		n, e := io.Copy(dst, body)
		if e != nil {
			return Version{}, e
		}
		if v.BundleSize > 0 && n != v.BundleSize {
			return Version{}, fmt.Errorf("incomplete bundle: %w", io.ErrUnexpectedEOF)
		}
		if e = s.repo.RecordDownload(ctx, user, detail.ID, v.ID); e != nil {
			return v, fmt.Errorf("%w: %w", ErrAudit, e)
		}
		return v, nil
	}
	return Version{}, ErrNotFound
}
