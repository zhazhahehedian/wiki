"use client";

import Link from "next/link";
import { use } from "react";
import { ArrowLeft } from "lucide-react";
import { DocUploader } from "@/components/docs/doc-uploader";
import { DocTable } from "@/components/docs/doc-table";
import { useDocsByKB } from "@/lib/hooks/use-docs";

export default function KBDocsPage({ params }: { params: Promise<{ kbId: string }> }) {
  const { kbId } = use(params);
  const { data, isLoading, isError, error } = useDocsByKB(kbId);

  return (
    <main className="container mx-auto max-w-5xl p-8">
      <Link href="/" className="inline-flex items-center text-sm text-muted-foreground hover:text-foreground mb-4">
        <ArrowLeft className="size-4 mr-1" /> 返回知识库列表
      </Link>

      <h1 className="text-2xl font-bold mb-6">文档</h1>

      <div className="mb-6">
        <DocUploader kbId={kbId} />
      </div>

      {isLoading && <p className="text-muted-foreground">加载中...</p>}
      {isError && <p className="text-destructive">加载失败：{(error as Error).message}</p>}
      {data && data.items.length === 0 && (
        <p className="text-center py-12 text-muted-foreground">还没有文档</p>
      )}
      {data && data.items.length > 0 && (
        <>
          <DocTable kbId={kbId} docs={data.items} />
          <p className="mt-4 text-xs text-muted-foreground">共 {data.total} 个文档</p>
        </>
      )}
    </main>
  );
}
