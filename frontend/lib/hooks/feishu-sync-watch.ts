import type { QueryClient } from "@tanstack/react-query";

import type { Doc } from "@/lib/schemas";

const SYNC_WATCH_WINDOW_MS = 60_000;

type FeishuSyncWatch = {
  baselineUpdatedAt: string;
  expiresAt: number;
  observedSyncing: boolean;
};

function watchKey(docId: string) {
  return ["feishu-sync-watch", docId] as const;
}

export function beginFeishuSyncWatch(queryClient: QueryClient, doc: Doc) {
  queryClient.setQueryData<FeishuSyncWatch>(watchKey(doc.id), {
    baselineUpdatedAt: doc.updated_at,
    expiresAt: Date.now() + SYNC_WATCH_WINDOW_MS,
    observedSyncing: doc.sync_status === "syncing",
  });
}

export function feishuSyncWatchActive(queryClient: QueryClient, docId: string): boolean {
  const watch = queryClient.getQueryData<FeishuSyncWatch>(watchKey(docId));
  return Boolean(watch && watch.expiresAt > Date.now());
}

export function observeFeishuSync(queryClient: QueryClient, doc: Doc) {
  const key = watchKey(doc.id);
  const watch = queryClient.getQueryData<FeishuSyncWatch>(key);
  if (!watch) return;

  if (doc.sync_status === "syncing") {
    if (!watch.observedSyncing) {
      queryClient.setQueryData<FeishuSyncWatch>(key, { ...watch, observedSyncing: true });
    }
    return;
  }

  const completed = watch.observedSyncing || doc.updated_at !== watch.baselineUpdatedAt;
  if (!completed) return;

  queryClient.removeQueries({ queryKey: key, exact: true });
  void queryClient.invalidateQueries({ queryKey: ["chunks", doc.id] });
}
