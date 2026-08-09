import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { Doc } from "@/lib/schemas";
import {
  beginFeishuSyncWatch,
  feishuSyncWatchInterval,
  observeFeishuSync,
} from "./feishu-sync-watch";
import { docNeedsRefresh, docsRefetchInterval } from "./use-docs";

function state(status: Doc["status"], syncStatus?: Doc["sync_status"]): Doc {
  return { status, sync_status: syncStatus } as Doc;
}

describe("docNeedsRefresh", () => {
  afterEach(() => vi.useRealTimers());

  it("treats ingestion and remote sync as independent state machines", () => {
    expect(docNeedsRefresh(state("ready", "syncing"))).toBe(true);
    expect(docNeedsRefresh(state("ready", "failed"))).toBe(false);
    expect(docNeedsRefresh(state("pending", "idle"))).toBe(true);
    expect(docNeedsRefresh(state("ready", "idle"))).toBe(false);
    expect(docNeedsRefresh(state("ready", "idle"), true)).toBe(true);
  });

  it("uses the shortest interval across normal work and slow sync watches", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00Z"));
    const queryClient = new QueryClient();
    const watched = {
      ...state("ready", "idle"),
      id: "watched",
      updated_at: "2026-08-09T09:00:00Z",
    };
    const ingesting = { ...state("parsing", "idle"), id: "ingesting" };
    beginFeishuSyncWatch(queryClient, watched);
    vi.setSystemTime(new Date("2026-08-09T10:01:00.001Z"));

    expect(docsRefetchInterval(queryClient, [watched])).toBe(15_000);
    expect(docsRefetchInterval(queryClient, [watched, ingesting])).toBe(2_000);
    expect(docsRefetchInterval(queryClient, [state("ready", "idle")])).toBe(false);
  });

  it("ignores an out-of-order idle baseline after another observer sees syncing", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-08-09T10:00:00Z"));
    const queryClient = new QueryClient();
    const baseline = {
      ...state("ready", "idle"),
      id: "doc-1",
      updated_at: "2026-08-09T10:00:00Z",
    };
    const syncing = {
      ...baseline,
      sync_status: "syncing" as const,
      updated_at: "2026-08-09T10:01:15Z",
    };
    const completed = {
      ...baseline,
      updated_at: "2026-08-09T10:01:17Z",
    };
    beginFeishuSyncWatch(queryClient, baseline);

    observeFeishuSync(queryClient, syncing);
    observeFeishuSync(queryClient, baseline);

    expect(feishuSyncWatchInterval(queryClient, baseline.id)).toBe(2_000);

    observeFeishuSync(queryClient, completed);

    expect(feishuSyncWatchInterval(queryClient, baseline.id)).toBe(false);
  });
});
