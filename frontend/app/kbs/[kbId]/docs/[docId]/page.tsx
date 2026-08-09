"use client";

import Link from "next/link";
import { use } from "react";
import { ArrowLeft, ExternalLink, RefreshCw } from "lucide-react";
import { toast } from "sonner";

import { ChunkList } from "@/components/chunks/chunk-list";
import { IngestStatusBadge } from "@/components/docs/ingest-status-badge";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useFeishuSync } from "@/lib/hooks/use-feishu-import";
import { useDoc } from "@/lib/hooks/use-docs";

const sourceLabels: Record<string, string> = {
  "local-upload": "本地上传",
  "feishu-docx": "飞书文档",
  "feishu-sheet": "飞书表格",
  "feishu-bitable": "飞书多维表格",
  "feishu-wiki": "飞书知识库",
};

export default function DocDetailPage({ params }: { params: Promise<{ kbId: string; docId: string }> }) {
  const { kbId, docId } = use(params);
  const { data: doc, isLoading, isError, error } = useDoc(docId);
  const sync = useFeishuSync(kbId, docId);

  function onSync() {
    sync.mutate(undefined, {
      onSuccess: () => toast.success("已加入飞书同步队列"),
      onError: (syncError) => toast.error(`同步失败：${(syncError as Error).message}`),
    });
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
      <div className="mx-auto max-w-4xl">
        <Link href={`/kbs/${kbId}/docs`} className="mb-4 inline-flex items-center text-sm text-muted-foreground hover:text-foreground">
          <ArrowLeft className="mr-1 size-4" /> 返回文档列表
        </Link>

        {isLoading && (
          <div className="space-y-3">
            <Skeleton className="h-8 w-64 max-w-full" />
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-32 w-full" />
          </div>
        )}
        {isError && <p className="text-destructive">加载失败：{(error as Error).message}</p>}

        {doc && (
          <>
            <header className="mb-6 space-y-3">
              <div className="flex flex-wrap items-center gap-2">
                <h1 className="min-w-0 break-words text-xl font-bold sm:text-2xl">{doc.title}</h1>
                <IngestStatusBadge status={doc.status} />
                {doc.source_type.startsWith("feishu-") && (
                  <Badge
                    variant="secondary"
                    className={doc.sync_status === "failed"
                      ? "bg-destructive/10 text-destructive"
                      : doc.sync_status === "syncing"
                        ? "bg-primary/10 text-primary"
                        : "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"}
                  >
                    {doc.sync_status === "failed" ? "同步失败" : doc.sync_status === "syncing" ? "同步中" : "已同步"}
                  </Badge>
                )}
              </div>

              <div className="flex flex-col gap-2 border-y py-3 text-xs text-muted-foreground sm:flex-row sm:flex-wrap sm:items-center sm:gap-x-4">
                <span>{sourceLabels[doc.source_type] ?? doc.source_type}</span>
                <span>{doc.mime_type} · {(doc.bytes / 1024).toFixed(1)} KB</span>
                {doc.remote_revision && <span>远端版本 {doc.remote_revision}</span>}
                {doc.last_synced_at && <span>上次同步 {new Date(doc.last_synced_at).toLocaleString("zh-CN")}</span>}
                {doc.source_url && (
                  <a
                    href={doc.source_url}
                    target="_blank"
                    rel="noreferrer"
                    aria-label="打开飞书原文"
                    className="inline-flex items-center gap-1 text-primary hover:underline"
                  >
                    打开原文 <ExternalLink className="size-3" />
                  </a>
                )}
              </div>

              {doc.status === "failed" && doc.error_message && (
                <Alert variant="destructive">
                  <AlertDescription>{doc.error_message}</AlertDescription>
                </Alert>
              )}

              {doc.sync_status === "failed" && (
                <Alert variant="destructive">
                  <AlertDescription>{doc.last_sync_error ?? "飞书同步失败，当前仍保留上次成功内容。"}</AlertDescription>
                </Alert>
              )}

              {doc.source_type.startsWith("feishu-") && (
                <Button
                  variant="outline"
                  onClick={onSync}
                  disabled={sync.isPending || doc.sync_status === "syncing"}
                  aria-label={doc.sync_status === "failed" ? "重试同步" : "同步飞书文档"}
                >
                  <RefreshCw className={sync.isPending ? "animate-spin" : ""} data-icon="inline-start" />
                  {doc.sync_status === "failed" ? "重试同步" : "立即同步"}
                </Button>
              )}
            </header>

            <section>
              <h2 className="mb-3 text-lg font-semibold">切片预览</h2>
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
    </div>
  );
}
