import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TaskProgressIndicator } from "./task-progress-indicator";

describe("TaskProgressIndicator", () => {
  it("announces progress and toggles from the keyboard", async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    render(
      <TaskProgressIndicator
        items={[
          { id: "one", status: "completed" },
          { id: "two", status: "running" },
        ]}
        expanded={false}
        onToggle={onToggle}
      />,
    );

    const progress = screen.getByRole("progressbar", { name: "已完成 1/2" });
    expect(progress).toHaveAttribute("aria-valuenow", "1");
    expect(progress).toHaveAttribute("aria-valuemax", "2");
    expect(screen.getByText("运行中")).toBeInTheDocument();

    const button = screen.getByRole("button", { name: /调用了 2 个工具/ });
    expect(button.className).toContain("max-w-[calc(100vw-2rem)]");
    button.focus();
    await user.keyboard("{Enter}");
    expect(onToggle).toHaveBeenCalledOnce();
  });

  it("announces completion when every task is complete", () => {
    render(
      <TaskProgressIndicator
        items={[{ id: "one", status: "completed" }]}
        expanded
        onToggle={vi.fn()}
      />,
    );

    expect(screen.getByText("已完成")).toBeInTheDocument();
  });
});
