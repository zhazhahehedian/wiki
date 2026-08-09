package domain

import "time"

type DocStatus string

const (
	StatusPending   DocStatus = "pending"
	StatusParsing   DocStatus = "parsing"
	StatusChunking  DocStatus = "chunking"
	StatusEmbedding DocStatus = "embedding"
	StatusReady     DocStatus = "ready"
	StatusFailed    DocStatus = "failed"
)

type Document struct {
	ID             string         `json:"id"`
	KBID           string         `json:"kb_id"`
	SourceType     string         `json:"source_type"`
	SourceRef      string         `json:"source_ref"`
	Title          string         `json:"title"`
	MimeType       string         `json:"mime_type"`
	Bytes          int64          `json:"bytes"`
	Checksum       string         `json:"checksum"`
	Status         DocStatus      `json:"status"`
	ErrorMessage   *string        `json:"error_message,omitempty"`
	SourceURL      *string        `json:"source_url,omitempty"`
	RemoteRevision *string        `json:"remote_revision,omitempty"`
	SyncStatus     string         `json:"sync_status,omitempty"`
	LastSyncError  *string        `json:"last_sync_error,omitempty"`
	LastSyncedAt   *time.Time     `json:"last_synced_at,omitempty"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}
