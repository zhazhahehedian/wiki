package agent

import (
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

const reactSystemPrompt = `You are a knowledge base agent with tools.
- Before answering any content question, call kb_retrieval to search the knowledge base. Call it again with a refined query if the first results are insufficient.
- When the user asks what documents exist in the knowledge base, call list_documents.
- Never invent facts that are not supported by tool results. If the knowledge base lacks the answer, say so.
- When you use retrieved content, include inline markers like [1] that match the numbered retrieval results.
- Answer concisely in the same language as the user.`

func BuildReActMessages(history []*domain.ChatMessage, question string) []ports.Message {
	msgs := []ports.Message{{Role: "system", Content: reactSystemPrompt}}
	for _, m := range history {
		if m.Role == domain.RoleUser || m.Role == domain.RoleAssistant {
			msgs = append(msgs, ports.Message{Role: m.Role, Content: m.Content})
		}
	}
	msgs = append(msgs, ports.Message{Role: domain.RoleUser, Content: question})
	return msgs
}
