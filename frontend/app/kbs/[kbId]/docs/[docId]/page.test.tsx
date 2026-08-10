import { Suspense } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Doc } from "@/lib/schemas";
import DocDetailPage from "./page";

const useDoc = vi.fn();
const syncMutate = vi.fn();

vi.mock("@/lib/hooks/use-docs", () => ({ useDoc: () => useDoc() }));
vi.mock("@/lib/hooks/use-feishu-import", () => ({
  useFeishuSync: () => ({ mutate: syncMutate, isPending: false }),
}));
vi.mock("@/components/chunks/chunk-list", () => ({ ChunkList: () => <div>旧版 ready chunks</div> }));

function doc(fields: Partial<Doc> = {}): Doc {
  return {
    id: "doc-1",
    kb_id: "kb-1",
    source_type: "feishu-docx",
    source_ref: "feishu://feishu.cn/docx/token",
    source_url: "https://acme.feishu.cn/docx/token",
    title: "Runbook",
    mime_type: "text/markdown",
    bytes: 2048,
    checksum: "checksum",
    status: "ready",
    sync_status: "failed",
    last_sync_error: "新版本拉取失败",
    last_synced_at: "2026-08-09T01:00:00Z",
    metadata: {},
    created_at: "2026-08-09T00:00:00Z",
    updated_at: "2026-08-09T00:00:00Z",
    ...fields,
  };
}

function params() {
  const value = { kbId: "kb-1", docId: "doc-1" };
  const promise = Promise.resolve(value) as Promise<typeof value> & { status: "fulfilled"; value: typeof value };
  promise.status = "fulfilled";
  promise.value = value;
  return promise;
}

describe("DocDetailPage remote sync state", () => {
  beforeEach(() => {
    syncMutate.mockReset();
    useDoc.mockReset();
  });

  it("keeps old ready chunks visible after a failed resync", async () => {
    const user = userEvent.setup();
    useDoc.mockReturnValue({ data: doc(), isLoading: false, isError: false });
    render(<Suspense><DocDetailPage params={params()} /></Suspense>);

    expect(screen.getByText("就绪")).toBeInTheDocument();
    expect(screen.getByText("同步失败")).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("新版本拉取失败");
    expect(screen.getByText("旧版 ready chunks")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "打开飞书原文" })).toHaveAttribute(
      "href",
      "https://acme.feishu.cn/docx/token",
    );

    await user.click(screen.getByRole("button", { name: "重试同步" }));
    expect(syncMutate).toHaveBeenCalledOnce();
  });

  it("does not claim old content exists when initial ingestion failed", () => {
    useDoc.mockReturnValue({
      data: doc({ status: "failed", last_synced_at: null, error_message: "首次摄入失败" }),
      isLoading: false,
      isError: false,
    });
    render(<Suspense><DocDetailPage params={params()} /></Suspense>);

    expect(screen.getByText("摄入失败，无切片可显示")).toBeInTheDocument();
    expect(screen.queryByText("旧版 ready chunks")).not.toBeInTheDocument();
  });
});
