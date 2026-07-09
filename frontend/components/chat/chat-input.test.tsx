import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ChatInput } from "./chat-input";

describe("ChatInput", () => {
  it("sends trimmed content on Enter and clears input", async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    render(<ChatInput onSend={onSend} onStop={vi.fn()} />);
    const box = screen.getByPlaceholderText(/输入问题/);
    await user.type(box, "  端口不通  {Enter}");
    expect(onSend).toHaveBeenCalledWith("端口不通");
    expect(box).toHaveValue("");
  });

  it("inserts newline on Shift+Enter without sending", async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    render(<ChatInput onSend={onSend} onStop={vi.fn()} />);
    const box = screen.getByPlaceholderText(/输入问题/);
    await user.type(box, "第一行{Shift>}{Enter}{/Shift}第二行");
    expect(onSend).not.toHaveBeenCalled();
    expect(box).toHaveValue("第一行\n第二行");
  });

  it("shows stop button and disables send while streaming", () => {
    render(<ChatInput disabled onSend={vi.fn()} onStop={vi.fn()} />);
    expect(screen.getByRole("button", { name: "停止生成" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "发送" })).toBeDisabled();
  });
});
