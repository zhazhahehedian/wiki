"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { knowledge } from "@/lib/api/knowledge";

export const panel = "rounded-xl border border-zinc-200 bg-white p-5";

export const documentStatusLabels: Record<string, string> = {
  pending: "等待索引",
  processing: "正在索引",
  ready: "可检索",
  failed: "处理失败",
  legacy_requires_reindex: "旧资料：请上传新版本完成质量核验",
};

export function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <p role="alert" className="my-3 text-sm text-red-700">
      {error instanceof Error ? error.message : "操作失败，请重试"}
    </p>
  ) : null;
}

export function useKnowledgeBases() {
  return useQuery({
    queryKey: ["knowledge", "bases"],
    queryFn: ({ signal }) => knowledge.bases(signal),
  });
}

export function useInvalidateKnowledge() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: ["knowledge"] });
}
