package service

import (
	"context"
	"fmt"

	"github.com/zenith-wang/it-wiki/backend/internal/agent"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

func (s *Chat) askReAct(ctx context.Context, conv generated.Conversation, content string, sink ChatStreamSink) error {
	if s.reactAgent == nil || s.toolFactory == nil {
		return fmt.Errorf("react mode is not configured")
	}
	history, err := s.recentHistory(ctx, conv.ID)
	if err != nil {
		return err
	}
	msgs := agent.BuildReActMessages(history, content)

	// 跨多次 kb_retrieval 按 chunk_id 去重，citations 全量重编号（spec D4）
	seen := map[string]bool{}
	var allHits []ports.VectorSearchHit
	var citations []domain.Citation
	onRetrieval := func(cbCtx context.Context, r *RetrievalResult) error {
		for _, hit := range r.Hits {
			if seen[hit.ChunkID] {
				continue
			}
			seen[hit.ChunkID] = true
			allHits = append(allHits, hit)
		}
		citations = BuildCitations(allHits)
		return sink.SendRetrieval(cbCtx, &RetrievalResult{
			EvidenceLevel: r.EvidenceLevel,
			Citations:     citations,
		})
	}
	tools := s.toolFactory(conv.KbID.String(), onRetrieval)

	result, err := s.reactAgent.Run(ctx, msgs, tools, sink)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		_ = sink.SendError(ctx, ChatStreamError{Code: "llm_stream_failed", Message: err.Error()})
		return err
	}
	// 持久化前必须再查取消状态，避免落半截消息（spec §4.1，沿用 2.5 保障）
	if err := ctx.Err(); err != nil {
		return err
	}

	assistant, err := s.createMessage(ctx, conv.ID, domain.RoleAssistant, result.Content, citations, result.Steps, result.Usage)
	if err != nil {
		_ = sink.SendError(ctx, ChatStreamError{Code: "assistant_persist_failed", Message: err.Error()})
		return err
	}
	if err := s.queries.TouchConversation(ctx, conv.ID); err != nil {
		return fmt.Errorf("touch conversation after assistant message: %w", err)
	}
	return sink.SendDone(ctx, ChatDone{MessageID: assistant.ID, ConversationID: conv.ID.String(), Usage: result.Usage})
}
