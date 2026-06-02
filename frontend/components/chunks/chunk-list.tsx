"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { useChunks } from "@/lib/hooks/use-chunks";

const PAGE_SIZE = 10;

export function ChunkList({ docId }: { docId: string }) {
  const [page, setPage] = useState(0);
  const { data, isLoading, isError, error } = useChunks(docId, PAGE_SIZE, page * PAGE_SIZE);

  if (isLoading) return <p className="text-muted-foreground">加载中...</p>;
  if (isError) return <p className="text-destructive">加载失败：{(error as Error).message}</p>;
  if (!data || data.items.length === 0) return <p className="text-muted-foreground">无切片</p>;

  const totalPages = Math.ceil(data.total / PAGE_SIZE);

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        共 {data.total} 个切片 · 第 {page + 1} / {totalPages} 页
      </p>

      <div className="space-y-3">
        {data.items.map((c) => (
          <div key={c.id} className="rounded-md border bg-card p-4">
            <div className="text-xs text-muted-foreground mb-2">
              #{c.seq} · {c.token_count} tokens
            </div>
            <pre className="whitespace-pre-wrap text-sm leading-relaxed font-sans">{c.content}</pre>
          </div>
        ))}
      </div>

      <div className="flex justify-between items-center pt-2">
        <Button size="sm" variant="outline" disabled={page === 0} onClick={() => setPage(p => p - 1)}>上一页</Button>
        <Button size="sm" variant="outline" disabled={page + 1 >= totalPages} onClick={() => setPage(p => p + 1)}>下一页</Button>
      </div>
    </div>
  );
}
