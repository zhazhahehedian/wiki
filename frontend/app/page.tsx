"use client";

import { KBCard } from "@/components/kb/kb-card";
import { KBCreateDialog } from "@/components/kb/kb-create-dialog";
import { useKbs } from "@/lib/hooks/use-kbs";

export default function Home() {
  const { data, isLoading, isError, error } = useKbs();

  return (
    <main className="container mx-auto max-w-6xl p-8">
      <header className="flex items-center justify-between mb-8">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">it-wiki</h1>
          <p className="text-muted-foreground text-sm">团队知识库 Agent</p>
        </div>
        <KBCreateDialog />
      </header>

      {isLoading && <p className="text-muted-foreground">加载中...</p>}
      {isError && <p className="text-destructive">加载失败：{(error as Error).message}</p>}
      {data && data.items.length === 0 && (
        <div className="text-center py-16 text-muted-foreground">
          <p className="mb-2">还没有知识库</p>
          <p className="text-sm">点击右上角&quot;新建知识库&quot;开始</p>
        </div>
      )}
      {data && data.items.length > 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {data.items.map((kb) => <KBCard key={kb.id} kb={kb} />)}
        </div>
      )}
      {data && (
        <p className="mt-6 text-xs text-muted-foreground">共 {data.total} 个</p>
      )}
    </main>
  );
}
