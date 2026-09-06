"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  getGovernanceIdentity,
  listReviews,
  listAudit,
  listProfiles,
  setTrustedDepartment,
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
  const query = useQuery({
    queryKey: ["registry", "identity"],
    queryFn: ({ signal }) => getGovernanceIdentity(signal),
  });
  if (query.isPending) return <p role="status">正在检查管理员权限…</p>;
  if (query.isError)
    return (
      <RegistryError error={query.error} retry={() => void query.refetch()} />
    );
  if (!query.data.is_admin)
    return (
      <div className="rounded-xl border border-zinc-200 bg-white p-8">
        <h1 className="font-semibold">需要管理员权限</h1>
        <p className="mt-3 text-sm text-zinc-500">
          当前账号可浏览注册中心并管理自己的发布。
        </p>
        <Link
          href="/hub/registry"
          className="mt-4 inline-block text-sm text-zinc-600"
        >
          返回注册中心
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
    <AdminGate>
      <ReviewQueue />
    </AdminGate>
  );
}
function ReviewQueue() {
  const [offset, setOffset] = useState(0);
  const [search, setSearch] = useState("");
  const [q, setQ] = useState("");
  const query = useQuery({
    queryKey: ["registry", "reviews", { q, offset }],
    queryFn: ({ signal }) => listReviews({ q, offset }, signal),
  });
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">审核队列</h1>
        <p className="mt-2 text-sm text-zinc-500">
          按提交时间处理。打开详情查看待审版本、Skill 文件和上线版本对照。
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
  };
  return (
    <div className="space-y-6">
      <header>
        <h1 className="text-2xl font-semibold">审计日志</h1>
        <p className="mt-2 text-sm text-zinc-500">
          查看能力治理与部门授权变更，按操作者、动作或目标筛选。
        </p>
      </header>
      <form
        className="grid gap-3 rounded-xl border border-zinc-200 bg-white p-4 sm:grid-cols-2 lg:grid-cols-4"
        onSubmit={(e) => {
          e.preventDefault();
          setFilter({ ...form, offset: 0 });
        }}
      >
        <select
          aria-label="审计动作"
          className={fieldClass}
          value={form.action}
          onChange={(e) => setForm({ ...form, action: e.target.value })}
        >
          <option value="">全部动作</option>
          {Object.entries(labels).map(([value, label]) => (
            <option key={value} value={value}>
              {label}
            </option>
          ))}
        </select>
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
        <h1 className="text-2xl font-semibold">部门授权</h1>
        <p className="mt-2 text-sm leading-6 text-zinc-500">
          由管理员核实并维护已登录用户的部门。未设置部门的用户无法访问部门范围能力，变更会记录审计。
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
            {query.data.items.map((profile) => (
              <ProfileRow
                key={`${profile.open_id}:${profile.revision}`}
                profile={profile}
              />
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
