import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { LocalToolStep } from "@/lib/hooks/use-chat-stream";
import { AgentTimeline } from "./agent-timeline";

const doneStep: LocalToolStep = {
  step: 1,
  id: "call-1",
  name: "kb_retrieval",
  thought: "需要先检索知识库",
  arguments: { query: "交换机 端口" },
  result: '{"chunks":4}',
  duration_ms: 300,
};

const runningStep: LocalToolStep = {
  step: 2,
  id: "call-2",
  name: "list_documents",
  arguments: {},
  running: true,
};

const failedStep: LocalToolStep = {
  step: 2,
  id: "call-3",
  name: "kb_retrieval",
  arguments: {},
  error: "timeout",
};

describe("AgentTimeline", () => {
  it("renders nothing for empty steps", () => {
    const { container } = render(<AgentTimeline steps={[]} answerStarted={false} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders thought as quote and tool step with duration when expanded", () => {
    render(<AgentTimeline steps={[doneStep]} answerStarted={false} />);
    expect(screen.getByText("需要先检索知识库")).toBeInTheDocument();
    expect(screen.getByText("kb_retrieval")).toBeInTheDocument();
    expect(screen.getByText(/300 ms/)).toBeInTheDocument();
  });

  it("shows running state", () => {
    render(<AgentTimeline steps={[doneStep, runningStep]} answerStarted={false} />);
    expect(screen.getByText("运行中...")).toBeInTheDocument();
  });

  it("shows error state", () => {
    render(<AgentTimeline steps={[failedStep]} answerStarted={false} />);
    expect(screen.getByText("失败")).toBeInTheDocument();
    expect(screen.getByText("timeout")).toBeInTheDocument();
  });

  it("collapses to summary when answer started, expands on click", async () => {
    const user = userEvent.setup();
    render(<AgentTimeline steps={[doneStep]} answerStarted />);
    expect(screen.queryByText("需要先检索知识库")).not.toBeInTheDocument();
    const summary = screen.getByRole("button", { name: /调用了 1 个工具/ });
    await user.click(summary);
    expect(screen.getByText("需要先检索知识库")).toBeInTheDocument();
  });
});
