"use client";

import { useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { CircleAlert, TriangleAlert } from "lucide-react";
import { knowledge, locationLabel } from "@/lib/api/knowledge";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ErrorMessage, StatusBadge } from "./shared";

/** Parsed content, positions and quality diagnostics of one document version. */
export function VersionPreview({ id }: { id: string }) {
  const version = useQuery({
    queryKey: ["knowledge", "version", id],
    queryFn: ({ signal }) => knowledge.preview(id, signal),
    refetchInterval: (q) =>
      q.state.data?.status === "processing" ||
      q.state.data?.status === "pending"
        ? 2000
        : false,
  });
  const data = version.data;
  return (
    <section aria-label="解析预览" className="space-y-4">
      <ErrorMessage error={version.error} />
      {version.isPending && (
        <div role="status" aria-label="正在加载预览" className="space-y-3">
          <Skeleton className="h-5 w-2/3" />
          <Skeleton className="h-24" />
          <Skeleton className="h-24" />
        </div>
      )}
      {data && (
        <>
          <div className="space-y-2 rounded-lg border border-zinc-200 p-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="min-w-0 truncate font-medium">{data.title}</span>
              <StatusBadge status={data.status} />
            </div>
            <div className="flex flex-wrap gap-1.5">
              <Badge variant="muted">
                {data.model} / {data.dim} 维
              </Badge>
              <Badge variant="muted">
                {data.preview.blocks.length} 个片段
              </Badge>
            </div>
            <p className="break-all font-mono text-[11px] text-zinc-400">
              版本 {id}
            </p>
          </div>
          {data.error && (
            <p
              role="alert"
              className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
            >
              {data.error}
            </p>
          )}
          {data.preview.diagnostics.length > 0 && (
            <ul className="space-y-2">
              {data.preview.diagnostics.map((d, i) => (
                <li
                  key={i}
                  className={
                    d.blocking
                      ? "flex gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
                      : "flex gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800"
                  }
                >
                  {d.blocking ? (
                    <CircleAlert className="mt-0.5 size-4 shrink-0" />
                  ) : (
                    <TriangleAlert className="mt-0.5 size-4 shrink-0" />
                  )}
                  <span>
                    {d.blocking ? "阻止索引" : "警告"}：
                    {d.page ? `第 ${d.page} 页：` : ""}
                    {d.message}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {!data.preview.blocks.length && (
            <p className="rounded-lg bg-zinc-50 p-4 text-sm text-zinc-500">
              尚无可预览正文。请等待解析完成，或查看失败诊断。
            </p>
          )}
          <ol className="space-y-3">
            {data.preview.blocks.map((b, i) => (
              <li
                key={i}
                className="rounded-lg border border-zinc-100 bg-zinc-50 p-4"
              >
                <p className="mb-2 flex items-center gap-2 text-xs text-zinc-500">
                  <span className="rounded bg-white px-1.5 py-0.5 font-mono text-[11px] text-zinc-400 ring-1 ring-zinc-200">
                    #{i + 1}
                  </span>
                  <span className="text-primary">{locationLabel(b.locator)}</span>
                </p>
                <pre className="whitespace-pre-wrap break-words font-sans text-sm leading-relaxed text-zinc-800">
                  {b.content}
                </pre>
              </li>
            ))}
          </ol>
        </>
      )}
    </section>
  );
}

/** Right-side drawer showing a version preview; closed when id is empty. */
export function VersionPreviewSheet({
  id,
  onClose,
}: {
  id: string;
  onClose: () => void;
}) {
  // Keep the last version rendered while the closing animation runs.
  const last = useRef(id);
  if (id) last.current = id;
  return (
    <Sheet
      open={!!id}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent className="w-full gap-0 sm:max-w-2xl data-[side=right]:sm:max-w-2xl">
        <SheetHeader className="border-b border-zinc-100 px-5 py-4 pr-12">
          <SheetTitle>解析预览与质量诊断</SheetTitle>
          <SheetDescription>
            核对解析出的正文、引用位置和阻止发布的原因。
          </SheetDescription>
        </SheetHeader>
        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          {last.current && <VersionPreview id={last.current} />}
        </div>
      </SheetContent>
    </Sheet>
  );
}
