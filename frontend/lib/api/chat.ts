import { apiFetch, apiFetchList } from "./client";
import { parseSSEBuffer } from "./sse";
import type { ChatMessage, ChunkNeighbors, Conversation } from "@/lib/schemas";

const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export type ChatStreamEvent =
  | {
      event: "retrieval";
      data: { evidence_level: "none" | "weak" | "sufficient"; citations: ChatMessage["citations"] };
    }
  | { event: "token"; data: { text: string } }
  | { event: "tool_call"; data: { id: string; name: string; arguments: string } }
  | {
      event: "tool_result";
      data: { id: string; name: string; result?: string; duration_ms: number; error?: string };
    }
  | { event: "done"; data: { message_id: string; conversation_id: string; usage?: Record<string, unknown> } }
  | { event: "error"; data: { code: string; message: string } };

export const chatApi = {
  listConversations(kbId: string, limit = 50, offset = 0) {
    return apiFetchList<Conversation>(`/api/v1/kbs/${kbId}/conversations?limit=${limit}&offset=${offset}`);
  },
  createConversation(kbId: string) {
    return apiFetch<Conversation>(`/api/v1/kbs/${kbId}/conversations`, { method: "POST" });
  },
  updateConversationMode(conversationId: string, mode: "rag" | "react") {
    return apiFetch<Conversation>(`/api/v1/conversations/${conversationId}`, {
      method: "PATCH",
      body: JSON.stringify({ mode }),
    });
  },
  listMessages(conversationId: string, limit = 100, offset = 0) {
    return apiFetchList<ChatMessage>(`/api/v1/conversations/${conversationId}/messages?limit=${limit}&offset=${offset}`);
  },
  getNeighbors(kbId: string, chunkId: string, window = 2) {
    return apiFetch<ChunkNeighbors>(`/api/v1/kbs/${kbId}/chunks/${chunkId}/neighbors?window=${window}`);
  },
};

export async function streamConversationMessage(
  conversationId: string,
  content: string,
  handlers: { onEvent: (event: ChatStreamEvent) => void; signal?: AbortSignal },
): Promise<void> {
  const response = await fetch(`${BASE}/api/v1/conversations/${conversationId}/messages/stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    body: JSON.stringify({ content }),
    signal: handlers.signal,
  });

  if (!response.ok || !response.body) {
    const message = await response.text().catch(() => "Stream request failed");
    throw new Error(message || "Stream request failed");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder("utf-8");
  let buffer = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parsed = parseSSEBuffer(buffer);
    buffer = parsed.rest;
    for (const event of parsed.events) {
      handlers.onEvent({ event: event.event, data: JSON.parse(event.data) } as ChatStreamEvent);
    }
  }
}
