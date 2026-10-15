"use client";

import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { knowledge, type Application } from "@/lib/api/knowledge";
import { AdminGate } from "@/components/hub/governance-pages";
import {
  fieldClass,
  primaryClass,
  secondaryClass,
} from "@/components/hub/registry-shared";
import {
  ErrorMessage,
  panel,
  useInvalidateKnowledge,
  useKnowledgeBases,
} from "./shared";

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
              知识库：{e.knowledge_base_ids.join("、") || "—"}
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
  const bases = useKnowledgeBases();
  const invalidate = useInvalidateKnowledge();
  const [editing, setEditing] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [ids, setIDs] = useState<string[]>([]);
  const [token, setToken] = useState("");
  const action = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: invalidate,
  });
  const shared = bases.data?.filter((b) => b.application_shared) ?? [];
  // Grants to bases that were later unshared or deleted can only be removed.
  const revoked = bases.data
    ? ids.filter((id) => !shared.some((b) => b.id === id))
    : [];
  function edit(a?: Application) {
    setEditing(a?.id ?? "");
    setName(a?.name ?? "");
    setEnabled(a?.enabled ?? true);
    setIDs(a?.knowledge_base_ids ?? []);
    setToken("");
  }
  function toggle(id: string, checked: boolean) {
    setIDs(checked ? [...ids, id] : ids.filter((x) => x !== id));
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
      <ErrorMessage error={action.error || apps.error || bases.error} />
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
            action.mutate(async () => {
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
            {shared.map((b) => (
              <label key={b.id} className="my-2 flex gap-2">
                <input
                  type="checkbox"
                  checked={ids.includes(b.id)}
                  onChange={(e) => toggle(b.id, e.target.checked)}
                />
                {b.name}
              </label>
            ))}
            {revoked.map((id) => (
              <label key={id} className="my-2 flex gap-2 text-zinc-500">
                <input
                  type="checkbox"
                  checked
                  onChange={(e) => toggle(id, e.target.checked)}
                />
                {bases.data?.find((b) => b.id === id)?.name ?? id}
                （已取消共享或已删除）
              </label>
            ))}
            <p className="text-xs text-zinc-500">
              只能选择 Owner 已确认整库可共享的知识库。
            </p>
          </fieldset>
          <div className="flex gap-3">
            <button disabled={action.isPending} className={primaryClass}>
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
              disabled={action.isPending}
              className={secondaryClass}
              onClick={() =>
                action.mutate(async () => {
                  setToken("");
                  const c = await knowledge.rotate(a.id);
                  setToken(c.token);
                })
              }
            >
              签发 / 轮换凭证
            </button>
            <button
              disabled={action.isPending}
              className={secondaryClass}
              onClick={() =>
                action.mutate(async () => {
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
