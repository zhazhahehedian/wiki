"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  knowledge,
  locationLabel,
  type KnowledgeBase,
  type Application,
  type Match,
  type RetrievalInfo,
} from "@/lib/api/knowledge";
import { AdminGate } from "@/components/hub/governance-pages";
import { Select } from "@/components/ui/select";
import {
  fieldClass,
  primaryClass,
  secondaryClass,
} from "@/components/hub/registry-shared";

const panel = "rounded-xl border border-zinc-200 bg-white p-5";
function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <p role="alert" className="my-3 text-sm text-red-700">
      {error instanceof Error ? error.message : "操作失败，请重试"}
    </p>
  ) : null;
}
function AuditLog() {
  const entries = useQuery({
    queryKey: ["knowledge", "audit"],
    queryFn: ({ signal }) => knowledge.audit(signal),
  });
  return (
    <section className={panel}>
      <h2 className="font-semibold">最近调用审计</h2>
      <p className="my-2 text-xs text-zinc-500">
        仅记录请求编号、主体、知识库标识、状态、耗时和结果数；不记录凭证、问题或正文。
      </p>
      <ErrorMessage error={entries.error} />
      <div className="mt-3 max-h-64 space-y-3 overflow-auto">
        {entries.data?.map((e, i) => (
          <div
            key={`${e.request_id}-${i}`}
            className="break-all border-t pt-2 text-xs"
          >
            <p>
              {new Date(e.created_at).toLocaleString()} · {e.status} ·{" "}
              {e.duration_ms} ms · {e.result_count} 条
            </p>
            <p className="text-zinc-500">
              {e.application_id || "人员检索"} · {e.request_id}
            </p>
            <p className="text-zinc-500">
              知识库：{e.knowledge_base_ids?.join("、") || "—"}
            </p>
          </div>
        ))}
        {entries.data?.length === 0 && (
          <p className="text-sm">尚无调用记录。</p>
        )}
      </div>
    </section>
  );
}
const emptyBase = {
  name: "",
  description: "",
  visibility: "private",
  department: "",
  members: [] as string[],
  application_shared: false,
};

export function KnowledgeBases() {
  const client = useQueryClient();
  const bases = useQuery({
    queryKey: ["knowledge", "bases"],
    queryFn: ({ signal }) => knowledge.bases(signal),
  });
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState(emptyBase);
  const [memberText, setMemberText] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  async function save() {
    setBusy(true);
    setError(null);
    try {
      await knowledge.saveBase(editing ?? "", {
        ...form,
        members: memberText
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
      });
      setEditing(null);
      await client.invalidateQueries({ queryKey: ["knowledge"] });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  function edit(b?: KnowledgeBase) {
    setMemberText(b?.members.join(",") ?? "");
    setEditing(b?.id ?? "");
    setForm(
      b
        ? {
            name: b.name,
            description: b.description,
            visibility: b.visibility,
            department: b.department,
            members: b.members,
            application_shared: b.application_shared,
          }
        : emptyBase,
    );
    setError(null);
  }
  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-8">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">知识库</h1>
          <p className="mt-2 text-sm text-zinc-500">
            管理文档、检查解析质量，向人员和应用提供授权检索。
          </p>
        </div>
        <button className={primaryClass} onClick={() => edit()}>
          创建知识库
        </button>
      </div>
      <div className="flex gap-5 text-sm text-blue-700">
        <Link href="/hub/knowledge/search">人员检索测试</Link>
        <Link href="/hub/knowledge/applications">应用接入</Link>
      </div>
      <ErrorMessage error={bases.error || error} />
      {bases.isPending && <p role="status">正在加载知识库…</p>}
      {bases.isError && (
        <button className={secondaryClass} onClick={() => void bases.refetch()}>
          重试加载
        </button>
      )}
      {editing !== null && (
        <form
          className={`${panel} space-y-4`}
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          <h2 className="font-semibold">
            {editing ? "编辑知识库" : "创建知识库"}
          </h2>
          <label className="block">
            名称
            <input
              required
              maxLength={120}
              className={fieldClass}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </label>
          <label className="block">
            说明
            <textarea
              className={fieldClass}
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
            />
          </label>
          <label className="block">
            人员可见范围
            <Select
              aria-label="人员可见范围"
              value={form.visibility}
              onValueChange={(value) =>
                setForm({
                  ...form,
                  visibility: value,
                  application_shared:
                    value === "private" ? false : form.application_shared,
                })
              }
              options={[
                { value: "private", label: "仅 Owner / Admin" },
                { value: "company", label: "全公司" },
                { value: "department", label: "指定可信部门" },
                { value: "members", label: "指定成员" },
              ]}
            />
          </label>
          {form.visibility === "department" && (
            <label className="block">
              部门名称
              <input
                required
                className={fieldClass}
                value={form.department}
                onChange={(e) =>
                  setForm({ ...form, department: e.target.value })
                }
              />
            </label>
          )}
          {form.visibility === "members" && (
            <label className="block">
              成员飞书 Open ID（逗号分隔）
              <input
                className={fieldClass}
                value={memberText}
                onChange={(e) => setMemberText(e.target.value)}
              />
            </label>
          )}
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              disabled={form.visibility === "private"}
              checked={form.application_shared}
              onChange={(e) =>
                setForm({ ...form, application_shared: e.target.checked })
              }
            />
            <span className="text-sm">
              允许管理员显式授权给应用。确认整库文档适合共享；全公司可见不会自动授权应用。
            </span>
          </label>
          <div className="flex gap-3">
            <button disabled={busy} className={primaryClass}>
              {busy ? "保存中…" : "保存"}
            </button>
            <button
              type="button"
              className={secondaryClass}
              onClick={() => setEditing(null)}
            >
              取消
            </button>
          </div>
        </form>
      )}
      {!bases.isPending && bases.data?.length === 0 && (
        <div className={panel}>
          还没有可见知识库。创建后可上传文档并检查解析结果。
        </div>
      )}
      <div className="grid gap-4 md:grid-cols-2">
        {bases.data?.map((b) => (
          <article key={b.id} className={panel}>
            <Link
              className="text-lg font-semibold text-blue-700"
              href={`/hub/knowledge/${b.id}`}
            >
              {b.name}
            </Link>
            <p className="my-3 text-sm text-zinc-600">
              {b.description || "暂无说明"}
            </p>
            <p className="text-xs text-zinc-500">
              {b.visibility} ·{" "}
              {b.application_shared ? "可授权应用" : "未开放应用共享"}
            </p>
            {b.manage && (
              <div className="mt-4 flex gap-3">
                <button className={secondaryClass} onClick={() => edit(b)}>
                  编辑权限
                </button>
                <button
                  className={secondaryClass}
                  onClick={async () => {
                    if (
                      !window.confirm(
                        `删除知识库“${b.name}”？所有文档会立即不可检索。`,
                      )
                    )
                      return;
                    try {
                      await knowledge.deleteBase(b.id);
                      await bases.refetch();
                    } catch (e) {
                      setError(e);
                    }
                  }}
                >
                  删除
                </button>
              </div>
            )}
          </article>
        ))}
      </div>
    </div>
  );
}

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
          {version.data.preview.diagnostics?.map((d, i) => (
            <p
              key={i}
              className={d.blocking ? "text-red-700" : "text-amber-700"}
            >
              {d.blocking ? "阻止索引" : "警告"}：
              {d.page ? `第 ${d.page} 页：` : ""}
              {d.message}
            </p>
          ))}
          {!version.data.preview.blocks?.length && (
            <p>尚无可预览正文。请等待解析完成，或查看失败诊断。</p>
          )}
          <div className="max-h-[36rem] space-y-3 overflow-auto">
            {version.data.preview.blocks?.map((b, i) => (
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

export function KnowledgeDocuments({ id }: { id: string }) {
  const bases = useQuery({
    queryKey: ["knowledge", "bases"],
    queryFn: ({ signal }) => knowledge.bases(signal),
  });
  const base = bases.data?.find((b) => b.id === id);
  const docs = useQuery({
    queryKey: ["knowledge", "documents", id],
    queryFn: ({ signal }) => knowledge.documents(id, signal),
    refetchInterval: 3000,
  });
  const [preview, setPreview] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  const [update, setUpdate] = useState("");
  const file = useRef<HTMLInputElement>(null);
  async function upload(f?: File) {
    if (!f) return;
    setBusy(true);
    setError(null);
    try {
      await knowledge.upload(id, f, update);
      setUpdate("");
      await docs.refetch();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
      if (file.current) file.current.value = "";
    }
  }
  async function action(fn: () => Promise<unknown>) {
    setError(null);
    try {
      await fn();
      await docs.refetch();
    } catch (e) {
      setError(e);
    }
  }
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
      <ErrorMessage error={error || docs.error || bases.error} />
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
              disabled={busy}
              onChange={(e) => void upload(e.target.files?.[0])}
              className="block w-full text-sm"
            />
          </label>
          {busy && <p role="status">正在上传…</p>}
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
              {(
                {
                  pending: "等待索引",
                  processing: "正在索引",
                  ready: "可检索",
                  failed: "处理失败",
                  legacy_requires_reindex: "旧资料：请上传新版本完成质量核验",
                } as Record<string, string>
              )[d.status] ?? d.status}
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
                    onClick={() => void action(() => knowledge.retry(d.id))}
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
                      void action(() => knowledge.deleteDocument(d.id));
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

export function KnowledgeSearch() {
  const bases = useQuery({
    queryKey: ["knowledge", "bases"],
    queryFn: ({ signal }) => knowledge.bases(signal),
  });
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

export function KnowledgeApplications() {
  return (
    <AdminGate>
      <ApplicationsContent />
    </AdminGate>
  );
}
function ApplicationsContent() {
  const apps = useQuery({
    queryKey: ["knowledge", "applications"],
    queryFn: ({ signal }) => knowledge.applications(signal),
  });
  const bases = useQuery({
    queryKey: ["knowledge", "bases"],
    queryFn: ({ signal }) => knowledge.bases(signal),
  });
  const [editing, setEditing] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [ids, setIDs] = useState<string[]>([]);
  const [token, setToken] = useState("");
  const [error, setError] = useState<unknown>();
  const [busy, setBusy] = useState(false);
  function edit(a?: Application) {
    setEditing(a?.id ?? "");
    setName(a?.name ?? "");
    setEnabled(a?.enabled ?? true);
    setIDs(a?.knowledge_base_ids ?? []);
    setToken("");
  }
  async function action(fn: () => Promise<unknown>) {
    setBusy(true);
    setError(null);
    try {
      await fn();
      await apps.refetch();
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="mx-auto max-w-5xl space-y-5 p-4 md:p-8">
      <div className="flex justify-between gap-4">
        <h1 className="text-2xl font-semibold">应用接入</h1>
        <button className={primaryClass} onClick={() => edit()}>
          创建应用
        </button>
      </div>
      <p className="text-sm text-zinc-500">
        凭证仅在签发时显示一次，有效期 90 天。轮换立即撤销旧凭证。每应用每分钟
        60 次、最多 4 个并发请求。
      </p>
      <ErrorMessage error={error || apps.error || bases.error} />
      {token && (
        <section className={`${panel} space-y-3`}>
          <h2 className="font-semibold">请立即保存凭证</h2>
          <code className="block break-all rounded bg-zinc-100 p-3">
            {token}
          </code>
          <button className={secondaryClass} onClick={() => setToken("")}>
            已保存，隐藏凭证
          </button>
        </section>
      )}
      {editing !== null && (
        <form
          className={`${panel} space-y-4`}
          onSubmit={(e) => {
            e.preventDefault();
            void action(async () => {
              await knowledge.saveApplication(editing, {
                name,
                enabled,
                knowledge_base_ids: ids,
              });
              setEditing(null);
            });
          }}
        >
          <label className="block">
            应用名称
            <input
              className={fieldClass}
              required
              maxLength={120}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </label>
          <label className="flex gap-2">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            启用应用
          </label>
          <fieldset>
            <legend className="mb-3">显式授权知识库</legend>
            {bases.data
              ?.filter((b) => b.application_shared)
              .map((b) => (
                <label key={b.id} className="my-2 flex gap-2">
                  <input
                    type="checkbox"
                    checked={ids.includes(b.id)}
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
            <p className="text-xs text-zinc-500">
              只能选择 Owner 已确认整库可共享的知识库。
            </p>
          </fieldset>
          <div className="flex gap-3">
            <button disabled={busy} className={primaryClass}>
              保存授权
            </button>
            <button
              type="button"
              className={secondaryClass}
              onClick={() => setEditing(null)}
            >
              取消
            </button>
          </div>
        </form>
      )}
      {apps.isPending && <p role="status">正在加载应用…</p>}
      {apps.data?.length === 0 && (
        <p className={panel}>尚无应用。创建应用、授权知识库，再签发凭证。</p>
      )}
      {apps.data?.map((a) => (
        <article key={a.id} className={`${panel} space-y-3`}>
          <h2 className="font-semibold">
            {a.name} · {a.enabled ? "已启用" : "已禁用"}
          </h2>
          <p className="text-sm">
            已授权 {a.knowledge_base_ids.length} 个知识库
          </p>
          <p className="text-xs text-zinc-500">
            凭证：{a.credential_prefix || "未签发 / 已撤销"} · 到期：
            {a.expires_at ? new Date(a.expires_at).toLocaleString() : "—"} ·
            最后使用：
            {a.last_used_at ? new Date(a.last_used_at).toLocaleString() : "—"}
          </p>
          <div className="flex flex-wrap gap-3">
            <button className={secondaryClass} onClick={() => edit(a)}>
              编辑授权 / 禁用
            </button>
            <button
              disabled={busy}
              className={secondaryClass}
              onClick={() =>
                void action(async () => {
                  setToken("");
                  const c = await knowledge.rotate(a.id);
                  setToken(c.token);
                })
              }
            >
              签发 / 轮换凭证
            </button>
            <button
              disabled={busy}
              className={secondaryClass}
              onClick={() =>
                void action(async () => {
                  setToken("");
                  await knowledge.revoke(a.id);
                })
              }
            >
              撤销凭证
            </button>
          </div>
        </article>
      ))}
      <AuditLog />
      <section className={panel}>
        <h2 className="mb-3 font-semibold">外部系统调用</h2>
        <pre className="overflow-auto whitespace-pre-wrap break-all text-xs">{`POST /api/v1/knowledge/search\nAuthorization: Bearer <application_token>\nContent-Type: application/json\n\n{"knowledge_base_ids":["<kb-id>"],"query":"报销需要哪些材料？","top_k":5}`}</pre>
        <p className="mt-3 text-sm text-zinc-500">
          应用后端持有凭证。客户端不能通过 user_id 或 department
          获取人员权限；对话与回答生成由业务系统负责。
        </p>
      </section>
    </div>
  );
}
