"use client";

import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { knowledge, locationLabel } from "@/lib/api/knowledge";
import { secondaryClass } from "@/components/hub/registry-shared";
import { ErrorMessage, panel } from "./shared";

export function VersionPreview({
  id,
  close,
}: {
  id: string;
  close: () => void;
}) {
  const previewRef = useRef<HTMLElement>(null);
  useEffect(() => {
    previewRef.current?.scrollIntoView?.({
      behavior: "smooth",
      block: "start",
    });
  }, [id]);
  const version = useQuery({
    queryKey: ["knowledge", "version", id],
    queryFn: ({ signal }) => knowledge.preview(id, signal),
    refetchInterval: (q) =>
      q.state.data?.status === "processing" ||
      q.state.data?.status === "pending"
        ? 2000
        : false,
  });
  return (
    <section
      ref={previewRef}
      className={`${panel} space-y-4`}
      aria-label="解析预览"
    >
      <div className="flex justify-between gap-3">
        <h2 className="font-semibold">解析预览与质量诊断</h2>
        <button className={secondaryClass} onClick={close}>
          关闭
        </button>
      </div>
      <ErrorMessage error={version.error} />
      {version.isPending && <p role="status">正在加载预览…</p>}
      {version.data && (
        <>
          <p className="break-all text-xs text-zinc-500">
            版本 {id} · {version.data.status} · {version.data.model} /{" "}
            {version.data.dim} 维
          </p>
          {version.data.error && (
            <p role="alert" className="text-red-700">
              {version.data.error}
            </p>
          )}
          {version.data.preview.diagnostics.map((d, i) => (
            <p
              key={i}
              className={d.blocking ? "text-red-700" : "text-amber-700"}
            >
              {d.blocking ? "阻止索引" : "警告"}：
              {d.page ? `第 ${d.page} 页：` : ""}
              {d.message}
            </p>
          ))}
          {!version.data.preview.blocks.length && (
            <p>尚无可预览正文。请等待解析完成，或查看失败诊断。</p>
          )}
          <div className="max-h-[36rem] space-y-3 overflow-auto">
            {version.data.preview.blocks.map((b, i) => (
              <article key={i} className="rounded-lg bg-zinc-50 p-4">
                <p className="mb-2 text-xs text-blue-700">
                  {locationLabel(b.locator)}
                </p>
                <pre className="whitespace-pre-wrap break-words font-sans text-sm">
                  {b.content}
                </pre>
              </article>
            ))}
          </div>
        </>
      )}
    </section>
  );
}
