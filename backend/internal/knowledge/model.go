// Package knowledge implements the document service; it never generates answers.
package knowledge

import (
	"context"
	"errors"
	"time"
)

const MaxUpload = 20 << 20
const MaxText = 2 << 20

var (
	ErrInvalid         = errors.New("invalid_request")
	ErrForbidden       = errors.New("knowledge_forbidden")
	ErrUnauthenticated = errors.New("invalid_credential")
	ErrNotFound        = errors.New("knowledge_not_found")
	ErrConflict        = errors.New("knowledge_conflict")
	ErrNotReady        = errors.New("knowledge_not_ready")
	ErrUnavailable     = errors.New("knowledge_unavailable")
	ErrLimited         = errors.New("knowledge_rate_limited")
)

type Principal struct {
	UserID        string
	ApplicationID string
	CredentialID  string
}
type KB struct {
	HasActive         bool     `json:"-"`
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	Visibility        string   `json:"visibility"`
	Department        string   `json:"department"`
	Members           []string `json:"members"`
	Manage            bool     `json:"manage"`
	ApplicationShared bool     `json:"application_shared"`
	Model             string   `json:"model"`
	Dim               int      `json:"dim"`
}
type Locator struct {
	Page         int    `json:"page,omitempty"`
	Paragraph    int    `json:"paragraph,omitempty"`
	ParagraphEnd int    `json:"paragraph_end,omitempty"`
	Line         int    `json:"line,omitempty"`
	LineEnd      int    `json:"line_end,omitempty"`
	Heading      string `json:"heading,omitempty"`
	Part         int    `json:"part,omitempty"`
}
type Block struct {
	Content string  `json:"content"`
	Locator Locator `json:"locator"`
}
type Diagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Page     int    `json:"page,omitempty"`
	Blocking bool   `json:"blocking"`
}
type Preview struct {
	Blocks      []Block      `json:"blocks"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Format      string       `json:"format"`
}
type Version struct {
	ID         string  `json:"id"`
	DocumentID string  `json:"document_id"`
	KBID       string  `json:"knowledge_base_id"`
	Title      string  `json:"title"`
	Status     string  `json:"status"`
	Error      string  `json:"error"`
	Preview    Preview `json:"preview"`
	Model      string  `json:"model"`
	Dim        int     `json:"dim"`
	ObjectKey  string  `json:"-"`
	Checksum   string  `json:"checksum"`
}
type Document struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Error          string `json:"error"`
	ActiveVersion  string `json:"active_version_id"`
	PendingVersion string `json:"pending_version_id"`
}
type SearchRequest struct {
	KBIDs []string `json:"knowledge_base_ids"`
	Query string   `json:"query"`
	TopK  int      `json:"top_k"`
}
type Match struct {
	KBID       string  `json:"knowledge_base_id"`
	DocumentID string  `json:"document_id"`
	VersionID  string  `json:"document_version_id"`
	ChunkID    string  `json:"chunk_id"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Score      float32 `json:"score"`
	Locator    Locator `json:"locator"`
}
type Application struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Enabled    bool       `json:"enabled"`
	KBIDs      []string   `json:"knowledge_base_ids"`
	Prefix     string     `json:"credential_prefix"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}
type ApplicationInput struct {
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	KBIDs   []string `json:"knowledge_base_ids"`
}
type Credential struct {
	Token     string    `json:"token"`
	Prefix    string    `json:"prefix"`
	ExpiresAt time.Time `json:"expires_at"`
}
type AuditEntry struct {
	KBIDs         []string  `json:"knowledge_base_ids"`
	RequestID     string    `json:"request_id"`
	ApplicationID string    `json:"application_id"`
	Status        string    `json:"status"`
	ResultCount   int       `json:"result_count"`
	DurationMS    int64     `json:"duration_ms"`
	CreatedAt     time.Time `json:"created_at"`
}
type Repository interface {
	ListKB(context.Context, Principal) ([]KB, error)
	SaveKB(context.Context, Principal, string, KB, string, int) (KB, error)
	DeleteKB(context.Context, Principal, string) error
	Documents(context.Context, Principal, string) ([]Document, error)
	Stage(context.Context, Principal, string, string, Version, int64) (Document, error)
	DeleteDocument(context.Context, Principal, string) error
	Preview(context.Context, Principal, string) (Version, error)
	Retry(context.Context, Principal, string) error
	LoadVersion(context.Context, string) (Version, error)
	RecordPreview(context.Context, string, Preview) error
	Publish(context.Context, Version, []Block, [][]float32) error
	Fail(context.Context, string, string) error
	Authorize(context.Context, Principal, []string) ([]KB, error)
	Search(context.Context, []string, []float32, int, float32) ([]Match, error)
	Recheck(context.Context, Principal, []string, []Match) ([]Match, error)
	Authenticate(context.Context, []byte) (Principal, error)
	Applications(context.Context, Principal) ([]Application, error)
	SaveApplication(context.Context, Principal, string, ApplicationInput) (Application, error)
	Rotate(context.Context, Principal, string, []byte, string, time.Time) error
	Revoke(context.Context, Principal, string) error
	Audit(context.Context, Principal, []string, string, string, int, time.Duration) error
	AuditEntries(context.Context, Principal) ([]AuditEntry, error)
}
