import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { LocalChatMessage, LocalToolStep } from "@/lib/hooks/use-chat-stream";
import { MessageBubble } from "./message-bubble";

function makeMessage(patch: Partial<LocalChatMessage>): LocalChatMessage {
  return {
    id: "m1",
    client_key: "m1",
    conversation_id: "c1",
    role: "assistant",
    content: "",
    citations: [],
    tool_calls: [],
    token_usage: {},
    created_at: "2026-07-09T00:00:00.000Z",
    ...patch,
  };
}

describe("MessageBubble", () => {
  it("renders user message as right-aligned bubble", () => {
    render(<MessageBubble message={makeMessage({ role: "user", content: "端口不通" })} onCitationClick={vi.fn()} />);
    const article = screen.getByText("端口不通").closest("article");
    expect(article?.className).toContain("justify-end");
  });

  it("renders assistant markdown full-width without bubble border", () => {
    render(<MessageBubble message={makeMessage({ content: "**排查步骤**" })} onCitationClick={vi.fn()} />);
    expect(screen.getByText("排查步骤")).toBeInTheDocument();
    const article = screen.getByText("排查步骤").closest("article");
    expect(article?.className).toContain("justify-start");
  });

  it("shows streaming cursor while pending without content", () => {
    const { container } = render(
      <MessageBubble message={makeMessage({ pending: true })} onCitationClick={vi.fn()} />,
    );
    expect(container.querySelector(".animate-caret-blink")).not.toBeNull();
  });

  it("renders citations footer with source label", () => {
    const citation = {
      id: "c1",
      chunk_id: "chunk-1",
      document_id: "d1",
      document_title: "排障手册",
      seq: 3,
      score: 0.92,
      snippet: "...",
    };
    render(<MessageBubble message={makeMessage({ content: "答案", citations: [citation] })} onCitationClick={vi.fn()} />);
    expect(screen.getByText("来源")).toBeInTheDocument();
    expect(screen.getByText("排障手册")).toBeInTheDocument();
  });

  it("keeps timeline expanded and shows cursor while pending with tool calls and no content", () => {
    const step: LocalToolStep = {
      step: 1,
      id: "call-1",
      name: "kb_retrieval",
      arguments: { query: "端口" },
      result: "[1] 内容",
      duration_ms: 120,
    };
    const { container } = render(
      <MessageBubble message={makeMessage({ pending: true, tool_calls: [step] })} onCitationClick={vi.fn()} />,
    );
    // 流式过程中时间线保持展开：无「调用了 N 个工具」pill，步骤可见
    expect(screen.queryByText(/调用了 1 个工具/)).not.toBeInTheDocument();
    expect(screen.getByText("kb_retrieval")).toBeInTheDocument();
    // 工具间隙（无内容）也要显示光标，避免空窗
    expect(container.querySelector(".animate-caret-blink")).not.toBeNull();
  });

  it("collapses timeline to pill once the message is no longer pending", () => {
    const step: LocalToolStep = {
      step: 1,
      id: "call-1",
      name: "kb_retrieval",
      arguments: { query: "端口" },
      result: "[1] 内容",
      duration_ms: 120,
    };
    render(
      <MessageBubble message={makeMessage({ content: "答案", tool_calls: [step] })} onCitationClick={vi.fn()} />,
    );
    expect(screen.getByText(/调用了 1 个工具/)).toBeInTheDocument();
    expect(screen.queryByText("kb_retrieval")).not.toBeInTheDocument();
  });
});
