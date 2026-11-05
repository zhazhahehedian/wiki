"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Select } from "@/components/ui/select";
import {
  getGovernanceIdentity,
  listReviews,
  listAudit,
  listProfiles,
  setTrustedDepartment,
  setReviewer,
  type GovernanceIdentity,
} from "@/lib/api/registry";
import {
  fieldClass,
  primaryClass,
  secondaryClass,
  RegistryError,
  StatusBadge,
  Field,
} from "./registry-shared";
import { actionLabels } from "./governance-controls";

export function AdminGate({ children }: { children: ReactNode }) {
  return <GovernanceGate>{children}</GovernanceGate>;
}
function GovernanceGate({ children, review = false }: { children: ReactNode; review?: boolean }) {
  const query = useQuery({
    queryKey: ["registry", "identity"],
    queryFn: ({ signal }) => getGovernanceIdentity(signal),
    refetchInterval: 30_000,
  });
  if (query.isPending) return <p role="status">正在检查治理权限…</p>;
  if (query.isError)
    return (
      <RegistryError error={query.error} retry={() => void query.refetch()} />
    );
  if (!query.data.is_admin && !(review && query.data.is_reviewer))
    return (
      <div className="rounded-xl border border-zinc-200 bg-white p-8">
        <h1 className="font-semibold">{review ? "需要审核员或管理员权限" : "需要管理员权限"}</h1>
        <p className="mt-3 text-sm text-zinc-500">
          当前账号可浏览能力市场并管理自己的发布。审核资格由平台管理员配置。
        </p>
        <Link
          href="/hub/registry"
          className="mt-4 inline-block text-sm text-zinc-600"
        >
          返回能力市场
        </Link>
      </div>
    );
  return <>{children}</>;
}
function Pagination({
  offset,
  more,
  change,
}: {
  offset: number;
  more?: boolean;
  change: (n: number) => void;
}) {
  return (
    <div className="mt-5 flex justify-end gap-2">
      <button
        className={`${secondaryClass} shrink-0 whitespace-nowrap`}
        disabled={!offset}
        onClick={() => change(Math.max(0, offset - 24))}
      >
        上一页
      </button>
      <button
        className={`${secondaryClass} shrink-0 whitespace-nowrap`}
        disabled={!more}
        onClick={() => change(offset + 24)}
      >
        下一页
      </button>
    </div>
  );
}
export function ReviewsPage() {
  return (
    <GovernanceGate review>
      <ReviewQueue />
    </GovernanceGate>
  );
}
function ReviewQueue() {
  const [offset, setOffset] = useState(0);
  const [search, setSearch] = useState("");
  const [q, setQ] = useState("");
  const query = useQuery({
    queryKey: ["registry", "reviews", { q, offset }],
    queryFn: ({ signal }) => listReviews({ q, offset }, signal),
    refetchInterval: 30_000,
  });
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">审核队列</h1>
        <p className="mt-2 text-sm text-zinc-500">
          全平台 MCP / Skill · 允许自审。按提交时间处理，打开详情查看待审版本、Skill 文件和上线版本对照。
        </p>
      </header>
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setQ(search.trim());
          setOffset(0);
        }}
      >
        <input
          aria-label="搜索待审能力"
          className={fieldClass}
          maxLength={200}
          placeholder="名称、标识或描述"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <button className={`${secondaryClass} shrink-0 whitespace-nowrap`}>
          搜索
        </button>
      </form>
      {query.isPending ? (
        <p role="status">正在加载待审能力…</p>
      ) : query.isError ? (
        <RegistryError error={query.error} retry={() => void query.refetch()} />
      ) : (
        <>
          <div className="divide-y divide-zinc-100 rounded-xl border border-zinc-200 bg-white">
            {query.data.items.length ? (
              query.data.items.map((item) => (
                <Link
                  key={item.id}
                  href={`/hub/registry/${item.slug}`}
                  className="flex flex-wrap items-center justify-between gap-4 p-5 hover:bg-zinc-50/50"
                >
                  <div className="min-w-0">
                    <h2 className="break-words font-medium text-zinc-700">
                      {item.name}
                    </h2>
                    <p className="mt-2 break-all text-xs text-zinc-400">
                      {item.type.toUpperCase()} · {item.slug} ·{" "}
                      {item.department || "未设置部门"}
                    </p>
                    <p className="mt-2 line-clamp-2 text-sm text-zinc-500">
                      {item.description}
                    </p>
                  </div>
                  <div className="space-y-2 text-right">
                    <StatusBadge status={item.status} />
                    <p className="text-xs text-zinc-400">
                      {new Date(item.updated_at).toLocaleString("zh-CN")}
                    </p>
                    {item.is_live && (
                      <p className="text-xs text-zinc-600">旧版本仍上线</p>
                    )}
                  </div>
                </Link>
              ))
            ) : (
              <p className="p-12 text-center text-sm text-zinc-400">
                暂无待审能力
              </p>
            )}
          </div>
          <Pagination
            offset={offset}
            more={query.data.has_more}
            change={setOffset}
          />
        </>
      )}
    </div>
  );
}
export function AuditPage() {
  return (
    <AdminGate>
      <AuditLog />
    </AdminGate>
  );
}
function AuditLog() {
  const [form, setForm] = useState({ action: "", actor: "", target: "" });
  const [filter, setFilter] = useState({ ...form, offset: 0 });
  const query = useQuery({
    queryKey: ["registry", "audit", filter],
    queryFn: ({ signal }) => listAudit(filter, signal),
  });
  const labels: Record<string, string> = {
    ...actionLabels,
    create: "创建草稿",
    edit: "编辑草稿",
    set_department: "维护可信部门",
    set_reviewer: "变更审核资格",
  };
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">审计日志</h1>
        <p className="mt-2 text-sm text-zinc-500">
          查看能力治理、审核资格与部门资料变更，按操作者、动作或目标筛选。
        </p>
      </header>
      <form
        className="grid gap-3 rounded-xl border border-zinc-200 bg-white p-4 sm:grid-cols-2 lg:grid-cols-4"
        onSubmit={(e) => {
          e.preventDefault();
          setFilter({ ...form, offset: 0 });
        }}
      >
        <Select
          aria-label="审计动作"
          className={fieldClass}
          value={form.action}
          onValueChange={(value) => setForm({ ...form, action: value })}
          options={[
            { value: "", label: "全部动作" },
            ...Object.entries(labels).map(([value, label]) => ({
              value,
              label,
            })),
          ]}
        />
        <input
          aria-label="操作者 Open ID"
          className={fieldClass}
          maxLength={128}
          placeholder="操作者 Open ID"
          value={form.actor}
          onChange={(e) => setForm({ ...form, actor: e.target.value })}
        />
        <input
          aria-label="目标 ID"
          className={fieldClass}
          maxLength={128}
          placeholder="能力 ID / 用户 Open ID"
          value={form.target}
          onChange={(e) => setForm({ ...form, target: e.target.value })}
        />
        <button className={primaryClass}>筛选日志</button>
      </form>
      {query.isPending ? (
        <p role="status">正在加载审计…</p>
      ) : query.isError ? (
        <RegistryError error={query.error} retry={() => void query.refetch()} />
      ) : (
        <>
          <div className="space-y-3">
            {query.data.items.map((entry) => (
              <details
                key={entry.id}
                className="rounded-xl border border-zinc-200 bg-white p-5"
              >
                <summary className="cursor-pointer">
                  <span className="text-sm font-medium">
                    {labels[entry.action] ?? entry.action}
                  </span>
                  <span className="ml-3 text-xs text-zinc-400">
                    {new Date(entry.created_at).toLocaleString("zh-CN")}
                  </span>
                  <p className="mt-2 break-all text-xs text-zinc-500">
                    {entry.actor_open_id} → {entry.target_id}
                  </p>
                </summary>
                <pre className="mt-4 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-zinc-50 p-4 text-xs leading-6">
                  {JSON.stringify(entry.detail, null, 2)}
                </pre>
                <p className="mt-3 break-all text-xs text-zinc-400">
                  请求 ID：{entry.request_id || "无（非 HTTP 操作）"}
                </p>
              </details>
            ))}
            {query.data.items.length === 0 && (
              <p className="p-12 text-center text-sm text-zinc-400">
                没有匹配的审计记录
              </p>
            )}
          </div>
          <Pagination
            offset={filter.offset}
            more={query.data.has_more}
            change={(offset) => setFilter({ ...filter, offset })}
          />
        </>
      )}
    </div>
  );
}
export function ProfilesPage() {
  return (
    <AdminGate>
      <Profiles />
    </AdminGate>
  );
}
function Profiles() {
  const [offset, setOffset] = useState(0);
  const [search, setSearch] = useState("");
  const [q, setQ] = useState("");
  const query = useQuery({
    queryKey: ["registry", "profiles", { q, offset }],
    queryFn: ({ signal }) => listProfiles({ q, offset }, signal),
  });
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">用户与审核授权</h1>
        <p className="mt-2 text-sm leading-6 text-zinc-500">为已登录用户授予或撤销全平台审核资格，允许自审。审核员只负责待审材料的查看、通过与驳回；管理员始终保有审核权限。</p>
        <p className="mt-2 text-sm leading-6 text-zinc-500">
          此页维护历史记录使用的可信部门名称，变更会记录审计。新的部门选择使用飞书部门 ID，并根据本人真实归属授权；此处手工填写不会赋予新的飞书部门权限。
        </p>
      </header>
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          setQ(search.trim());
          setOffset(0);
        }}
      >
        <input
          aria-label="搜索用户"
          className={fieldClass}
          maxLength={200}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="姓名或 Open ID"
        />
        <button className={`${secondaryClass} shrink-0 whitespace-nowrap`}>
          搜索
        </button>
      </form>
      {query.isPending ? (
        <p role="status">正在加载用户…</p>
      ) : query.isError ? (
        <RegistryError error={query.error} retry={() => void query.refetch()} />
      ) : (
        <>
          <div className="space-y-4">
            {query.data.items.length === 0 && <p className="p-6 text-sm text-zinc-500">没有匹配的已登录用户。请先让对方通过飞书登录平台。</p>}
            {query.data.items.map((profile) => (
              <div key={`${profile.open_id}:${profile.revision}`} className="space-y-3 rounded-xl border border-zinc-200 bg-white p-5">
                <ReviewerAuthorization profile={profile} />
                <details>
                  <summary className="cursor-pointer text-sm text-zinc-500">历史可信部门资料</summary>
                  <ProfileRow profile={profile} />
                </details>
              </div>
            ))}
          </div>
          <Pagination
            offset={offset}
            more={query.data.has_more}
            change={setOffset}
          />
        </>
      )}
    </div>
  );
}
function ReviewerAuthorization({ profile }: { profile: GovernanceIdentity }) {
  const client = useQueryClient();
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  const granting = !profile.is_reviewer;
  return (
    <form className="space-y-4" aria-label={`审核资格：${profile.display_name || profile.open_id}`} onSubmit={async e => {
      e.preventDefault();
      setBusy(true); setError(undefined);
      try {
        await setReviewer(profile, granting, reason.trim());
        await client.invalidateQueries({ queryKey: ["registry"] });
      } catch (e) { setError(e); } finally { setBusy(false); }
    }}>
      <div>
        <h2 className="font-medium">{profile.display_name || "飞书用户"} <span className="ml-2 text-xs text-zinc-600">{profile.is_admin ? "管理员" : profile.is_reviewer ? "审核员" : "普通用户"}</span></h2>
        <p className="mt-1 break-all text-xs text-zinc-400">{profile.open_id}</p>
        <p className="mt-2 text-sm text-zinc-500">{profile.is_admin ? "管理员已拥有全平台审核权限，无需另行授予。" : profile.is_reviewer ? "可审核全平台 MCP / Skill，允许自审。" : "暂无审核资格，可管理自己的发布。"}</p>
      </div>
      {!profile.is_admin && <>
        <Field label="审核资格变更说明">
          <input className={fieldClass} required maxLength={2000} value={reason} onChange={e => setReason(e.target.value)} placeholder="填写授权或撤销原因，操作将记录审计" />
        </Field>
        <button className={secondaryClass} disabled={busy || !reason.trim()}>{busy ? "正在保存…" : granting ? "授予审核资格" : "撤销审核资格"}</button>
      </>}
      {error != null && <RegistryError error={error} retry={() => void client.invalidateQueries({ queryKey: ["registry"] })} />}
    </form>
  );
}
function ProfileRow({ profile }: { profile: GovernanceIdentity }) {
  const client = useQueryClient();
  const [department, setDepartment] = useState(profile.department);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>();
  async function save() {
    setBusy(true);
    setError(undefined);
    try {
      await setTrustedDepartment(profile, department.trim(), reason.trim());
      await client.invalidateQueries({ queryKey: ["registry"] });
    } catch (e) {
      setError(e);
    } finally {
      setBusy(false);
    }
  }
  return (
    <form
      className="space-y-4 rounded-xl border border-zinc-200 bg-white p-5"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <div>
        <h2 className="font-medium">
          {profile.display_name || "飞书用户"}
          {profile.is_admin && (
            <span className="ml-2 text-xs text-zinc-600">管理员</span>
          )}
        </h2>
        <p className="mt-1 break-all text-xs text-zinc-400">
          {profile.open_id}
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="可信部门">
          <input
            className={fieldClass}
            maxLength={120}
            value={department}
            onChange={(e) => setDepartment(e.target.value)}
            placeholder="留空则撤销部门访问"
          />
        </Field>
        <Field label="核实与变更说明">
          <input
            className={fieldClass}
            required
            maxLength={2000}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </Field>
      </div>
      {error != null && (
        <RegistryError
          error={error}
          retry={() =>
            void client.invalidateQueries({
              queryKey: ["registry", "profiles"],
            })
          }
        />
      )}
      <button
        className={`${secondaryClass} shrink-0 whitespace-nowrap`}
        disabled={busy || !reason.trim()}
      >
        {busy ? "正在保存…" : "保存部门"}
      </button>
    </form>
  );
}
