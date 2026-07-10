"use client";

import { useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";

import { KBCreateDialog } from "@/components/kb/kb-create-dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useDeleteKb, useKbs } from "@/lib/hooks/use-kbs";
import { clearLastKbId } from "@/lib/last-kb";
import { cn } from "@/lib/utils";
import type { KB } from "@/lib/schemas";

function KbRow({ kb, active }: { kb: KB; active: boolean }) {
  const [open, setOpen] = useState(false);
  const del = useDeleteKb();

  return (
    <div
      className={cn(
        "group flex items-center gap-1 rounded-md border-l-2 border-transparent pr-1 transition-colors hover:bg-accent",
        active && "border-primary bg-accent",
      )}
    >
      <Link href={`/kbs/${kb.id}/docs`} className="min-w-0 flex-1 px-3 py-2">
        <span className="block truncate text-sm font-medium">{kb.name}</span>
        <span className="block truncate text-xs text-muted-foreground">{kb.description || "（无描述）"}</span>
      </Link>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <AlertDialogTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`删除 ${kb.name}`}
              className="opacity-0 transition-opacity group-hover:opacity-100"
            >
              <Trash2 className="size-3.5" />
            </Button>
          }
        />
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除知识库「{kb.name}」？</AlertDialogTitle>
            <AlertDialogDescription>其中所有文档和切片会一并删除，此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={del.isPending}
              onClick={() =>
                del.mutate(kb.id, {
                  onSuccess: () => {
                    setOpen(false);
                    clearLastKbId(kb.id);
                    toast.success("已删除");
                  },
                  onError: (e) => toast.error(`删除失败：${(e as Error).message}`),
                })
              }
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export function KbPanel() {
  const params = useParams<{ kbId?: string }>();
  const { data, isLoading, isError, error } = useKbs();

  return (
    <div className="flex h-full flex-col">
      <div className="border-b p-3">
        <KBCreateDialog />
      </div>
      <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
        {isLoading && (
          <div className="space-y-2 p-1">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        )}
        {isError && <p className="p-2 text-xs text-destructive">{(error as Error).message}</p>}
        {data?.items.length === 0 && (
          <p className="p-2 text-xs leading-relaxed text-muted-foreground">还没有知识库，点击上方按钮创建。</p>
        )}
        {data?.items.map((kb) => (
          <KbRow key={kb.id} kb={kb} active={kb.id === params.kbId} />
        ))}
      </nav>
    </div>
  );
}
