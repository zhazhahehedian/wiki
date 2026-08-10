import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { docApi } from "@/lib/api/docs";
import type { Doc } from "@/lib/schemas";
import { feishuSyncWatchInterval } from "./feishu-sync-watch";
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

  it("polls every two seconds throughout the initial sixty-second claim window", async () => {
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

    await advancePolling(58_000);
    const callsBeforeWindowEnd = vi.mocked(docApi.get).mock.calls.length;
    await advancePolling(2_000);

    expect(callsBeforeWindowEnd).toBeGreaterThan(callsAfterClaim + 20);
    expect(docApi.get).toHaveBeenCalledTimes(callsBeforeWindowEnd + 1);
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
    invalidate.mockClear();
    for (let cycle = 0; cycle < 3 && result.current.doc.data !== completed; cycle += 1) {
      await advancePolling(2_000);
    }

    expect(result.current.doc.data).toEqual(completed);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["docs"], refetchType: "none" });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["doc", "doc-1"],
      exact: true,
      refetchType: "none",
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["chunks", "doc-1"] });
    expect(docApi.get).toHaveBeenCalledTimes(4);
    const completedCalls = vi.mocked(docApi.get).mock.calls.length;

    await advancePolling(10_000);

    expect(docApi.get).toHaveBeenCalledTimes(completedCalls);
  });

  it("continues at a fifteen-second interval after the fast claim window", async () => {
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
    const callsAtWindowEnd = vi.mocked(docApi.get).mock.calls.length;
    expect(callsAtWindowEnd).toBeGreaterThan(20);

    await advancePolling(14_999);

    expect(docApi.get).toHaveBeenCalledTimes(callsAtWindowEnd);

    await advancePolling(1);

    expect(docApi.get).toHaveBeenCalledTimes(callsAtWindowEnd + 1);
  });

  it("keeps the low-frequency watch beyond the query cache garbage-collection window", async () => {
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
    await act(() => result.current.sync.mutateAsync());
    await advancePolling(10 * 60_000);
    const callsAfterTenMinutes = vi.mocked(docApi.get).mock.calls.length;

    await advancePolling(15_000);

    expect(docApi.get).toHaveBeenCalledTimes(callsAfterTenMinutes + 1);
  });

  it("stops actual GET polling on unmount while retaining the watch", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00Z"));
    const baseline = feishuDoc();
    vi.mocked(docApi.get).mockResolvedValue(baseline);
    vi.mocked(docApi.syncFeishu).mockResolvedValue(baseline);
    const { result, unmount } = renderHook(() => ({
      doc: useDoc("doc-1"),
      sync: useFeishuSync("kb-1", "doc-1"),
    }), { wrapper });

    await advancePolling(0);
    await act(() => result.current.sync.mutateAsync());
    const callsBeforeUnmount = vi.mocked(docApi.get).mock.calls.length;
    unmount();

    await advancePolling(75_000);

    expect(docApi.get).toHaveBeenCalledTimes(callsBeforeUnmount);
    expect(feishuSyncWatchInterval(queryClient, baseline.id)).toBe(15_000);
  });

  it("observes a delayed worker claim and completion, then stops polling", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00Z"));
    const watchStartedAt = Date.now();
    const baseline = feishuDoc();
    const syncing = feishuDoc({ sync_status: "syncing", updated_at: "2026-08-09T10:01:15Z" });
    const completed = feishuDoc({
      remote_revision: "rev-delayed",
      last_synced_at: "2026-08-09T10:01:17Z",
      updated_at: "2026-08-09T10:01:17Z",
    });
    vi.mocked(docApi.get).mockImplementation(async () => {
      const elapsed = Date.now() - watchStartedAt;
      if (elapsed >= 77_000) return completed;
      if (elapsed >= 70_000) return syncing;
      return baseline;
    });
    vi.mocked(docApi.syncFeishu).mockResolvedValue(baseline);
    const invalidate = vi.spyOn(queryClient, "invalidateQueries");
    const { result } = renderHook(() => ({
      doc: useDoc("doc-1"),
      sync: useFeishuSync("kb-1", "doc-1"),
    }), { wrapper });

    await advancePolling(0);
    expect(result.current.doc.data).toEqual(baseline);
    await act(() => result.current.sync.mutateAsync());
    invalidate.mockClear();
    await advancePolling(60_000);
    await advancePolling(15_000);
    await advancePolling(0);
    expect(queryClient.getQueryData(["doc", "doc-1"])).toEqual(syncing);
    for (let cycle = 0; cycle < 3 && result.current.doc.data !== completed; cycle += 1) {
      await advancePolling(2_000);
    }

    expect(result.current.doc.data).toEqual(completed);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["docs"], refetchType: "none" });
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ["doc", "doc-1"],
      exact: true,
      refetchType: "none",
    });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["chunks", "doc-1"] });
    const callsAfterCompletion = vi.mocked(docApi.get).mock.calls.length;

    await advancePolling(30_000);

    expect(docApi.get).toHaveBeenCalledTimes(callsAfterCompletion);
  });
});
