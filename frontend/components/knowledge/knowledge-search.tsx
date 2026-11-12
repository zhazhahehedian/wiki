"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import {
  Check,
  ChevronRight,
  Eye,
  Loader2,
  Search,
  SearchX,
  Sparkles,
  X,
} from "lucide-react";
import {
  knowledge,
  locationLabel,
  type Match,
  type RetrievalInfo,
} from "@/lib/api/knowledge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ErrorMessage, FileIcon, useKnowledgeBases } from "./shared";
import { VersionPreviewSheet } from "./version-preview";

const maxBases = 10;

export function KnowledgeSearch() {
  const bases = useKnowledgeBases();
  const [ids, setIDs] = useState<string[]>([]);
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<Match[] | null>(null);
  const [retrieval, setRetrieval] = useState<RetrievalInfo>();
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState("");
  const controller = useRef<AbortController | null>(null);
  const form = useRef<HTMLFormElement>(null);
  useEffect(() => () => controller.current?.abort(), []);
  const baseName = new Map(bases.data?.map((b) => [b.id, b.name] as const));
  async function search() {
    controller.current?.abort();
    const c = new AbortController();
    controller.current = c;
    setBusy(true);
    setError(null);
    setMatches(null);
    setRetrieval(undefined);
    try {
      const r = await knowledge.search(ids, query, c.signal);
      setMatches(r.matches);
      setRetrieval(r.retrieval);
    } catch (e) {
      if (!c.signal.aborted) setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="mx-auto max-w-5xl space-y-6 p-4 md:p-8">
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
        <span className="px-1 text-zinc-900">检索测试</span>
      </nav>
      <header>
        <h1 className="text-2xl font-semibold">人员检索测试</h1>
        <p className="mt-1.5 text-sm text-zinc-500">
          使用当前登录账号的权限检索，返回资料片段与引用；结果与业务系统通过接口调用得到的一致。分数是排序信号，不是回答正确率。
        </p>
      </header>

      <form
        ref={form}
        className="space-y-5 rounded-xl border border-zinc-200 bg-white p-5"
        onSubmit={(e) => {
          e.preventDefault();
          void search();
        }}
      >
        <fieldset>
          <legend className="mb-3 flex w-full items-center justify-between text-sm font-medium">
            <span>选择知识库</span>
            <span className="text-xs font-normal text-zinc-500">
              已选 {ids.length} / {maxBases}
            </span>
          </legend>
          {bases.data?.length === 0 && (
            <p className="text-sm text-zinc-500">
              暂无可检索的知识库。
              <Link href="/hub/knowledge" className="text-primary hover:underline">
                去创建
              </Link>
            </p>
          )}
          <div className="flex flex-wrap gap-2">
            {bases.data?.map((b) => (
              <label key={b.id} className="group cursor-pointer">
                <input
                  type="checkbox"
                  className="peer sr-only"
                  checked={ids.includes(b.id)}
                  disabled={!ids.includes(b.id) && ids.length >= maxBases}
                  onChange={(e) =>
                    setIDs(
                      e.target.checked
                        ? [...ids, b.id]
                        : ids.filter((id) => id !== b.id),
                    )
                  }
                />
                <span className="inline-flex items-center gap-1.5 rounded-full border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-700 transition select-none group-hover:border-zinc-300 group-hover:bg-zinc-50 peer-checked:border-primary peer-checked:bg-primary/10 peer-checked:text-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary/30 peer-disabled:cursor-not-allowed peer-disabled:opacity-40">
                  <Check className="hidden size-3.5 group-has-checked:block" />
                  {b.name}
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        <div>
          <label htmlFor="knowledge-query" className="text-sm font-medium">
            业务问题
          </label>
          <div className="mt-2 rounded-xl border border-zinc-200 bg-white transition focus-within:border-zinc-400 focus-within:ring-2 focus-within:ring-blue-100">
            <textarea
              id="knowledge-query"
              required
              maxLength={4000}
              rows={3}
              className="block w-full resize-none rounded-t-xl bg-transparent px-3.5 pt-3 text-sm outline-none"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (
                  e.key === "Enter" &&
                  !e.shiftKey &&
                  !e.nativeEvent.isComposing
                ) {
                  e.preventDefault();
                  form.current?.requestSubmit();
                }
              }}
              placeholder="例如：报销需要哪些材料？"
            />
            <div className="flex items-center justify-between gap-3 px-3 pb-3">
              <span className="text-xs text-zinc-400">
                Enter 检索，Shift + Enter 换行
              </span>
              <div className="flex gap-2">
                {busy && (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => controller.current?.abort()}
                  >
                    <X />
                    取消
                  </Button>
                )}
                <Button
                  type="submit"
                  className="px-3.5"
                  disabled={busy || !ids.length || !query.trim()}
                >
                  {busy ? <Loader2 className="animate-spin" /> : <Search />}
                  {busy ? "检索中…" : "检索"}
                </Button>
              </div>
            </div>
          </div>
          {!ids.length && (
            <p className="mt-2 text-xs text-zinc-500">请先选择至少一个知识库。</p>
          )}
        </div>
      </form>

      <ErrorMessage error={error || bases.error} />

      {retrieval && matches && (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="font-semibold">
            {matches.length ? `找到 ${matches.length} 个相关片段` : "检索结果"}
          </h2>
          <p
            role="status"
            className="flex items-center gap-1.5 text-xs text-zinc-500"
          >
            <Sparkles className="size-3.5" />
            {retrieval.strategy === "hybrid_rrf"
              ? "已使用语义与关键词混合检索。"
              : "已完成检索。"}
            {retrieval.rewrite === "applied" && "已进行单次问题改写。"}
            {retrieval.rerank === "model" && "已进行模型相关性重排。"}
            {retrieval.rewrite === "not_configured" &&
              "尚未配置改写与重排模型，当前使用基础排序。"}
            {retrieval.warnings.length > 0 &&
              "部分检索优化暂不可用，已回退至基础检索或排序。"}
          </p>
        </div>
      )}

      {matches?.length === 0 && (
        <div className="flex flex-col items-center rounded-xl border border-dashed border-zinc-300 bg-white px-6 py-12 text-center">
          <SearchX className="size-8 text-zinc-300" />
          <p className="mt-3 font-medium">没有匹配资料</p>
          <p className="mt-1 max-w-md text-sm text-zinc-500">
            请检查问题与文档内容，不应据此推断业务答案。
          </p>
        </div>
      )}

      <ol className="space-y-4">
        {matches?.map((m, i) => (
          <li
            key={m.chunk_id}
            className="rounded-xl border border-zinc-200 bg-white p-5 transition hover:border-zinc-300 hover:shadow-sm"
          >
            <div className="flex items-start gap-3">
              <FileIcon title={m.title} />
              <div className="min-w-0 flex-1">
                <h3 className="truncate font-semibold">{m.title}</h3>
                <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-zinc-500">
                  <Badge variant="muted">#{i + 1}</Badge>
                  {baseName.get(m.knowledge_base_id) && (
                    <Badge variant="outline">
                      {baseName.get(m.knowledge_base_id)}
                    </Badge>
                  )}
                  <span>{locationLabel(m.locator)}</span>
                </div>
              </div>
              <div className="w-24 shrink-0 text-right">
                <p className="text-xs text-zinc-500">
                  排序分{" "}
                  <span className="font-mono font-medium text-zinc-800">
                    {m.score.toFixed(3)}
                  </span>
                </p>
                <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-zinc-100">
                  <div
                    className="h-full rounded-full bg-primary/70"
                    style={{
                      width: `${Math.max(0, Math.min(1, m.score)) * 100}%`,
                    }}
                  />
                </div>
              </div>
            </div>
            <pre className="mt-4 whitespace-pre-wrap break-words rounded-lg border-l-2 border-primary/40 bg-zinc-50 px-4 py-3 font-sans text-sm leading-relaxed text-zinc-800">
              {m.content}
            </pre>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
              <span className="break-all font-mono text-[11px] text-zinc-400">
                版本 {m.document_version_id}
              </span>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPreview(m.document_version_id)}
              >
                <Eye />
                核对引用原文
              </Button>
            </div>
          </li>
        ))}
      </ol>

      <VersionPreviewSheet id={preview} onClose={() => setPreview("")} />
    </div>
  );
}
