import type { QueryClient } from "@tanstack/react-query";

import type { Doc } from "@/lib/schemas";

const SYNC_WATCH_FAST_WINDOW_MS = 60_000;
const SYNC_WATCH_FAST_INTERVAL_MS = 2_000;
const SYNC_WATCH_SLOW_INTERVAL_MS = 15_000;

type FeishuSyncWatch = {
  baselineUpdatedAt: string;
  fastUntil: number;
  observedSyncingAt?: string;
};

function watchKey(docId: string) {
  return ["feishu-sync-watch", docId] as const;
}

export function beginFeishuSyncWatch(queryClient: QueryClient, doc: Doc) {
  // River backlog can exceed QueryClient's default five-minute inactive-query GC.
  queryClient.setQueryDefaults(["feishu-sync-watch"], { gcTime: Infinity });
  queryClient.setQueryData<FeishuSyncWatch>(watchKey(doc.id), {
    baselineUpdatedAt: doc.updated_at,
    fastUntil: Date.now() + SYNC_WATCH_FAST_WINDOW_MS,
    observedSyncingAt: doc.sync_status === "syncing" ? doc.updated_at : undefined,
  });
}

export function feishuSyncWatchInterval(queryClient: QueryClient, docId: string): number | false {
  const watch = queryClient.getQueryData<FeishuSyncWatch>(watchKey(docId));
  if (!watch) return false;
  return Date.now() < watch.fastUntil
    ? SYNC_WATCH_FAST_INTERVAL_MS
    : SYNC_WATCH_SLOW_INTERVAL_MS;
}

export function observeFeishuSync(queryClient: QueryClient, doc: Doc) {
  const key = watchKey(doc.id);
  const watch = queryClient.getQueryData<FeishuSyncWatch>(key);
  if (!watch) return;

  if (doc.sync_status === "syncing") {
    if (!watch.observedSyncingAt || timestampAtOrAfter(doc.updated_at, watch.observedSyncingAt)) {
      queryClient.setQueryData<FeishuSyncWatch>(key, {
        ...watch,
        observedSyncingAt: doc.updated_at,
      });
    }
    return;
  }

  const completed = doc.updated_at !== watch.baselineUpdatedAt
    && (!watch.observedSyncingAt || timestampAtOrAfter(doc.updated_at, watch.observedSyncingAt));
  if (!completed) return;

  queryClient.removeQueries({ queryKey: key, exact: true });
  void queryClient.invalidateQueries({ queryKey: ["docs"], refetchType: "none" });
  void queryClient.invalidateQueries({
    queryKey: ["doc", doc.id],
    exact: true,
    refetchType: "none",
  });
  void queryClient.invalidateQueries({ queryKey: ["chunks", doc.id] });
}

function timestampAtOrAfter(value: string, minimum: string): boolean {
  const valueTime = Date.parse(value);
  const minimumTime = Date.parse(minimum);
  return Number.isFinite(valueTime) && Number.isFinite(minimumTime) && valueTime >= minimumTime;
}
