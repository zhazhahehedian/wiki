package registry

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrCommitUnknown    = errors.New("registry commit outcome unknown")
	ErrInvalid          = errors.New("invalid capability")
	ErrNotFound         = errors.New("capability not found")
	ErrConflict         = errors.New("capability revision or version conflict")
	ErrSkillPublication = errors.New("invalid skill publication format")
	ErrForbidden        = errors.New("registry permission denied")
	ErrIdentity         = errors.New("feishu identity required")
)

type Capability struct {
	ID               string    `json:"id"`
	Slug             string    `json:"slug"`
	Type             string    `json:"type"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	OwnerOpenID      string    `json:"owner_open_id"`
	Department       string    `json:"department"`
	Status           string    `json:"status"`
	IsLive           bool      `json:"is_live"`
	ReviewReason     string    `json:"review_reason,omitempty"`
	Visibility       string    `json:"visibility"`
	CurrentVersionID string    `json:"current_version_id,omitempty"`
	DraftVersionID   string    `json:"draft_version_id,omitempty"`
	Revision         int64     `json:"revision"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}
type Version struct {
	ID            string          `json:"id"`
	Version       string          `json:"version"`
	Changelog     string          `json:"changelog"`
	CreatedBy     string          `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
	MCPEndpoint   string          `json:"mcp_endpoint,omitempty"`
	MCPTransport  string          `json:"mcp_transport,omitempty"`
	MCPAuthScheme string          `json:"mcp_auth_scheme,omitempty"`
	Tools         []Tool          `json:"tools,omitempty"`
	SkillManifest json.RawMessage `json:"skill_manifest,omitempty"`
	BundleKey     string          `json:"-"`
	PublishedAt   *time.Time      `json:"published_at,omitempty"`
	HasBundle     bool            `json:"has_bundle"`
}
type Detail struct {
	Capability
	Versions  []Version          `json:"versions"`
	Allowlist []string           `json:"allowlist,omitempty"`
	IsOwner   bool               `json:"is_owner"`
	IsAdmin   bool               `json:"is_admin"`
	Published *PublishedMetadata `json:"published,omitempty"`
}
type SaveRequest struct {
	Slug          string   `json:"slug"`
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Department    string   `json:"department"`
	Visibility    string   `json:"visibility"`
	Allowlist     []string `json:"allowlist"`
	Revision      int64    `json:"revision"`
	Version       string   `json:"version"`
	Changelog     string   `json:"changelog"`
	MCPEndpoint   string   `json:"mcp_endpoint"`
	MCPTransport  string   `json:"mcp_transport"`
	MCPAuthScheme string   `json:"mcp_auth_scheme"`
	Tools         []Tool   `json:"tools"`
}
type File struct {
	Name string
	Data []byte
}
type Filter struct {
	Type, Status, Department, Search string
	Limit, Offset                    int32
}
type Page struct {
	Items   []Capability `json:"items"`
	HasMore bool         `json:"has_more"`
}
type Repository interface {
	Owner(context.Context, string) (string, error)
	List(context.Context, string, Filter) (Page, error)
	Get(context.Context, string, string) (Detail, error)
	Save(context.Context, string, string, SaveRequest, Version) (Detail, error)
	BundleReferenced(context.Context, string) (bool, error)
	Identity(context.Context, string) (Identity, error)
	Read(context.Context, string, string, bool) (Detail, error)
	Catalog(context.Context, string, Filter) (Page, error)
	Reviews(context.Context, string, Filter) (Page, error)
	Act(context.Context, string, string, string, ActionRequest) (Detail, error)
	Audit(context.Context, string, AuditFilter) (AuditPage, error)
	Profiles(context.Context, string, Filter) (ProfilePage, error)
	SetDepartment(context.Context, string, string, DepartmentRequest) (Identity, error)
}
