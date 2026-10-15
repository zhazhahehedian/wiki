import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

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

  it("copies the plain source text of a fenced block (highlight spans stripped)", async () => {
    const user = userEvent.setup({ writeToClipboard: false });
    // userEvent.setup() 会用自带的 clipboard stub 覆盖 navigator.clipboard，
    // 因此需在 setup 之后对 stub 的 writeText 做 spy
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    render(<MarkdownContent content={"```bash\nshow interface status\n```"} />);
    await user.click(screen.getByRole("button", { name: "复制代码" }));
    // 复制内容应为纯源码文本：rehype-highlight 的 span 被 extractText 摊平，末尾换行被去掉
    expect(writeText).toHaveBeenCalledWith("show interface status");
  });

  it("renders inline code without CodeBlock chrome", () => {
    render(<MarkdownContent content={"先执行 `goose up` 再继续"} />);
    expect(screen.getByText("goose up")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "复制代码" })).not.toBeInTheDocument();
  });
});
