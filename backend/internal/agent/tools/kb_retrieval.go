package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

// KBRetrieval 让 Agent 检索当前会话所属 KB。kbID 闭包注入，不暴露给 LLM。
// onRetrieval 在每次成功检索后触发（citation 收集 + retrieval SSE 推送由调用方实现）。
type KBRetrieval struct {
	retrieval   *service.Retrieval
	kbID        string
	onRetrieval func(ctx context.Context, r *service.RetrievalResult) error
}

func NewKBRetrieval(retrieval *service.Retrieval, kbID string, onRetrieval func(ctx context.Context, r *service.RetrievalResult) error) *KBRetrieval {
	return &KBRetrieval{retrieval: retrieval, kbID: kbID, onRetrieval: onRetrieval}
}

func (t *KBRetrieval) Name() string { return "kb_retrieval" }

func (t *KBRetrieval) Description() string {
	return "Search the knowledge base for content relevant to a query. Returns numbered passages with document titles. Call again with a refined query if results are insufficient."
}

func (t *KBRetrieval) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query in the same language as the documents"}
		},
		"required": ["query"]
	}`)
}

type kbRetrievalArgs struct {
	Query string `json:"query"`
}

func (t *KBRetrieval) Invoke(ctx context.Context, argsJSON string) (string, error) {
	var args kbRetrievalArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	args.Query = strings.TrimSpace(args.Query)
	if args.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	r, err := t.retrieval.Retrieve(ctx, t.kbID, args.Query)
	if err != nil {
		return "", err
	}
	if t.onRetrieval != nil {
		if err := t.onRetrieval(ctx, r); err != nil {
			return "", err
		}
	}
	if len(r.Hits) == 0 {
		return "The knowledge base has no relevant content for this query.", nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "evidence_level: %s\n", r.EvidenceLevel)
	for i, hit := range r.Hits {
		fmt.Fprintf(&b, "[%d] document: %s (seq %d, score %.4f)\n%s\n\n",
			i+1, hit.DocumentTitle, hit.Seq, hit.Score, hit.Content)
	}
	return b.String(), nil
}
