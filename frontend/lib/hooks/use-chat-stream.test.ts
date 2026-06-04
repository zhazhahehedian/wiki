import { describe, expect, it } from "vitest";

import { applyStreamEvent, type ChatStreamState } from "./use-chat-stream";

describe("applyStreamEvent", () => {
  const baseState: ChatStreamState = {
    isStreaming: true,
    error: null,
    messages: [
      {
        id: "draft-assistant",
        conversation_id: "conversation-1",
        role: "assistant",
        content: "",
        citations: [],
        tool_calls: [],
        token_usage: {},
        created_at: "2026-06-04T00:00:00.000Z",
        pending: true,
      },
    ],
  };

  it("adds retrieval citations to the draft assistant", () => {
    const next = applyStreamEvent(baseState, "draft-assistant", {
      event: "retrieval",
      data: {
        evidence_level: "sufficient",
        citations: [
          {
            id: "c1",
            chunk_id: "chunk-1",
            document_id: "doc-1",
            document_title: "Runbook.md",
            seq: 1,
            score: 0.9,
            snippet: "rotate",
          },
        ],
      },
    });

    expect(next.messages[0].citations).toHaveLength(1);
  });

  it("appends token text and finalizes the draft id on done", () => {
    const withToken = applyStreamEvent(baseState, "draft-assistant", {
      event: "token",
      data: { text: "hello" },
    });
    const done = applyStreamEvent(withToken, "draft-assistant", {
      event: "done",
      data: { message_id: "message-1", conversation_id: "conversation-1", usage: { total_tokens: 3 } },
    });

    expect(done.messages[0]).toMatchObject({
      id: "message-1",
      content: "hello",
      pending: false,
      token_usage: { total_tokens: 3 },
    });
  });

  it("marks the draft failed on stream error", () => {
    const next = applyStreamEvent(baseState, "draft-assistant", {
      event: "error",
      data: { code: "llm_stream_failed", message: "provider failed" },
    });

    expect(next.error).toBe("provider failed");
    expect(next.isStreaming).toBe(false);
    expect(next.messages[0]).toMatchObject({ pending: false, error: "provider failed" });
  });
});
