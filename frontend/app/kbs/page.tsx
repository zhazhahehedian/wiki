"use client";

import { Library } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { useKbs } from "@/lib/hooks/use-kbs";

export default function KbsIndexPage() {
  const { data } = useKbs();
  const hasKbs = (data?.items.length ?? 0) > 0;

  return (
    <main className="flex flex-1 items-center justify-center">
      <EmptyState
        icon={Library}
        title={hasKbs ? "选择一个知识库" : "创建第一个知识库"}
        description={
          hasKbs
            ? "从左侧列表选择知识库，管理它的文档。"
            : "点击左侧「新建知识库」按钮，然后上传文档开始构建。"
        }
      />
    </main>
  );
}
