import type { Metadata } from "next";
import Link from "next/link";
import { Boxes } from "lucide-react";
import { OperationGuide } from "@/components/guide/operation-guide";

export const metadata: Metadata = {
  title: "操作说明 · 能力中心",
  description: "能力中心使用指南：飞书登录、团队能力复用、知识检索、模型体验与能力发布。",
};

export default function PublicGuidePage() {
  return (
    <div className="min-h-screen bg-zinc-50">
      <header className="border-b border-zinc-200 bg-white px-5 sm:px-8">
        <nav aria-label="说明页导航" className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4">
          <Link href="/" className="flex items-center gap-2 text-sm font-semibold"><Boxes className="size-6 text-blue-600" />能力中心</Link>
          <Link href="/login" className="text-sm font-medium text-blue-700">登录平台</Link>
        </nav>
      </header>
      <main className="px-5 py-8 sm:px-8 sm:py-12"><OperationGuide /></main>
    </div>
  );
}
