"use client";

import { Library } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { useKbs } from "@/lib/hooks/use-kbs";

export default function KbsIndexPage() {
  const { data, isLoading, isError } = useKbs();
  const hasKbs = (data?.items.length ?? 0) > 0;

  const title = isLoading || isError ? "知识库" : hasKbs ? "选择一个知识库" : "创建第一个知识库";
  const description = isLoading
    ? "正在加载知识库列表…"
    : isError
      ? "知识库列表加载失败，请在左侧面板查看详情。"
      : hasKbs
        ? "从左侧列表选择知识库，管理它的文档。"
        : "点击左侧「新建知识库」按钮，然后上传文档开始构建。";

  return (
    <div className="flex h-full flex-col">
      <header className="flex min-h-14 items-center gap-2 border-b px-4">
        <SidebarTrigger className="md:hidden" />
        <div>
          <h1 className="text-base font-semibold">知识库</h1>
          <p className="text-xs text-muted-foreground">选择或创建知识库</p>
        </div>
      </header>

      <div className="flex flex-1 items-center justify-center">
        <EmptyState icon={Library} title={title} description={description} />
      </div>
    </div>
  );
}
