import { Suspense } from "react";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import KBDocsPage from "./page";

vi.mock("@/lib/hooks/use-docs", () => ({
  useDocsByKB: () => ({ data: { items: [], total: 0 }, isLoading: false, isError: false }),
}));
vi.mock("@/lib/hooks/use-kbs", () => ({
  useKbs: () => ({ data: { items: [{ id: "kb-1", name: "运维知识库" }] } }),
}));
vi.mock("@/components/docs/doc-uploader", () => ({
  DocUploader: () => <button>上传文件</button>,
}));
vi.mock("@/components/docs/feishu-import-dialog", () => ({
  FeishuImportDialog: () => <button>从飞书导入</button>,
}));
vi.mock("@/components/ui/sidebar", () => ({ SidebarTrigger: () => <button>打开侧栏</button> }));

describe("KBDocsPage import actions", () => {
  it("keeps file upload and Feishu import available together", async () => {
    const params = Promise.resolve({ kbId: "kb-1" }) as Promise<{ kbId: string }> & {
      status: "fulfilled";
      value: { kbId: string };
    };
    params.status = "fulfilled";
    params.value = { kbId: "kb-1" };
    render(
      <Suspense fallback={<p>loading</p>}>
        <KBDocsPage params={params} />
      </Suspense>,
    );

    expect(await screen.findByRole("button", { name: "上传文件" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "从飞书导入" })).toBeInTheDocument();
  });
});
