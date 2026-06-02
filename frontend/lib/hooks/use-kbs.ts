"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { kbApi } from "@/lib/api/kb";

export function useKbs(limit = 20, offset = 0) {
  return useQuery({
    queryKey: ["kbs", { limit, offset }],
    queryFn: () => kbApi.list(limit, offset),
  });
}

export function useCreateKb() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { name: string; description?: string }) => kbApi.create(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["kbs"] }),
  });
}

export function useDeleteKb() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => kbApi.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["kbs"] }),
  });
}
