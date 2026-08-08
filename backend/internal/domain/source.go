package domain

// ResourceType identifies the kind of remote Feishu resource.
type ResourceType string

const (
	ResourceDocx    ResourceType = "docx"
	ResourceSheet   ResourceType = "sheet"
	ResourceBitable ResourceType = "bitable"
	ResourceWiki    ResourceType = "wiki"
)

// ResourceRef is a parsed, stable reference to a Feishu resource.
type ResourceRef struct {
	Type        ResourceType `json:"type"`
	Tenant      string       `json:"tenant"`
	Token       string       `json:"token"`
	TableID     string       `json:"table_id,omitempty"`
	ViewID      string       `json:"view_id,omitempty"`
	SheetID     string       `json:"sheet_id,omitempty"`
	OriginalURL string       `json:"original_url"`
	Identity    string       `json:"identity"`
}

// CanonicalDocument is the provider-independent Markdown snapshot consumed by
// the ingestion pipeline.
type CanonicalDocument struct {
	Title          string         `json:"title"`
	Markdown       string         `json:"markdown"`
	RemoteRevision string         `json:"remote_revision"`
	SourceMetadata map[string]any `json:"source_metadata,omitempty"`
	SourceURL      string         `json:"source_url"`
}
