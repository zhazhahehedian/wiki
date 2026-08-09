package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/zenith-wang/it-wiki/backend/internal/agent"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
)

type preparedReAct struct {
	runner    ports.AgentRunner
	messages  []ports.Message
	tools     []ports.Tool
	citations []domain.Citation
}

func (s *Chat) prepareReAct(ctx context.Context, runner ports.AgentRunner, conv generated.Conversation, content string, sink ChatStreamSink) (*preparedReAct, error) {
	if runner == nil || s.toolRegistry == nil {
		return nil, fmt.Errorf("react mode is not configured")
	}
	ownerID, err := ownerUUID(OwnerIDFromContext(ctx))
	if err != nil {
		return nil, err
	}
	history, err := s.recentHistory(ctx, ownerID, conv.ID)
	if err != nil {
		return nil, err
	}
	prepared := &preparedReAct{
		runner:   runner,
		messages: agent.BuildReActMessages(history, content),
	}

	// 跨多次 kb_retrieval 按 chunk_id 去重，citations 全量重编号（spec D4）
	seen := map[string]bool{}
	var allHits []ports.VectorSearchHit
	onRetrieval := func(cbCtx context.Context, r *RetrievalResult) error {
		for _, hit := range r.Hits {
			if seen[hit.ChunkID] {
				continue
			}
			seen[hit.ChunkID] = true
			allHits = append(allHits, hit)
		}
		prepared.citations = BuildCitations(allHits)
		return sink.SendRetrieval(cbCtx, &RetrievalResult{
			EvidenceLevel: r.EvidenceLevel,
			Citations:     prepared.citations,
		})
	}
	prepared.tools, err = s.toolRegistry.ToolsFor(ctx, conv.AgentID, conv.KbID.String(), onRetrieval)
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

func (s *Chat) askReAct(ctx context.Context, ownerID pgtype.UUID, prepared *preparedReAct, conv generated.Conversation, sink ChatStreamSink) error {
	result, err := prepared.runner.Run(ctx, prepared.messages, prepared.tools, sink)
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

	assistant, err := s.createMessage(ctx, ownerID, conv.ID, domain.RoleAssistant, result.Content, prepared.citations, result.Steps, result.Usage)
	if err != nil {
		_ = sink.SendError(ctx, ChatStreamError{Code: "assistant_persist_failed", Message: err.Error()})
		return err
	}
	if err := s.queries.TouchConversationForOwner(ctx, generated.TouchConversationForOwnerParams{ID: conv.ID, OwnerUserID: ownerID}); err != nil {
		return fmt.Errorf("touch conversation after assistant message: %w", err)
	}
	return sink.SendDone(ctx, ChatDone{MessageID: assistant.ID, ConversationID: conv.ID.String(), Usage: result.Usage})
}
