# IT-Wiki Phase 2 Deterministic RAG Design

- Date: 2026-06-02
- Status: Ready for user review
- Scope: Phase 2 only, after the Phase 1 ingestion loop

---

## 0. Decision Summary

Phase 2 uses the recommended Approach 1: a deterministic RAG flow orchestrated inside the existing Go backend. The goal is to close the product loop from uploaded KB documents to streamed answers with citations. We will not introduce Agent behavior in this phase.

The flow is intentionally small:

1. Embed the user question.
2. Search ready chunks in the selected KB with pgvector.
3. Assemble citations from retrieval metadata.
4. Stream an LLM answer through SSE.
5. Persist the user message and final assistant message.
6. Let the user open each citation and inspect neighboring chunks.

Eino Chain/Graph and Agent-style orchestration are deferred. The Phase 2 services should still keep clean Go interfaces so an Eino adapter can wrap them later without rewriting retrieval, citation assembly, or chat persistence.

---

## 1. Goals And Non-Goals

### Goals

- Ask questions inside one KB and receive a streamed answer.
- Show citations before or alongside the answer.
- Let the user click a citation and inspect the matched chunk with nearby chunks.
- Persist conversations and messages using the existing `conversations` and `messages` tables.
- Add backend contracts that make later hybrid search or Agent integration possible without changing the frontend protocol.
- Keep behavior deterministic and testable with fake embedders, vector stores, and LLM streams.

### Non-Goals

- Elasticsearch, BM25, hybrid search, RRF, or reranking.
- LLM query planning, rewrite, decomposition, or HyDE.
- ReAct, tool calls, Eino Graph, or autonomous Agent behavior.
- Multi-KB retrieval.
- Memory compression or long-term user memory.
- Full-document highlight positioning.
- Evaluation dashboards or offline retrieval metrics.

---

## 2. Current Baseline

Phase 1 has already established most of the ingestion side:

- `Embedder` exists in `backend/internal/domain/ports/embedder.go`.
- `LLMClient` exists with non-streaming `Chat` in `backend/internal/domain/ports/llm.go`.
- `VectorStore` currently exposes only `InsertChunks`.
- `pgvector` can insert chunks and list chunks by document, but does not search yet.
- `conversations` and `messages` migrations exist, and sqlc generated models include both tables.
- There are no conversation queries, chat services, chat HTTP handlers, or frontend chat routes yet.
- The frontend has KB, docs, and chunks features, but no chat feature components.

Phase 2 should build on these boundaries instead of replacing the ingestion implementation.

---

## 3. Argus Reference Decisions

The local Argus project is useful as a reference, but Phase 2 should borrow selectively.

### Borrow

- A retrieval facade as the stable boundary between chat orchestration and vector search.
- KB-scoped metadata filters, plus a defensive second validation that returned chunks belong to the requested KB.
- Citation assembly from retrieval results instead of ad hoc frontend parsing.
- Evidence awareness: the backend tells the prompt and frontend whether retrieval found enough context.
- Neighbor windows around a matched chunk.
- SSE event names that separate retrieval, tokens, completion, and errors.

### Defer

- Elasticsearch/BM25 hybrid retrieval.
- RRF fusion.
- Query planning, query rewrite, and query decomposition.
- Agent tool invocation.
- Short-term memory compression.

Argus is richer than this project needs right now. Phase 2 should land the smaller deterministic core first.

---

## 4. Backend Architecture

### 4.1 Service Flow

```text
POST /api/v1/conversations/:conversationID/messages/stream
  -> ChatHandler
  -> ChatService.AskStream
      -> validate conversation belongs to one KB
      -> save user message
      -> RetrievalService.Retrieve
          -> Embedder.Embed(question)
          -> VectorStore.Search(kbID, embedding, topK, filter)
          -> CitationAssembler.Build(hits)
      -> send SSE retrieval event
      -> build prompt from system rules + citations + recent history + user question
      -> LLMClient.ChatStream
      -> send SSE token events
      -> save assistant message with citations after the stream completes
      -> send SSE done event
```

If the user disconnects, the request context must cancel the LLM stream. The user message remains saved. A partial assistant message is not saved unless the stream completes successfully.

### 4.2 Domain Ports

Extend `VectorStore` with search:

```go
type VectorSearchOptions struct {
    TopK     int
    MinScore float32
}

type VectorSearchHit struct {
    ChunkID    string
    KBID       string
    DocumentID string
    Seq        int
    Content    string
    Score      float32
    Metadata   map[string]any
}

type VectorStore interface {
    InsertChunks(ctx context.Context, items []domain.ChunkWithEmbedding) error
    Search(ctx context.Context, kbID string, query []float32, opts VectorSearchOptions) ([]VectorSearchHit, error)
}
```

Extend `LLMClient` with streaming:

```go
type StreamChunk struct {
    Text  string
    Done  bool
    Usage *TokenUsage
}

type LLMClient interface {
    Chat(ctx context.Context, msgs []Message, opts ChatOptions) (*Message, error)
    ChatStream(ctx context.Context, msgs []Message, opts ChatOptions) (<-chan StreamChunk, error)
}
```

The exact Go type names can be adjusted during planning, but the responsibilities should stay the same.

### 4.3 Retrieval Service

`RetrievalService` owns deterministic retrieval:

- Validate `kbID`, `question`, `topK`, and embedding dimension.
- Embed the user question as one text.
- Call `VectorStore.Search` with a KB filter.
- Re-check each returned hit's `KBID` equals the requested KB.
- Trim snippets for citation cards without changing the original chunk content.
- Return citations and an evidence level.

Evidence levels for Phase 2:

| Level | Meaning |
|---|---|
| `none` | No hits returned after filtering. |
| `weak` | Hits exist, but the top score is below configured `RAG_MIN_SCORE`. |
| `sufficient` | At least one hit is above configured `RAG_MIN_SCORE`. |

Default `RAG_TOP_K` is `8`. Default `RAG_MIN_SCORE` is `0.0` in Phase 2 so local development does not hide useful context before evaluation exists. The `weak` evidence level is still supported and activates when `RAG_MIN_SCORE` is raised through configuration.

### 4.4 Citation Shape

Citations are assembled on the backend and persisted in `messages.citations`.

```json
{
  "id": "c1",
  "chunk_id": "uuid",
  "document_id": "uuid",
  "document_title": "Runbook.md",
  "seq": 12,
  "score": 0.82,
  "snippet": "short excerpt shown in the UI"
}
```

The assistant prompt should label retrieved context as `[1]`, `[2]`, etc. The UI renders citation cards from the structured citation list rather than relying only on parsing inline markers from model text.

### 4.5 Prompt Rules

The Phase 2 system prompt should be strict and boring:

- Answer using the provided context when possible.
- Do not invent facts that are absent from the context.
- If evidence level is `none`, say that the KB does not contain enough information.
- Prefer concise answers.
- When using a cited chunk, include inline markers like `[1]` where natural.

Recent history is bounded by `RAG_HISTORY_MESSAGES`, defaulting to `10` messages from the same conversation. The prompt order is system rules, retrieved context, recent history, then the current user question.

---

## 5. HTTP API

All endpoints stay under `/api/v1`.

### Conversations

```http
GET  /kbs/:kbID/conversations
POST /kbs/:kbID/conversations
GET  /conversations/:conversationID/messages
```

`POST /kbs/:kbID/conversations` creates a conversation with default mode `rag`. Phase 2 does not expose a mode switch because Agent/ReAct is out of scope.

### Streaming Message

```http
POST /conversations/:conversationID/messages/stream
Content-Type: application/json
Accept: text/event-stream
```

Request:

```json
{
  "content": "How do we rotate the database password?"
}
```

Response is `text/event-stream`.

Native `EventSource` is not used for this endpoint because the request needs to be a POST with a JSON body. The frontend should use `fetch` plus a small SSE parser over `ReadableStream`.

### Chunk Neighbors

```http
GET /kbs/:kbID/chunks/:chunkID/neighbors?window=2
```

Response:

```json
{
  "primary_chunk_id": "uuid",
  "window": 2,
  "chunks": [
    {
      "id": "uuid",
      "document_id": "uuid",
      "seq": 10,
      "content": "...",
      "is_primary": false
    }
  ]
}
```

The backend must validate that the primary chunk belongs to `kbID`, then return chunks from the same document whose `seq` is between `primary.seq - window` and `primary.seq + window`.

---

## 6. SSE Protocol

Phase 2 uses four event types.

### `retrieval`

Sent after retrieval completes and before model tokens begin.

```text
event: retrieval
data: {"evidence_level":"sufficient","citations":[...]}
```

### `token`

Sent for each streamed assistant delta.

```text
event: token
data: {"text":"partial text"}
```

### `done`

Sent after the assistant message is persisted.

```text
event: done
data: {"message_id":"uuid","conversation_id":"uuid","usage":{}}
```

### `error`

Sent when the stream can still write an SSE error. The HTTP status may already be `200`, so the frontend must handle this event as a failed send.

```text
event: error
data: {"code":"llm_stream_failed","message":"..."}
```

Errors before headers are written should use the existing JSON error envelope and appropriate HTTP status.

---

## 7. Persistence Rules

- Save the user message before retrieval starts.
- Save the assistant message only after the LLM stream completes.
- Persist assistant citations as JSON in `messages.citations`.
- Keep `tool_calls` as an empty array in Phase 2.
- Update `conversations.updated_at` when a user message is saved and when the assistant message is saved.
- If retrieval returns no hits, still call the LLM with an explicit "no evidence" context so the answer is graceful.
- If the LLM stream fails after the user message is saved, return an SSE `error` event and do not save a partial assistant message.

---

## 8. Frontend Design

### 8.1 Routes

Add chat routes under the KB workspace:

```text
frontend/app/kbs/[kbId]/chats/page.tsx
frontend/app/kbs/[kbId]/chats/[conversationId]/page.tsx
```

The first screen for Phase 2 should be the usable chat experience, not a marketing or explanatory page.

### 8.2 Components

Place chat product components under `frontend/components/chat`:

- `chat-sidebar`
- `chat-header`
- `message-list`
- `message-bubble`
- `chat-input`
- `citation-card`
- `citation-drawer`

Use existing `components/ui` primitives and lucide icons where the app already uses them. Citation drawer state stays local to the chat page. Phase 2 does not add Zustand.

### 8.3 Client API And Hooks

Add chat API helpers under `frontend/lib/api/chat.ts`.

Add a streaming hook under `frontend/lib/hooks/use-chat-stream.ts`:

- Optimistically append the user message.
- Start a `fetch` POST request.
- Parse SSE events from `response.body`.
- Append `token` text into one assistant draft.
- Store citations from the `retrieval` event.
- Replace the draft with the persisted assistant message on `done`.
- Use `AbortController` to cancel the request if the user navigates away or presses stop.

Phase 2 adds `react-markdown` and `rehype-highlight` for assistant message rendering. Keep the dependency addition scoped to chat output rendering.

### 8.4 Citation Interaction

Citation cards appear with the assistant answer. Clicking a card opens a drawer:

1. Fetch `/api/v1/kbs/:kbID/chunks/:chunkID/neighbors?window=2`.
2. Show the primary chunk clearly.
3. Show surrounding chunks from the same document.
4. Keep the drawer closeable by button, escape, and overlay.

---

## 9. Error Handling

| Scenario | Behavior |
|---|---|
| Conversation does not belong to KB | Return 404 with the existing API error envelope; do not stream. |
| KB has no ready chunks | Save user message, send retrieval with `none`, stream a no-evidence answer. |
| Embedding fails | Return JSON error before stream if possible; otherwise SSE `error`. |
| Vector search fails | Return JSON error before stream if possible; otherwise SSE `error`. |
| LLM stream fails | SSE `error`; no partial assistant persistence. |
| Client disconnects | Cancel context; stop LLM stream; keep user message only. |
| Citation neighbor chunk missing | Return 404 with existing error envelope. |

---

## 10. Testing And Verification

Backend tests should cover:

- `RetrievalService` with fake embedder and fake vector store.
- KB defensive filtering in retrieval results.
- Citation assembly and evidence level decisions.
- `ChatService` persistence order with fake repositories and fake LLM stream.
- SSE event formatting for retrieval, token, done, and error.
- Neighbor lookup window and KB ownership validation.

Frontend checks should cover:

- Type safety for chat API schemas.
- SSE parser behavior for multi-line and partial chunks.
- `useChatStream` cancellation state.
- Citation drawer loading and empty/error states.

Expected verification before Phase 2 is called done:

```bash
cd backend && go test ./...
cd frontend && pnpm typecheck
cd frontend && pnpm build
```

---

## 11. Acceptance Criteria

- A user can open a KB with ready documents and create a new chat.
- Sending a question streams an answer in the UI.
- Retrieval citations are visible for the assistant answer.
- Clicking a citation opens nearby chunks around the cited chunk.
- Conversations and messages survive page refresh.
- Disconnecting or stopping a stream cancels backend work.
- No Agent, tool-call, hybrid-search, or query-planning behavior is present in Phase 2.

---

## 12. Next Step

After this spec is reviewed, use the `writing-plans` workflow to create the Phase 2 implementation plan in `docs/superpowers/plans/`.
