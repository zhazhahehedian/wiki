import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { LocalChatMessage } from "@/lib/hooks/use-chat-stream";
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
});
