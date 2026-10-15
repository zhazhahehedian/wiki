"use client";

import Link from "next/link";
import { useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { knowledge } from "@/lib/api/knowledge";
import { secondaryClass } from "@/components/hub/registry-shared";
import {
  documentStatusLabels,
  ErrorMessage,
  panel,
  useInvalidateKnowledge,
  useKnowledgeBases,
} from "./shared";
import { VersionPreview } from "./version-preview";

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
  const [update, setUpdate] = useState("");
  const file = useRef<HTMLInputElement>(null);
  const upload = useMutation({
    mutationFn: (f: File) => knowledge.upload(id, f, update),
    onSuccess: () => {
      setUpdate("");
      return invalidate();
    },
    onSettled: () => {
      if (file.current) file.current.value = "";
    },
  });
  const action = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: invalidate,
  });
  return (
    <div className="mx-auto max-w-6xl space-y-5 p-4 md:p-8">
      <Link className="text-sm text-blue-700" href="/hub/knowledge">
        返回知识库
      </Link>
      <h1 className="text-2xl font-semibold">{base?.name ?? "知识库文档"}</h1>
      <p className="text-sm text-zinc-500">
        支持 TXT、Markdown、DOCX、文字型 PDF，单文件最大 20
        MiB。扫描件和正文缺失会阻止发布；更新失败时旧版仍可检索。
      </p>
      <ErrorMessage
        error={upload.error || action.error || docs.error || bases.error}
      />
      {base?.manage && (
        <div className={panel}>
          <label className="block space-y-3">
            <span>
              {update ? "选择替换文档（成功后切换版本）" : "上传文档"}
            </span>
            <input
              ref={file}
              aria-label="上传文档文件"
              type="file"
              accept=".txt,.md,.markdown,.docx,.pdf"
              disabled={upload.isPending}
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) upload.mutate(f);
              }}
              className="block w-full text-sm"
            />
          </label>
          {upload.isPending && <p role="status">正在上传…</p>}
          {update && (
            <button
              className={`${secondaryClass} mt-3`}
              onClick={() => setUpdate("")}
            >
              取消更新
            </button>
          )}
        </div>
      )}
      {docs.isPending && <p role="status">正在加载文档…</p>}
      {docs.data?.length === 0 && (
        <p className={panel}>
          还没有文档。上传后会自动进入异步解析和索引队列。
        </p>
      )}
      {docs.data?.map((d) => (
        <article key={d.id} className={`${panel} space-y-3`}>
          <div className="flex flex-wrap justify-between gap-3">
            <h2 className="break-all font-medium">{d.title}</h2>
            <span className="text-sm">
              {documentStatusLabels[d.status] ?? d.status}
            </span>
          </div>
          {d.error && (
            <p role="alert" className="text-sm text-red-700">
              {d.error}
              {d.active_version_id
                ? "；更新失败，旧版仍可用"
                : "；暂无可检索版本"}
            </p>
          )}
          <p className="break-all text-xs text-zinc-500">
            可检索版本：{d.active_version_id || "尚未就绪"}
          </p>
          <div className="flex flex-wrap gap-2">
            {d.active_version_id && (
              <button
                className={secondaryClass}
                onClick={() => setPreview(d.active_version_id)}
              >
                查看可检索版本
              </button>
            )}
            {base?.manage && (
              <>
                {d.pending_version_id && (
                  <button
                    className={secondaryClass}
                    onClick={() => setPreview(d.pending_version_id)}
                  >
                    查看待处理版本 / 诊断
                  </button>
                )}
                <button
                  className={secondaryClass}
                  onClick={() => {
                    setUpdate(d.id);
                    file.current?.focus();
                  }}
                >
                  上传新版本
                </button>
                {d.status === "failed" && (
                  <button
                    className={secondaryClass}
                    onClick={() => action.mutate(() => knowledge.retry(d.id))}
                  >
                    重试索引
                  </button>
                )}
                <button
                  className={secondaryClass}
                  onClick={() => {
                    if (
                      window.confirm(`删除“${d.title}”？新请求将立即不可见。`)
                    )
                      action.mutate(() => knowledge.deleteDocument(d.id));
                  }}
                >
                  删除文档
                </button>
              </>
            )}
          </div>
        </article>
      ))}
      {preview && <VersionPreview id={preview} close={() => setPreview("")} />}
    </div>
  );
}
