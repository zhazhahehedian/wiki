"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { docApi } from "@/lib/api/docs";
import type { Doc } from "@/lib/schemas";

const TERMINAL_STATUSES: Doc["status"][] = ["ready", "failed"];

export function useDocsByKB(kbId: string, limit = 20, offset = 0) {
  return useQuery({
    queryKey: ["docs", kbId, { limit, offset }],
    queryFn: () => docApi.listByKB(kbId, { limit, offset }),
    enabled: !!kbId,
    refetchInterval: (q) => {
      const items = q.state.data?.items ?? [];
      const inProgress = items.some((d) => !TERMINAL_STATUSES.includes(d.status));
      return inProgress ? 2_000 : false;
    },
  });
}

export function useDoc(id: string) {
  return useQuery({
    queryKey: ["doc", id],
    queryFn: () => docApi.get(id),
    enabled: !!id,
    refetchInterval: (q) => {
      const d = q.state.data;
      if (!d) return 2_000;
      return TERMINAL_STATUSES.includes(d.status) ? false : 2_000;
    },
  });
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
