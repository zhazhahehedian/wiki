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
	ErrMCPPackage       = errors.New("invalid MCP package")
	ErrForbidden        = errors.New("registry permission denied")
	ErrIdentity         = errors.New("feishu identity required")
	ErrUploadQuota      = errors.New("too many unfinished bundle uploads")
	ErrUploadNotReady   = errors.New("bundle upload unfinished or expired")
)

type Capability struct {
	ID               string    `json:"id"`
	Slug             string    `json:"slug"`
	Type             string    `json:"type"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	OwnerOpenID      string    `json:"owner_open_id"`
	Department       string    `json:"department"`
	DepartmentID     string    `json:"department_id"`
	Status           string    `json:"status"`
	IsLive           bool      `json:"is_live"`
	ReviewReason     string    `json:"review_reason,omitempty"`
	Visibility       string    `json:"visibility"`
	CurrentVersionID string    `json:"current_version_id,omitempty"`
	PublishedVersion string    `json:"published_version,omitempty"`
	PublisherName    string    `json:"publisher_name,omitempty"`
	DraftVersionID   string    `json:"draft_version_id,omitempty"`
	DownloadCount    int64     `json:"download_count,omitempty"`
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
	BundleSHA256  string          `json:"bundle_sha256,omitempty"`
	BundleSize    int64           `json:"bundle_size,omitempty"`
	// BundleFindings are upload-time review hints, never an approval gate.
	BundleFindings []BundleFinding `json:"bundle_findings,omitempty"`
}
type Detail struct {
	Capability
	Versions     []Version          `json:"versions"`
	Allowlist    []string           `json:"allowlist,omitempty"`
	IsOwner      bool               `json:"is_owner"`
	IsAdmin      bool               `json:"is_admin"`
	CanReview    bool               `json:"can_review"`
	IsReviewView bool               `json:"is_review_view"`
	Published    *PublishedMetadata `json:"published,omitempty"`
}
type SaveRequest struct {
	Slug          string   `json:"slug"`
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Department    string   `json:"department"`
	DepartmentID  string   `json:"department_id"`
	Visibility    string   `json:"visibility"`
	Allowlist     []string `json:"allowlist"`
	Revision      int64    `json:"revision"`
	Version       string   `json:"version"`
	Changelog     string   `json:"changelog"`
	MCPEndpoint   string   `json:"mcp_endpoint"`
	MCPTransport  string   `json:"mcp_transport"`
	MCPAuthScheme string   `json:"mcp_auth_scheme"`
	Tools         []Tool   `json:"tools"`
	// BundleUploadID references a completed part upload instead of a file in
	// the request; MCP only.
	BundleUploadID string `json:"bundle_upload_id,omitempty"`
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
	RecordDownload(ctx context.Context, user, capabilityID, versionID string) error
	Identity(context.Context, string) (Identity, error)
	Read(context.Context, string, string, bool) (Detail, error)
	Catalog(context.Context, string, Filter) (Page, error)
	Reviews(context.Context, string, Filter) (Page, error)
	Act(context.Context, string, string, string, ActionRequest) (Detail, error)
	Audit(context.Context, string, AuditFilter) (AuditPage, error)
	Profiles(context.Context, string, Filter) (ProfilePage, error)
	SetDepartment(context.Context, string, string, DepartmentRequest) (Identity, error)
	SetReviewer(context.Context, string, string, ReviewerRequest) (Identity, error)
	Reviewers(context.Context, string, Filter) (ReviewerPage, error)
	Downloaders(ctx context.Context, user, slug string, f Filter) (DownloaderPage, error)
}

// Downloader aggregates audited downloads per user for owners and admins.
type Downloader struct {
	OpenID           string    `json:"open_id"`
	DisplayName      string    `json:"display_name"`
	Downloads        int64     `json:"downloads"`
	LastDownloadedAt time.Time `json:"last_downloaded_at"`
	LastVersionID    string    `json:"last_version_id"`
}
type DownloaderPage struct {
	Items   []Downloader `json:"items"`
	HasMore bool         `json:"has_more"`
}
