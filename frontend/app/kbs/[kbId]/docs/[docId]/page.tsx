"use client";

import Link from "next/link";
import { use } from "react";
import { ArrowLeft } from "lucide-react";
import { useDoc } from "@/lib/hooks/use-docs";
import { IngestStatusBadge } from "@/components/docs/ingest-status-badge";
import { ChunkList } from "@/components/chunks/chunk-list";
import { Skeleton } from "@/components/ui/skeleton";

export default function DocDetailPage({ params }: { params: Promise<{ kbId: string; docId: string }> }) {
  const { kbId, docId } = use(params);
  const { data: doc, isLoading, isError, error } = useDoc(docId);

  return (
    <main className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
      <div className="mx-auto max-w-4xl">
      <Link href={`/kbs/${kbId}/docs`} className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4">
        <ArrowLeft className="size-4 mr-1" /> 返回文档列表
      </Link>

      {isLoading && (
        <div className="space-y-3">
          <Skeleton className="h-8 w-64" />
          <Skeleton className="h-4 w-40" />
          <Skeleton className="h-32 w-full" />
        </div>
      )}
      {isError && <p className="text-destructive">加载失败：{(error as Error).message}</p>}

      {doc && (
        <>
          <header className="mb-6">
            <div className="flex items-center gap-3 mb-1">
              <h1 className="text-2xl font-bold">{doc.title}</h1>
              <IngestStatusBadge status={doc.status} />
            </div>
            <div className="text-xs text-muted-foreground">
              {doc.mime_type} · {(doc.bytes / 1024).toFixed(1)} KB · {doc.checksum.slice(0, 24)}...
            </div>
            {doc.status === "failed" && doc.error_message && (
              <p className="mt-2 text-sm text-destructive bg-destructive/10 p-2 rounded">
                {doc.error_message}
              </p>
            )}
          </header>

          <section>
            <h2 className="text-lg font-semibold mb-3">切片预览</h2>
            {doc.status === "ready" ? (
              <ChunkList docId={docId} />
            ) : doc.status === "failed" ? (
              <p className="text-muted-foreground">摄入失败，无切片可显示</p>
            ) : (
              <p className="text-muted-foreground">摄入中，请稍候...</p>
            )}
          </section>
        </>
      )}
      </div>
    </main>
  );
}
