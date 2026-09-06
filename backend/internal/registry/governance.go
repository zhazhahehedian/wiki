package registry

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type Identity struct {
	OpenID      string `json:"open_id"`
	IsAdmin     bool   `json:"is_admin"`
	Department  string `json:"department"`
	Revision    int64  `json:"revision"`
	DisplayName string `json:"display_name,omitempty"`
}
type PublishedMetadata struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Department  string   `json:"department"`
	Visibility  string   `json:"visibility"`
	Allowlist   []string `json:"allowlist,omitempty"`
}

// CanViewPublished is also the permission boundary for future credential grants.
func CanViewPublished(who Identity, owner string, live bool, meta PublishedMetadata) bool {
	if who.OpenID == "" || !live {
		return false
	}
	if who.IsAdmin || who.OpenID == owner {
		return true
	}
	switch meta.Visibility {
	case "org":
		return true
	case "department":
		return who.Department != "" && meta.Department != "" && who.Department == meta.Department
	case "allowlist":
		for _, id := range meta.Allowlist {
			if id == who.OpenID {
				return true
			}
		}
	}
	return false
}

type ActionRequest struct {
	Revision   int64    `json:"revision"`
	VersionID  string   `json:"version_id"`
	Reason     string   `json:"reason"`
	Visibility string   `json:"visibility,omitempty"`
	Department string   `json:"department,omitempty"`
	Allowlist  []string `json:"allowlist,omitempty"`
}
type AuditEntry struct {
	ID          string         `json:"id"`
	ActorOpenID string         `json:"actor_open_id"`
	Action      string         `json:"action"`
	TargetType  string         `json:"target_type"`
	TargetID    string         `json:"target_id"`
	Detail      map[string]any `json:"detail"`
	RequestID   string         `json:"request_id"`
	CreatedAt   time.Time      `json:"created_at"`
}
type AuditFilter struct {
	Action, Actor, Target string
	Limit, Offset         int32
}
type AuditPage struct {
	Items   []AuditEntry `json:"items"`
	HasMore bool         `json:"has_more"`
}
type ProfilePage struct {
	Items   []Identity `json:"items"`
	HasMore bool       `json:"has_more"`
}
type DepartmentRequest struct {
	Department string `json:"department"`
	Revision   int64  `json:"revision"`
	Reason     string `json:"reason"`
}
type auditContextKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, auditContextKey{}, id)
}
func RequestID(ctx context.Context) string { id, _ := ctx.Value(auditContextKey{}).(string); return id }
func (s *Service) Identity(ctx context.Context, user string) (Identity, error) {
	return s.repo.Identity(ctx, user)
}
func validFilter(f Filter) bool {
	return f.Limit > 0 && f.Limit <= 100 && f.Offset >= 0 && f.Offset <= 100000 && len(f.Search) <= 200 && utf8.RuneCountInString(f.Department) <= 120 && (f.Type == "" || f.Type == "mcp" || f.Type == "skill") && (f.Status == "" || f.Status == "draft" || f.Status == "in_review" || f.Status == "published" || f.Status == "offline")
}
func (s *Service) Catalog(ctx context.Context, user string, f Filter) (Page, error) {
	if !validFilter(f) {
		return Page{}, ErrInvalid
	}
	return s.repo.Catalog(ctx, user, f)
}
func (s *Service) Published(ctx context.Context, user, slug string) (Detail, error) {
	return s.repo.Read(ctx, user, slug, true)
}
func (s *Service) Reviews(ctx context.Context, user string, f Filter) (Page, error) {
	if !validFilter(f) || (f.Status != "" && f.Status != "in_review") {
		return Page{}, ErrInvalid
	}
	return s.repo.Reviews(ctx, user, f)
}
func (s *Service) Act(ctx context.Context, user, slug, action string, r ActionRequest) (Detail, error) {
	r.Reason = strings.TrimSpace(r.Reason)
	if r.Revision < 1 || utf8.RuneCountInString(r.Reason) > 2000 {
		return Detail{}, ErrInvalid
	}
	if _, e := uuid.Parse(r.VersionID); e != nil {
		return Detail{}, ErrInvalid
	}
	switch action {
	case "submit", "approve", "new-draft":
	case "reject", "offline", "force-publish", "visibility":
		if r.Reason == "" {
			return Detail{}, ErrInvalid
		}
	default:
		return Detail{}, ErrInvalid
	}
	if action == "visibility" {
		input := SaveRequest{Slug: "visibility", Type: "skill", Name: "visibility", Description: "visibility", Version: "1", Visibility: r.Visibility, Department: r.Department, Allowlist: r.Allowlist}
		if e := validate(&input); e != nil {
			return Detail{}, e
		}
		r.Department = input.Department
	} else if r.Visibility != "" || r.Department != "" || len(r.Allowlist) > 0 {
		return Detail{}, ErrInvalid
	}
	// Validate the exact candidate before entering the short SQL transaction. The
	// repository rechecks revision + version under lock, so concurrent edits fail.
	if action == "submit" || action == "approve" || action == "force-publish" {
		d, e := s.Get(ctx, user, slug)
		if e != nil {
			return Detail{}, e
		}
		who, e := s.Identity(ctx, user)
		if e != nil {
			return Detail{}, e
		}
		if (action == "submit" && d.OwnerOpenID != who.OpenID) || (action != "submit" && !who.IsAdmin) {
			return Detail{}, ErrForbidden
		}
		if d.Revision != r.Revision {
			return Detail{}, ErrConflict
		}
		candidate := d.DraftVersionID
		if candidate == "" && d.Status == "offline" {
			candidate = d.CurrentVersionID
		}
		if candidate != r.VersionID {
			return Detail{}, ErrConflict
		}
		var version *Version
		for i := range d.Versions {
			if d.Versions[i].ID == candidate {
				version = &d.Versions[i]
				break
			}
		}
		if version == nil {
			return Detail{}, ErrConflict
		}
		if d.Type == "skill" {
			if _, e = s.skillFiles(ctx, d.Slug, *version, true); e != nil {
				// A concurrent edit may replace and retire the old bundle while
				// validation is reading it. Report the stale review as a conflict.
				if latest, readErr := s.Get(ctx, user, slug); readErr == nil && latest.Revision != r.Revision {
					return Detail{}, ErrConflict
				}
				if errors.Is(e, ErrInvalid) {
					return Detail{}, ErrSkillPublication
				}
				return Detail{}, e
			}
		}
	}
	return s.repo.Act(ctx, user, slug, action, r)
}
func (s *Service) Audit(ctx context.Context, user string, f AuditFilter) (AuditPage, error) {
	if f.Limit < 1 || f.Limit > 100 || f.Offset < 0 || f.Offset > 100000 || len(f.Action) > 64 || len(f.Actor) > 128 || len(f.Target) > 128 {
		return AuditPage{}, ErrInvalid
	}
	return s.repo.Audit(ctx, user, f)
}
func (s *Service) Profiles(ctx context.Context, user string, f Filter) (ProfilePage, error) {
	if !validFilter(f) {
		return ProfilePage{}, ErrInvalid
	}
	return s.repo.Profiles(ctx, user, f)
}
func (s *Service) SetDepartment(ctx context.Context, user, openID string, r DepartmentRequest) (Identity, error) {
	r.Department = strings.TrimSpace(r.Department)
	r.Reason = strings.TrimSpace(r.Reason)
	if openID == "" || len(openID) > 128 || r.Revision < 0 || utf8.RuneCountInString(r.Department) > 120 || r.Reason == "" || utf8.RuneCountInString(r.Reason) > 2000 {
		return Identity{}, ErrInvalid
	}
	return s.repo.SetDepartment(ctx, user, openID, r)
}
