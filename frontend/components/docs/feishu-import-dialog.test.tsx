import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { FeishuImportDialog } from "./feishu-import-dialog";

const mutate = vi.fn();
const reset = vi.fn();
const useFeishuImport = vi.fn();

vi.mock("@/lib/hooks/use-feishu-import", () => ({
  useFeishuImport: () => useFeishuImport(),
}));

describe("FeishuImportDialog", () => {
  beforeEach(() => {
    mutate.mockReset();
    reset.mockReset();
    useFeishuImport.mockReset();
    useFeishuImport.mockReturnValue({ mutate, reset, isPending: false, isError: false });
  });

  async function openDialog() {
    const user = userEvent.setup();
    render(<FeishuImportDialog kbId="kb-1" />);
    await user.click(screen.getByRole("button", { name: "从飞书导入" }));
    return user;
  }

  it("validates a single Feishu URL before submission", async () => {
    const user = await openDialog();
    const input = screen.getByLabelText("飞书文档链接");

    await user.type(input, "https://example.com/docx/token");
    await user.click(screen.getByRole("button", { name: "开始导入" }));
    expect(screen.getByRole("alert")).toHaveTextContent("请输入有效的飞书文档链接");
    expect(mutate).not.toHaveBeenCalled();

    await user.clear(input);
    await user.type(input, "https://acme.feishu.cn/docx/one https://acme.feishu.cn/docx/two");
    await user.click(screen.getByRole("button", { name: "开始导入" }));
    expect(screen.getByRole("alert")).toHaveTextContent("每次只能导入一个链接");
    expect(mutate).not.toHaveBeenCalled();
  });

  it("submits a trimmed URL and closes after success", async () => {
    mutate.mockImplementation((_url: string, options: { onSuccess: () => void }) => options.onSuccess());
    const user = await openDialog();

    await user.type(screen.getByLabelText("飞书文档链接"), "  https://acme.feishu.cn/wiki/token  ");
    await user.click(screen.getByRole("button", { name: "开始导入" }));

    expect(mutate).toHaveBeenCalledWith(
      "https://acme.feishu.cn/wiki/token",
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("shows the request error and preserves the URL for retry", async () => {
    useFeishuImport.mockReturnValue({
      mutate,
      reset,
      isPending: false,
      isError: true,
      error: new Error("没有该资源的访问权限"),
    });
    await openDialog();
    const input = screen.getByLabelText("飞书文档链接");
    await userEvent.setup().type(input, "https://acme.feishu.cn/docx/token");

    expect(screen.getByRole("alert")).toHaveTextContent("没有该资源的访问权限");
    expect(input).toHaveValue("https://acme.feishu.cn/docx/token");
    expect(reset).toHaveBeenCalled();
  });

  it("locks the URL while an import request is pending", async () => {
    useFeishuImport.mockReturnValue({ mutate, reset, isPending: true, isError: false });
    await openDialog();

    expect(screen.getByRole("textbox")).toBeDisabled();
  });

  it("supports Escape and constrains the dialog on mobile", async () => {
    const user = await openDialog();
    const dialog = screen.getByRole("dialog");
    expect(dialog.className).toContain("max-w-[calc(100%-2rem)]");

    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
