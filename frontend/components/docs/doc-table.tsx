"use client";

import { useState } from "react";
import Link from "next/link";
import { ExternalLink, RefreshCw, Trash2 } from "lucide-react";
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
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useFeishuSync } from "@/lib/hooks/use-feishu-import";
import { useDeleteDoc, useReingestDoc } from "@/lib/hooks/use-docs";
import type { Doc, DocSyncStatus } from "@/lib/schemas";
import { IngestStatusBadge } from "./ingest-status-badge";

const sourceLabels: Record<string, string> = {
  "local-upload": "本地上传",
  "feishu-docx": "飞书文档",
  "feishu-sheet": "飞书表格",
  "feishu-bitable": "飞书多维表格",
  "feishu-wiki": "飞书知识库",
};

function fmt(bytes: number): string {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / 1024 / 1024).toFixed(1) + " MB";
}

function isFeishuDoc(doc: Doc): boolean {
  return doc.source_type.startsWith("feishu-");
}

function SyncStatusBadge({ status, hasSynced }: { status: DocSyncStatus; hasSynced: boolean }) {
  const labels = {
    idle: hasSynced ? "已同步" : "等待同步",
    syncing: "同步中",
    failed: "同步失败",
  } as const;
  const tones = {
    idle: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
    syncing: "bg-primary/10 text-primary",
    failed: "bg-destructive/10 text-destructive",
  } as const;
  return <Badge variant="secondary" className={tones[status]}>{labels[status]}</Badge>;
}

function DocRowActions({ kbId, doc }: { kbId: string; doc: Doc }) {
  const [open, setOpen] = useState(false);
  const del = useDeleteDoc(kbId);
  const reingest = useReingestDoc(kbId);
  const sync = useFeishuSync(kbId, doc.id);
  const remote = isFeishuDoc(doc);

  function onRetry() {
    if (remote) {
      sync.mutate(undefined, {
        onSuccess: () => toast.success("已加入飞书同步队列"),
        onError: (error) => toast.error(`同步失败：${(error as Error).message}`),
      });
      return;
    }
    reingest.mutate(doc.id, {
      onSuccess: () => toast.success("已重新入队处理"),
      onError: (error) => toast.error(`重新处理失败：${(error as Error).message}`),
    });
  }

  function onDelete() {
    del.mutate(doc.id, {
      onSuccess: () => {
        setOpen(false);
        toast.success("已删除");
      },
      onError: (error) => toast.error(`删除失败：${(error as Error).message}`),
    });
  }

  const retryLabel = remote
    ? doc.sync_status === "failed" ? "重试同步" : "同步飞书文档"
    : "重新处理";
  const retryDisabled = remote
    ? sync.isPending || doc.sync_status === "syncing"
    : reingest.isPending || (doc.status !== "ready" && doc.status !== "failed");

  return (
    <div className="flex gap-1">
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={retryLabel}
              onClick={onRetry}
              disabled={retryDisabled}
            >
              <RefreshCw className={sync.isPending ? "size-4 animate-spin" : "size-4"} />
            </Button>
          }
        />
        <TooltipContent>{retryLabel}</TooltipContent>
      </Tooltip>
      <AlertDialog open={open} onOpenChange={setOpen}>
        <Tooltip>
          <TooltipTrigger
            render={
              <AlertDialogTrigger
                render={<Button size="icon-sm" variant="ghost" aria-label="删除" disabled={del.isPending} />}
              >
                <Trash2 className="size-4" />
              </AlertDialogTrigger>
            }
          />
          <TooltipContent>删除</TooltipContent>
        </Tooltip>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除文档「{doc.title}」？</AlertDialogTitle>
            <AlertDialogDescription>切片会一并删除，此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction disabled={del.isPending} onClick={onDelete}>删除</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export function DocTable({ kbId, docs }: { kbId: string; docs: Doc[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow className="bg-muted/50">
          <TableHead>文档</TableHead>
          <TableHead>来源</TableHead>
          <TableHead>摄入状态</TableHead>
          <TableHead>同步状态</TableHead>
          <TableHead>大小</TableHead>
          <TableHead className="w-24" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {docs.map((doc) => {
          const remote = isFeishuDoc(doc);
          return (
            <TableRow key={doc.id}>
              <TableCell className="min-w-52 whitespace-normal">
                <Link href={`/kbs/${kbId}/docs/${doc.id}`} className="font-medium hover:text-primary hover:underline">
                  {doc.title}
                </Link>
                {(doc.status === "failed" && doc.error_message) && (
                  <p className="mt-1 line-clamp-1 text-xs text-destructive">{doc.error_message}</p>
                )}
                {(doc.sync_status === "failed" && doc.last_sync_error) && (
                  <p className="mt-1 line-clamp-1 text-xs text-destructive">{doc.last_sync_error}</p>
                )}
              </TableCell>
              <TableCell>
                <div className="flex min-w-36 flex-col items-start gap-1">
                  <span>{sourceLabels[doc.source_type] ?? (remote ? "飞书资源" : doc.source_type)}</span>
                  {doc.source_url && (
                    <a
                      href={doc.source_url}
                      target="_blank"
                      rel="noreferrer"
                      aria-label="打开飞书原文"
                      className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
                    >
                      原文 <ExternalLink className="size-3" />
                    </a>
                  )}
                  {doc.remote_revision && (
                    <span className="text-xs text-muted-foreground">
                      远端版本 {doc.remote_revision}
                    </span>
                  )}
                  {doc.last_synced_at && (
                    <span className="text-xs text-muted-foreground">
                      上次同步 {new Date(doc.last_synced_at).toLocaleString("zh-CN")}
                    </span>
                  )}
                </div>
              </TableCell>
              <TableCell><IngestStatusBadge status={doc.status} /></TableCell>
              <TableCell>
                {remote
                  ? <SyncStatusBadge status={doc.sync_status ?? "idle"} hasSynced={Boolean(doc.last_synced_at)} />
                  : <span className="text-xs text-muted-foreground">不适用</span>}
              </TableCell>
              <TableCell className="text-muted-foreground">{fmt(doc.bytes)}</TableCell>
              <TableCell><DocRowActions kbId={kbId} doc={doc} /></TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
