import { describe, expect, it } from "vitest";

import { applyStreamEvent, type ChatStreamState, type LocalToolStep } from "./use-chat-stream";

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

describe("applyStreamEvent tool events", () => {
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

  it("moves draft content to thought and appends running step on tool_call", () => {
    const withContent = applyStreamEvent(baseState, "draft-assistant", {
      event: "token",
      data: { text: "让我查一下。" },
    });
    const next = applyStreamEvent(withContent, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: '{"query":"部署"}' },
    });

    const draft = next.messages[0];
    expect(draft.content).toBe("");
    expect(draft.tool_calls).toHaveLength(1);
    const step = draft.tool_calls[0] as LocalToolStep;
    expect(step).toMatchObject({
      step: 1,
      id: "call_1",
      name: "kb_retrieval",
      thought: "让我查一下。",
      running: true,
    });
    expect(step.arguments).toEqual({ query: "部署" });
  });

  it("keeps thought empty for subsequent calls in the same round", () => {
    let state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "{}" },
    });
    state = applyStreamEvent(state, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_2", name: "list_documents", arguments: "{}" },
    });
    const steps = state.messages[0].tool_calls as LocalToolStep[];
    expect(steps).toHaveLength(2);
    expect(steps[1].thought).toBeUndefined();
    expect(steps[1].step).toBe(2);
  });

  it("settles the matching step on tool_result", () => {
    let state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "{}" },
    });
    state = applyStreamEvent(state, "draft-assistant", {
      event: "tool_result",
      data: { id: "call_1", name: "kb_retrieval", result: "[1] 内容", duration_ms: 840 },
    });
    const step = (state.messages[0].tool_calls as LocalToolStep[])[0];
    expect(step).toMatchObject({ result: "[1] 内容", duration_ms: 840, running: false });
  });

  it("records tool error on tool_result with error", () => {
    let state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "{}" },
    });
    state = applyStreamEvent(state, "draft-assistant", {
      event: "tool_result",
      data: { id: "call_1", name: "kb_retrieval", duration_ms: 5, error: "boom" },
    });
    const step = (state.messages[0].tool_calls as LocalToolStep[])[0];
    expect(step.error).toBe("boom");
    expect(step.running).toBe(false);
  });

  it("keeps non-JSON arguments as raw string", () => {
    const state = applyStreamEvent(baseState, "draft-assistant", {
      event: "tool_call",
      data: { id: "call_1", name: "kb_retrieval", arguments: "not-json" },
    });
    const step = (state.messages[0].tool_calls as LocalToolStep[])[0];
    expect(step.arguments).toBe("not-json");
  });
});
