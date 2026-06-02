"use client";

import Link from "next/link";
import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useDeleteKb } from "@/lib/hooks/use-kbs";
import type { KB } from "@/lib/schemas";
import { toast } from "sonner";

export function KBCard({ kb }: { kb: KB }) {
  const del = useDeleteKb();

  function handleDelete() {
    if (!confirm(`删除知识库 "${kb.name}"？所有文档和切片会一并删除。`)) return;
    del.mutate(kb.id, {
      onSuccess: () => toast.success("已删除"),
      onError: (e) => toast.error("删除失败: " + (e as Error).message),
    });
  }

  return (
    <div className="rounded-lg border bg-card p-4 flex flex-col gap-2">
      <Link href={`/kbs/${kb.id}/docs`} className="text-lg font-semibold hover:underline">
        {kb.name}
      </Link>
      <p className="text-sm text-muted-foreground line-clamp-2">{kb.description || "（无描述）"}</p>
      <div className="text-xs text-muted-foreground">
        {kb.embed_model} · dim={kb.embed_dim} · {new Date(kb.created_at).toLocaleDateString()}
      </div>
      <div className="mt-auto pt-2 flex justify-end">
        <Button size="sm" variant="ghost" onClick={handleDelete} disabled={del.isPending}>
          <Trash2 className="size-4" />
        </Button>
      </div>
    </div>
  );
}
