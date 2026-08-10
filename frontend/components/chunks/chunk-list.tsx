"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useChunks } from "@/lib/hooks/use-chunks";

const PAGE_SIZE = 10;

export function ChunkList({ docId }: { docId: string }) {
  const [page, setPage] = useState(0);
  const { data, isLoading, isError, error } = useChunks(docId, PAGE_SIZE, page * PAGE_SIZE);

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }
  if (isError) return <p className="text-sm text-destructive">加载失败：{(error as Error).message}</p>;
  if (!data || data.total === 0) return <p className="text-sm text-muted-foreground">无切片</p>;

  const totalPages = Math.ceil(data.total / PAGE_SIZE);

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        共 {data.total} 个切片 · 第 {page + 1} / {totalPages} 页
      </p>

      <div className="space-y-3">
        {data.items.map((c) => (
          <div key={c.id} className="rounded-lg border bg-card p-4 shadow-sm">
            <div className="mb-2 text-xs text-muted-foreground">
              #{c.seq} · {c.token_count} tokens
            </div>
            <pre className="whitespace-pre-wrap font-sans text-sm leading-relaxed">{c.content}</pre>
          </div>
        ))}
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button size="sm" variant="outline" disabled={page === 0} onClick={() => setPage(p => p - 1)}>上一页</Button>
        <Button size="sm" variant="outline" disabled={page + 1 >= totalPages} onClick={() => setPage(p => p + 1)}>下一页</Button>
      </div>
    </div>
  );
}
