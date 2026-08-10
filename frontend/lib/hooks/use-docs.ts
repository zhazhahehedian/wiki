"use client";

import { useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { QueryClient } from "@tanstack/react-query";
import { docApi } from "@/lib/api/docs";
import type { Doc } from "@/lib/schemas";
import { feishuSyncWatchInterval, observeFeishuSync } from "./feishu-sync-watch";

const TERMINAL_STATUSES: Doc["status"][] = ["ready", "failed"];
const ACTIVE_DOC_REFETCH_INTERVAL_MS = 2_000;

export function docNeedsRefresh(doc: Doc, manualSyncPending = false): boolean {
  return manualSyncPending || !TERMINAL_STATUSES.includes(doc.status) || doc.sync_status === "syncing";
}

export function docRefetchInterval(queryClient: QueryClient, doc: Doc): number | false {
  if (docNeedsRefresh(doc)) return ACTIVE_DOC_REFETCH_INTERVAL_MS;
  return feishuSyncWatchInterval(queryClient, doc.id);
}

export function docsRefetchInterval(queryClient: QueryClient, docs: readonly Doc[]): number | false {
  return docs.reduce<number | false>((shortest, doc) => {
    const interval = docRefetchInterval(queryClient, doc);
    if (interval === false) return shortest;
    return shortest === false ? interval : Math.min(shortest, interval);
  }, false);
}

export function useDocsByKB(kbId: string, limit = 20, offset = 0) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["docs", kbId, { limit, offset }],
    queryFn: () => docApi.listByKB(kbId, { limit, offset }),
    enabled: !!kbId,
    refetchInterval: (q) => {
      const items = q.state.data?.items ?? [];
      return docsRefetchInterval(queryClient, items);
    },
  });
  useEffect(() => {
    query.data?.items.forEach((doc) => observeFeishuSync(queryClient, doc));
  }, [query.data, queryClient]);
  return query;
}

export function useDoc(id: string) {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["doc", id],
    queryFn: () => docApi.get(id),
    enabled: !!id,
    refetchInterval: (q) => {
      const d = q.state.data;
      if (!d) return ACTIVE_DOC_REFETCH_INTERVAL_MS;
      return docRefetchInterval(queryClient, d);
    },
  });
  useEffect(() => {
    if (query.data) observeFeishuSync(queryClient, query.data);
  }, [query.data, queryClient]);
  return query;
}

export function useUploadDoc(kbId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (file: File) => docApi.upload(kbId, file),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["docs", kbId] });
    },
  });
}

export function useDeleteDoc(kbId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => docApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["docs", kbId] }),
  });
}

export function useReingestDoc(kbId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => docApi.reingest(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["docs", kbId] }),
  });
}
