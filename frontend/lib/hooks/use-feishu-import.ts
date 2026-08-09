"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { docApi } from "@/lib/api/docs";
import { beginFeishuSyncWatch } from "./feishu-sync-watch";

export function useFeishuImport(kbId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (url: string) => docApi.importFeishu(kbId, url),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["docs", kbId] }),
  });
}

export function useFeishuSync(kbId: string, docId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => docApi.syncFeishu(docId),
    onSuccess: async (doc) => {
      beginFeishuSyncWatch(queryClient, doc);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["docs", kbId] }),
        queryClient.invalidateQueries({ queryKey: ["doc", docId] }),
      ]);
    },
  });
}
