import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { MarkdownContent } from "./markdown-content";

describe("MarkdownContent", () => {
  it("renders fenced code block as macOS-style CodeBlock with copy button", () => {
    render(<MarkdownContent content={"```bash\nshow interface status\n```"} />);
    expect(screen.getByRole("button", { name: "复制代码" })).toBeInTheDocument();
    expect(screen.getByText("bash")).toBeInTheDocument();
  });

  it("renders GFM table", () => {
    const md = "| 端口 | 状态 |\n| --- | --- |\n| Gi0/1 | up |";
    render(<MarkdownContent content={md} />);
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByText("Gi0/1")).toBeInTheDocument();
  });

  it("renders inline code without CodeBlock chrome", () => {
    render(<MarkdownContent content={"先执行 `goose up` 再继续"} />);
    expect(screen.getByText("goose up")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "复制代码" })).not.toBeInTheDocument();
  });
});
