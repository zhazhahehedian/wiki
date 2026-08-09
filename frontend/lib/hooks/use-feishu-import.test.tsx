import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { docApi } from "@/lib/api/docs";
import type { Doc } from "@/lib/schemas";
import { useDoc } from "./use-docs";
import { useFeishuImport, useFeishuSync } from "./use-feishu-import";

vi.mock("@/lib/api/docs", () => ({
  docApi: {
    importFeishu: vi.fn(),
    syncFeishu: vi.fn(),
    get: vi.fn(),
  },
}));

function feishuDoc(overrides: Partial<Doc> = {}): Doc {
  return {
    id: "doc-1",
    kb_id: "kb-1",
    source_type: "feishu-docx",
    source_ref: "feishu://feishu.cn/docx/token",
    title: "Runbook",
    mime_type: "text/markdown",
    bytes: 42,
    checksum: "checksum",
    status: "ready",
    sync_status: "idle",
    metadata: {},
    created_at: "2026-08-09T10:00:00Z",
    updated_at: "2026-08-09T10:00:00Z",
    ...overrides,
  };
}

async function advancePolling(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe("Feishu document mutations", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.mocked(docApi.importFeishu).mockReset();
    vi.mocked(docApi.syncFeishu).mockReset();
    vi.mocked(docApi.get).mockReset();
    queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
  });

  afterEach(() => vi.useRealTimers());

  function wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  }

  it("imports into the selected KB and refreshes its document list", async () => {
    vi.mocked(docApi.importFeishu).mockResolvedValue({ id: "doc-1" } as never);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useFeishuImport("kb-1"), { wrapper });

    await act(() => result.current.mutateAsync("https://acme.feishu.cn/docx/token"));

    expect(docApi.importFeishu).toHaveBeenCalledWith("kb-1", "https://acme.feishu.cn/docx/token");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["docs", "kb-1"] });
  });

  it("refreshes both detail and list after a manual sync request", async () => {
    vi.mocked(docApi.syncFeishu).mockResolvedValue({ id: "doc-1" } as never);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => useFeishuSync("kb-1", "doc-1"), { wrapper });

    await act(() => result.current.mutateAsync());

    expect(docApi.syncFeishu).toHaveBeenCalledWith("doc-1");
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["docs", "kb-1"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["doc", "doc-1"] });
  });

  it("continues polling while the worker claim race still reports idle", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00Z"));
    const baseline = feishuDoc();
    vi.mocked(docApi.get).mockResolvedValue(baseline);
    vi.mocked(docApi.syncFeishu).mockResolvedValue(baseline);
    const { result } = renderHook(() => ({
      doc: useDoc("doc-1"),
      sync: useFeishuSync("kb-1", "doc-1"),
    }), { wrapper });

    await advancePolling(0);
    expect(result.current.doc.data).toEqual(baseline);
    await act(() => result.current.sync.mutateAsync());
    const callsAfterClaim = vi.mocked(docApi.get).mock.calls.length;

    await advancePolling(2_000);
    await advancePolling(2_000);

    expect(docApi.get).toHaveBeenCalledTimes(callsAfterClaim + 2);
    expect(result.current.doc.data).toEqual(baseline);
  });

  it("stops after syncing returns to idle and invalidates list, detail, and chunks", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00Z"));
    const baseline = feishuDoc();
    const syncing = feishuDoc({ sync_status: "syncing", updated_at: "2026-08-09T10:00:02Z" });
    const completed = feishuDoc({
      remote_revision: "rev-2",
      last_synced_at: "2026-08-09T10:00:04Z",
      updated_at: "2026-08-09T10:00:04Z",
    });
    vi.mocked(docApi.get)
      .mockResolvedValueOnce(baseline)
      .mockResolvedValueOnce(baseline)
      .mockResolvedValueOnce(syncing)
      .mockResolvedValueOnce(completed);
    vi.mocked(docApi.syncFeishu).mockResolvedValue(baseline);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => ({
      doc: useDoc("doc-1"),
      sync: useFeishuSync("kb-1", "doc-1"),
    }), { wrapper });

    await advancePolling(0);
    expect(result.current.doc.data).toEqual(baseline);
    await act(() => result.current.sync.mutateAsync());
    for (let cycle = 0; cycle < 3 && result.current.doc.data !== completed; cycle += 1) {
      await advancePolling(2_000);
    }

    expect(result.current.doc.data).toEqual(completed);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["docs", "kb-1"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["doc", "doc-1"] });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["chunks", "doc-1"] });
    const completedCalls = vi.mocked(docApi.get).mock.calls.length;

    await advancePolling(10_000);

    expect(docApi.get).toHaveBeenCalledTimes(completedCalls);
  });

  it("stops issuing GETs after the sixty second watch deadline", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00Z"));
    const baseline = feishuDoc();
    vi.mocked(docApi.get).mockResolvedValue(baseline);
    vi.mocked(docApi.syncFeishu).mockResolvedValue(baseline);
    const { result } = renderHook(() => ({
      doc: useDoc("doc-1"),
      sync: useFeishuSync("kb-1", "doc-1"),
    }), { wrapper });

    await advancePolling(0);
    expect(result.current.doc.data).toEqual(baseline);
    await act(() => result.current.sync.mutateAsync());
    await advancePolling(60_000);
    const callsAtDeadline = vi.mocked(docApi.get).mock.calls.length;
    expect(callsAtDeadline).toBeGreaterThan(2);

    await advancePolling(10_000);

    expect(docApi.get).toHaveBeenCalledTimes(callsAtDeadline);
  });
});
