"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { chunkApi } from "@/lib/api/chunks";

export function useChunks(docId: string, limit = 20, offset = 0) {
  return useQuery({
    queryKey: ["chunks", docId, { limit, offset }],
    queryFn: () => chunkApi.listByDoc(docId, limit, offset),
    enabled: !!docId,
    placeholderData: keepPreviousData,
  });
}
