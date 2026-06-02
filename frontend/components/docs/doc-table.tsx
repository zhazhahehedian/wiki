"use client";

import Link from "next/link";
import { Trash2 } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { IngestStatusBadge } from "./ingest-status-badge";
import { useDeleteDoc } from "@/lib/hooks/use-docs";
import type { Doc } from "@/lib/schemas";
import { toast } from "sonner";

export function DocTable({ kbId, docs }: { kbId: string; docs: Doc[] }) {
  const del = useDeleteDoc(kbId);

  function fmt(bytes: number): string {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / 1024 / 1024).toFixed(1) + " MB";
  }

  function onDelete(d: Doc) {
    if (!confirm(`删除文档 "${d.title}"？切片会一并删除。`)) return;
    del.mutate(d.id, {
      onSuccess: () => toast.success("已删除"),
      onError: (e) => toast.error("删除失败: " + (e as Error).message),
    });
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>文件</TableHead>
          <TableHead>状态</TableHead>
          <TableHead>大小</TableHead>
          <TableHead>上传时间</TableHead>
          <TableHead className="w-12" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {docs.map((d) => (
          <TableRow key={d.id}>
            <TableCell>
              <Link href={`/kbs/${kbId}/docs/${d.id}`} className="font-medium hover:underline">
                {d.title}
              </Link>
              {d.status === "failed" && d.error_message && (
                <p className="text-xs text-destructive mt-1 line-clamp-1">{d.error_message}</p>
              )}
            </TableCell>
            <TableCell><IngestStatusBadge status={d.status} /></TableCell>
            <TableCell className="text-muted-foreground">{fmt(d.bytes)}</TableCell>
            <TableCell className="text-muted-foreground text-xs">
              {new Date(d.created_at).toLocaleString()}
            </TableCell>
            <TableCell>
              <Button size="sm" variant="ghost" onClick={() => onDelete(d)} disabled={del.isPending}>
                <Trash2 className="size-4" />
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
