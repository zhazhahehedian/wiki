import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Doc } from "@/lib/schemas";
import { DocTable } from "./doc-table";

const syncMutate = vi.fn();

vi.mock("@/lib/hooks/use-docs", () => ({
  useDeleteDoc: () => ({ mutate: vi.fn(), isPending: false }),
  useReingestDoc: () => ({ mutate: vi.fn(), isPending: false }),
}));
vi.mock("@/lib/hooks/use-feishu-import", () => ({
  useFeishuSync: () => ({ mutate: syncMutate, isPending: false }),
}));
vi.mock("@/components/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ render }: { render: React.ReactElement }) => render,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <span>{children}</span>,
}));

function doc(fields: Partial<Doc> = {}): Doc {
  return {
    id: "doc-1",
    kb_id: "kb-1",
    source_type: "local-upload",
    source_ref: "objects/doc-1.md",
    title: "Runbook",
    mime_type: "text/markdown",
    bytes: 2048,
    checksum: "checksum",
    status: "ready",
    metadata: {},
    created_at: "2026-08-09T00:00:00Z",
    updated_at: "2026-08-09T00:00:00Z",
    ...fields,
  };
}

describe("DocTable source and sync status", () => {
  beforeEach(() => syncMutate.mockReset());

  it("keeps ready ingestion separate from a failed remote sync", async () => {
    const user = userEvent.setup();
    render(
      <DocTable
        kbId="kb-1"
        docs={[doc({
          source_type: "feishu-docx",
          source_url: "https://acme.feishu.cn/docx/token",
          sync_status: "failed",
          remote_revision: "rev-17",
          last_sync_error: "飞书权限已失效",
          last_synced_at: "2026-08-09T01:00:00Z",
        })]}
      />,
    );

    expect(screen.getByText("就绪")).toBeInTheDocument();
    expect(screen.getByText("同步失败")).toBeInTheDocument();
    expect(screen.getByText("飞书文档")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "打开飞书原文" })).toHaveAttribute(
      "href",
      "https://acme.feishu.cn/docx/token",
    );
    expect(screen.getByText(/上次同步/)).toBeInTheDocument();
    expect(screen.getByText("飞书权限已失效")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "重试同步" }));
    expect(screen.getByText(/rev-17/)).toBeInTheDocument();
    expect(syncMutate).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "重新处理" })).not.toBeInTheDocument();
  });

  it("labels local uploads and remains horizontally scrollable on narrow screens", () => {
    const { container } = render(<DocTable kbId="kb-1" docs={[doc()]} />);

    expect(screen.getByText("本地上传")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "重新处理" })).toBeInTheDocument();
    expect(container.querySelector("[data-slot=table-container]")?.className).toContain("overflow-x-auto");
  });
});
