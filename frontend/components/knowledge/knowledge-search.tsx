"use client";

import { useEffect, useRef, useState } from "react";
import {
  knowledge,
  locationLabel,
  type Match,
  type RetrievalInfo,
} from "@/lib/api/knowledge";
import {
  fieldClass,
  primaryClass,
  secondaryClass,
} from "@/components/hub/registry-shared";
import { ErrorMessage, panel, useKnowledgeBases } from "./shared";
import { VersionPreview } from "./version-preview";

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
  useEffect(() => () => controller.current?.abort(), []);
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
    <div className="mx-auto max-w-5xl space-y-5 p-4 md:p-8">
      <h1 className="text-2xl font-semibold">人员检索测试</h1>
      <p className="text-sm text-zinc-500">
        使用当前登录账号权限，返回资料片段与引用；分数是排序信号，不是回答正确率。
      </p>
      <form
        className={`${panel} space-y-4`}
        onSubmit={(e) => {
          e.preventDefault();
          void search();
        }}
      >
        <fieldset>
          <legend className="mb-3 font-medium">选择知识库（最多 10 个）</legend>
          <div className="flex flex-wrap gap-4">
            {bases.data?.map((b) => (
              <label key={b.id} className="flex gap-2">
                <input
                  type="checkbox"
                  checked={ids.includes(b.id)}
                  disabled={!ids.includes(b.id) && ids.length >= 10}
                  onChange={(e) =>
                    setIDs(
                      e.target.checked
                        ? [...ids, b.id]
                        : ids.filter((id) => id !== b.id),
                    )
                  }
                />
                {b.name}
              </label>
            ))}
          </div>
        </fieldset>
        <label className="block">
          业务问题
          <textarea
            required
            maxLength={4000}
            className={fieldClass}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="报销需要哪些材料？"
          />
        </label>
        <div className="flex gap-3">
          <button disabled={busy || !ids.length} className={primaryClass}>
            {busy ? "检索中…" : "检索"}
          </button>
          {busy && (
            <button
              type="button"
              className={secondaryClass}
              onClick={() => controller.current?.abort()}
            >
              取消
            </button>
          )}
        </div>
      </form>
      <ErrorMessage error={error || bases.error} />
      {retrieval && matches && (
        <p role="status" className="text-sm text-zinc-500">
          {retrieval.strategy === "hybrid_rrf" ? "已使用语义与关键词混合检索。" : "已完成检索。"}
          {retrieval.rewrite === "applied" && "已进行单次问题改写。"}
          {retrieval.rerank === "model" && "已进行模型相关性重排。"}
          {retrieval.rewrite === "not_configured" && "尚未配置改写与重排模型，当前使用基础排序。"}
          {retrieval.warnings.length > 0 && "部分检索优化暂不可用，已回退至基础检索或排序。"}
        </p>
      )}
      {matches?.length === 0 && (
        <p className={panel}>
          没有匹配资料。请检查问题与文档内容，不应据此推断业务答案。
        </p>
      )}
      {matches?.map((m) => (
        <article className={`${panel} space-y-3`} key={m.chunk_id}>
          <h2 className="font-semibold">{m.title}</h2>
          <p className="text-xs text-zinc-500">
            {locationLabel(m.locator)} · 排序分 {m.score.toFixed(3)}
          </p>
          <pre className="whitespace-pre-wrap break-words font-sans text-sm">
            {m.content}
          </pre>
          <p className="break-all text-xs text-zinc-500">
            版本 {m.document_version_id}
          </p>
          <button
            className={secondaryClass}
            onClick={() => setPreview(m.document_version_id)}
          >
            核对引用原文
          </button>
        </article>
      ))}
      {preview && <VersionPreview id={preview} close={() => setPreview("")} />}
    </div>
  );
}
