package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/domain"
)

const listDocumentsLimit = 50

// DocumentLister 是 service.Document 的窄接口（*service.Document 自动满足）。
type DocumentLister interface {
	ListByKB(ctx context.Context, kbID string, statusFilter *string, limit, offset int) ([]*domain.Document, int, error)
}

// ListDocuments 让 Agent 列出当前 KB 内的文档清单——回答
// “知识库里有哪些文档”这类向量检索无法回答的元问题。
type ListDocuments struct {
	docs DocumentLister
	kbID string
}

func NewListDocuments(docs DocumentLister, kbID string) *ListDocuments {
	return &ListDocuments{docs: docs, kbID: kbID}
}

func (t *ListDocuments) Name() string { return "list_documents" }

func (t *ListDocuments) Description() string {
	return "List the documents in the knowledge base with title, ingestion status, size and last update time. Use when the user asks what documents exist."
}

func (t *ListDocuments) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"status": {
				"type": "string",
				"enum": ["pending", "parsing", "chunking", "embedding", "ready", "failed"],
				"description": "Optional: only list documents with this ingestion status"
			}
		}
	}`)
}

type listDocumentsArgs struct {
	Status string `json:"status"`
}

var validDocStatuses = map[string]bool{
	"pending": true, "parsing": true, "chunking": true,
	"embedding": true, "ready": true, "failed": true,
}

func (t *ListDocuments) Invoke(ctx context.Context, argsJSON string) (string, error) {
	var args listDocumentsArgs
	if argsJSON != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	var statusFilter *string
	if s := strings.TrimSpace(args.Status); s != "" {
		if !validDocStatuses[s] {
			return "", fmt.Errorf("invalid status: %s", s)
		}
		statusFilter = &s
	}

	docs, total, err := t.docs.ListByKB(ctx, t.kbID, statusFilter, listDocumentsLimit, 0)
	if err != nil {
		return "", err
	}
	if len(docs) == 0 {
		return "The knowledge base has no documents matching the filter.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d document(s) in the knowledge base:\n", total)
	for i, d := range docs {
		fmt.Fprintf(&b, "%d. %s — status: %s, size: %s, updated: %s\n",
			i+1, d.Title, d.Status, humanBytes(d.Bytes), d.UpdatedAt.Format("2006-01-02 15:04"))
	}
	if total > len(docs) {
		fmt.Fprintf(&b, "(showing first %d of %d)\n", len(docs), total)
	}
	return b.String(), nil
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
