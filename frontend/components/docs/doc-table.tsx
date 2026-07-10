"use client";

import Link from "next/link";
import { RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";

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
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { IngestStatusBadge } from "./ingest-status-badge";
import { useDeleteDoc, useReingestDoc } from "@/lib/hooks/use-docs";
import type { Doc } from "@/lib/schemas";

function fmt(bytes: number): string {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / 1024 / 1024).toFixed(1) + " MB";
}

export function DocTable({ kbId, docs }: { kbId: string; docs: Doc[] }) {
  const del = useDeleteDoc(kbId);
  const reingest = useReingestDoc(kbId);

  function onReingest(d: Doc) {
    reingest.mutate(d.id, {
      onSuccess: () => toast.success("已重新入队处理"),
      onError: (e) => toast.error(`重新处理失败：${(e as Error).message}`),
    });
  }

  function onDelete(d: Doc) {
    del.mutate(d.id, {
      onSuccess: () => toast.success("已删除"),
      onError: (e) => toast.error(`删除失败：${(e as Error).message}`),
    });
  }

  return (
    <Table>
      <TableHeader>
        <TableRow className="bg-muted/50">
          <TableHead>文件</TableHead>
          <TableHead>状态</TableHead>
          <TableHead>大小</TableHead>
          <TableHead>上传时间</TableHead>
          <TableHead className="w-24" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {docs.map((d) => (
          <TableRow key={d.id}>
            <TableCell>
              <Link href={`/kbs/${kbId}/docs/${d.id}`} className="font-medium hover:text-primary hover:underline">
                {d.title}
              </Link>
              {d.status === "failed" && d.error_message && (
                <p className="mt-1 line-clamp-1 text-xs text-destructive">{d.error_message}</p>
              )}
            </TableCell>
            <TableCell><IngestStatusBadge status={d.status} /></TableCell>
            <TableCell className="text-muted-foreground">{fmt(d.bytes)}</TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {new Date(d.created_at).toLocaleString("zh-CN")}
            </TableCell>
            <TableCell>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label="重新处理"
                      onClick={() => onReingest(d)}
                      disabled={reingest.isPending || (d.status !== "ready" && d.status !== "failed")}
                    >
                      <RefreshCw className="size-4" />
                    </Button>
                  }
                />
                <TooltipContent>重新处理</TooltipContent>
              </Tooltip>
              <AlertDialog>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <AlertDialogTrigger
                        render={
                          <Button size="icon-sm" variant="ghost" aria-label="删除" disabled={del.isPending}>
                            <Trash2 className="size-4" />
                          </Button>
                        }
                      />
                    }
                  />
                  <TooltipContent>删除</TooltipContent>
                </Tooltip>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>删除文档「{d.title}」？</AlertDialogTitle>
                    <AlertDialogDescription>切片会一并删除，此操作不可撤销。</AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>取消</AlertDialogCancel>
                    <AlertDialogAction disabled={del.isPending} onClick={() => onDelete(d)}>
                      删除
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
