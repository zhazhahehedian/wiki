"use client";

import { use } from "react";
import { FileText } from "lucide-react";

import { DocTable } from "@/components/docs/doc-table";
import { DocUploader } from "@/components/docs/doc-uploader";
import { EmptyState } from "@/components/common/empty-state";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocsByKB } from "@/lib/hooks/use-docs";
import { useKbs } from "@/lib/hooks/use-kbs";

export default function KBDocsPage({ params }: { params: Promise<{ kbId: string }> }) {
  const { kbId } = use(params);
  const { data, isLoading, isError, error } = useDocsByKB(kbId);
  const kbs = useKbs();
  const kbName = kbs.data?.items.find((kb) => kb.id === kbId)?.name;

  return (
    <div className="flex h-full flex-col">
      <header className="flex min-h-14 items-center gap-2 border-b px-4">
        <SidebarTrigger className="md:hidden" />
        <div>
          <h1 className="text-base font-semibold">{kbName ?? "文档管理"}</h1>
          <p className="text-xs text-muted-foreground">上传与管理该知识库的文档</p>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
        <div className="mx-auto max-w-4xl space-y-6">
          <DocUploader kbId={kbId} />

          {isLoading && (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          )}
          {isError && <p className="text-sm text-destructive">加载失败：{(error as Error).message}</p>}
          {data && data.items.length === 0 && (
            <EmptyState icon={FileText} title="还没有文档" description="拖入或选择文件上传，处理完成后即可对话检索。" />
          )}
          {data && data.items.length > 0 && (
            <div className="rounded-lg border border-border/50">
              <DocTable kbId={kbId} docs={data.items} />
            </div>
          )}
          {data && data.items.length > 0 && (
            <p className="text-xs text-muted-foreground">共 {data.total} 个文档</p>
          )}
        </div>
      </div>
    </div>
  );
}
