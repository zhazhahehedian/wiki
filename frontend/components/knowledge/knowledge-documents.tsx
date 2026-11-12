"use client";

import Link from "next/link";
import { useRef, useState, type DragEvent } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  ChevronRight,
  Eye,
  FileClock,
  FileUp,
  Loader2,
  MoreHorizontal,
  RotateCw,
  Search,
  Trash2,
  Upload,
  UploadCloud,
} from "lucide-react";
import { toast } from "sonner";
import { knowledge, type KnowledgeDocument } from "@/lib/api/knowledge";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { confirm } from "@/components/ui/confirm-dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import {
  ErrorMessage,
  errorText,
  FileIcon,
  StatusBadge,
  useInvalidateKnowledge,
  useKnowledgeBases,
  VisibilityBadge,
} from "./shared";
import { VersionPreviewSheet } from "./version-preview";

const accept = ".txt,.md,.markdown,.docx,.pdf";

export function KnowledgeDocuments({ id }: { id: string }) {
  const bases = useKnowledgeBases();
  const base = bases.data?.find((b) => b.id === id);
  const docs = useQuery({
    queryKey: ["knowledge", "documents", id],
    queryFn: ({ signal }) => knowledge.documents(id, signal),
    refetchInterval: (q) =>
      q.state.data?.some(
        (d) => d.status === "pending" || d.status === "processing",
      )
        ? 3000
        : false,
  });
  const invalidate = useInvalidateKnowledge();
  const [preview, setPreview] = useState("");
  const [uploading, setUploading] = useState("");
  const [uploadErrors, setUploadErrors] = useState<string[]>([]);
  const [dragging, setDragging] = useState(false);
  const file = useRef<HTMLInputElement>(null);
  // Document receiving a new version from the next file selection; null uploads new documents.
  const target = useRef<KnowledgeDocument | null>(null);
  const action = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: invalidate,
  });

  function choose(doc: KnowledgeDocument | null) {
    target.current = doc;
    if (file.current) {
      file.current.multiple = !doc;
      file.current.click();
    }
  }

  async function uploadFiles(files: File[], doc: KnowledgeDocument | null) {
    if (!files.length || uploading) return;
    setUploadErrors([]);
    const failed: string[] = [];
    let done = 0;
    for (const [i, f] of files.entries()) {
      setUploading(
        doc
          ? `正在上传“${doc.title}”的新版本…`
          : `正在上传 ${f.name}${files.length > 1 ? `（${i + 1}/${files.length}）` : ""}…`,
      );
      try {
        await knowledge.upload(id, f, doc?.id ?? "");
        done++;
      } catch (e) {
        failed.push(`${f.name}：${errorText(e)}`);
      }
    }
    setUploading("");
    setUploadErrors(failed);
    if (done)
      toast.success(
        doc
          ? "新版本已上传，解析成功后自动切换，旧版在此之前仍可检索"
          : `已上传 ${done} 个文件，正在解析和索引`,
      );
    await invalidate();
  }

  async function removeDocument(d: KnowledgeDocument) {
    if (
      await confirm({
        title: `删除“${d.title}”？`,
        description: "删除后新的检索请求将立即看不到该文档。",
        confirmText: "删除",
        destructive: true,
      })
    )
      action.mutate(() => knowledge.deleteDocument(d.id), {
        onSuccess: () => toast.success("文档已删除"),
      });
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    if (base?.manage) void uploadFiles([...e.dataTransfer.files], null);
  }

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-8">
      <nav
        aria-label="面包屑"
        className="flex items-center gap-1 text-sm text-zinc-500"
      >
        <Link
          href="/hub/knowledge"
          className="rounded px-1 hover:bg-zinc-100 hover:text-zinc-900"
        >
          知识库
        </Link>
        <ChevronRight className="size-3.5" />
        <span className="truncate px-1 text-zinc-900">
          {base?.name ?? "文档"}
        </span>
      </nav>

      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold">
            {base?.name ?? "知识库文档"}
          </h1>
          {base?.description && (
            <p className="mt-1.5 text-sm text-zinc-500">{base.description}</p>
          )}
          {base && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              <VisibilityBadge
                visibility={base.visibility}
                department={base.department}
              />
              {docs.data && (
                <Badge variant="muted">{docs.data.length} 个文档</Badge>
              )}
            </div>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Link
            href="/hub/knowledge/search"
            className={buttonVariants({ variant: "outline", size: "lg" })}
          >
            <Search />
            检索测试
          </Link>
          {base?.manage && (
            <Button
              size="lg"
              className="px-3.5"
              disabled={!!uploading}
              onClick={() => choose(null)}
            >
              <Upload />
              上传文档
            </Button>
          )}
        </div>
      </header>

      <input
        ref={file}
        aria-label="上传文档文件"
        type="file"
        accept={accept}
        className="sr-only"
        tabIndex={-1}
        onChange={(e) => {
          const files = [...(e.target.files ?? [])];
          e.target.value = "";
          void uploadFiles(files, target.current);
        }}
      />

      <ErrorMessage error={action.error || docs.error || bases.error} />
      {uploadErrors.length > 0 && (
        <ErrorMessage error={new Error(`上传失败：${uploadErrors.join("；")}`)} />
      )}

      {base?.manage && (
        <button
          type="button"
          disabled={!!uploading}
          onClick={() => choose(null)}
          onDragOver={(e) => {
            e.preventDefault();
            setDragging(true);
          }}
          onDragLeave={() => setDragging(false)}
          onDrop={onDrop}
          className={cn(
            "flex w-full flex-col items-center gap-2 rounded-xl border-2 border-dashed px-6 py-8 text-center transition",
            dragging
              ? "border-primary bg-primary/5"
              : "border-zinc-200 bg-white hover:border-primary/50 hover:bg-zinc-50",
            uploading && "cursor-progress",
          )}
        >
          {uploading ? (
            <>
              <Loader2 className="size-7 animate-spin text-primary" />
              <span role="status" className="text-sm font-medium">
                {uploading}
              </span>
            </>
          ) : (
            <>
              <UploadCloud className="size-7 text-zinc-400" />
              <span className="text-sm">
                <span className="font-medium text-primary">点击选择文件</span>
                ，或拖拽文件到这里
              </span>
              <span className="max-w-xl text-xs text-zinc-500">
                支持 TXT、Markdown、DOCX、文字型 PDF，单文件最大 20
                MiB。DOCX 中的图片由视觉模型识别为文字，请在预览中核对；扫描件和缺少正文的文件会阻止发布；更新失败时旧版仍可检索。
              </span>
            </>
          )}
        </button>
      )}

      <section className="overflow-hidden rounded-xl border border-zinc-200 bg-white">
        <div className="hidden grid-cols-[minmax(0,1fr)_10rem_11rem] gap-4 border-b border-zinc-100 bg-zinc-50/80 px-5 py-2.5 text-xs font-medium text-zinc-500 md:grid">
          <span>文档</span>
          <span>状态</span>
          <span className="text-right">操作</span>
        </div>
        {docs.isPending && (
          <div role="status" aria-label="正在加载文档" className="space-y-3 p-5">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-10" />
            ))}
          </div>
        )}
        {docs.data?.length === 0 && (
          <div className="flex flex-col items-center px-6 py-14 text-center">
            <FileClock className="size-8 text-zinc-300" />
            <p className="mt-3 font-medium">还没有文档</p>
            <p className="mt-1 text-sm text-zinc-500">
              {base?.manage
                ? "上传后会自动进入异步解析和索引队列。"
                : "知识库管理者上传文档后，这里会显示可检索的资料。"}
            </p>
          </div>
        )}
        <ul className="divide-y divide-zinc-100">
          {docs.data?.map((d) => (
            <li
              key={d.id}
              className="grid gap-3 px-5 py-4 transition hover:bg-zinc-50/60 md:grid-cols-[minmax(0,1fr)_10rem_11rem] md:items-center md:gap-4"
            >
              <div className="flex min-w-0 items-start gap-3">
                <FileIcon title={d.title} />
                <div className="min-w-0">
                  {d.active_version_id ? (
                    <button
                      type="button"
                      className="max-w-full truncate text-left font-medium text-zinc-900 hover:text-primary hover:underline"
                      onClick={() => setPreview(d.active_version_id)}
                    >
                      {d.title}
                    </button>
                  ) : (
                    <p className="truncate font-medium text-zinc-900">
                      {d.title}
                    </p>
                  )}
                  {d.error && (
                    <p role="alert" className="mt-1 text-xs text-red-700">
                      {d.error}
                      {d.active_version_id
                        ? "；更新失败，旧版仍可用"
                        : "；暂无可检索版本"}
                    </p>
                  )}
                </div>
              </div>
              <div>
                <StatusBadge status={d.status} />
              </div>
              <div className="flex items-center gap-1.5 md:justify-end">
                {d.status === "failed" && base?.manage && (
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={action.isPending}
                    onClick={() =>
                      action.mutate(() => knowledge.retry(d.id), {
                        onSuccess: () => toast.success("已重新加入索引队列"),
                      })
                    }
                  >
                    <RotateCw />
                    重试索引
                  </Button>
                )}
                {d.active_version_id && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => setPreview(d.active_version_id)}
                  >
                    <Eye />
                    预览
                  </Button>
                )}
                {base?.manage && (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="text-zinc-500"
                          aria-label={`更多操作：${d.title}`}
                        />
                      }
                    >
                      <MoreHorizontal />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent>
                      {d.pending_version_id && (
                        <DropdownMenuItem
                          onClick={() => setPreview(d.pending_version_id)}
                        >
                          <FileClock />
                          查看待处理版本 / 诊断
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem
                        disabled={!!uploading}
                        onClick={() => choose(d)}
                      >
                        <FileUp />
                        上传新版本
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => void removeDocument(d)}
                      >
                        <Trash2 />
                        删除文档
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </div>
            </li>
          ))}
        </ul>
      </section>

      <VersionPreviewSheet id={preview} onClose={() => setPreview("")} />
    </div>
  );
}
