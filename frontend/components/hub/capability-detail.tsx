"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Pencil, Download } from "lucide-react";
import { GovernanceControls } from "./governance-controls";
import { SkillFiles } from "./skill-files";
import { getCapability, skillBundleURL } from "@/lib/api/registry";
import {
  primaryClass,
  secondaryClass,
  StatusBadge,
  RegistryError,
  visibilityLabels,
} from "./registry-shared";

export function CapabilityDetailPage({ slug }: { slug: string }) {
  const query = useQuery({
    queryKey: ["registry", "detail", slug],
    queryFn: ({ signal }) => getCapability(slug, signal),
  });
  const [selected, setSelected] = useState("");
  if (query.isPending) return <p role="status">正在加载能力…</p>;
  if (query.isError)
    return (
      <RegistryError error={query.error} retry={() => void query.refetch()} />
    );
  const item = query.data;
  const version =
    item.versions.find(
      (v) =>
        v.id === (selected || item.draft_version_id || item.current_version_id),
    ) ?? item.versions[0];
  return (
    <div className="space-y-6">
      <Link
        href="/hub/registry"
        className="inline-flex items-center gap-2 text-sm text-zinc-500"
      >
        <ArrowLeft className="size-4" />
        注册中心
      </Link>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <span className="text-xs font-semibold uppercase text-zinc-600">
              {item.type}
            </span>
            <StatusBadge status={item.status} />
          </div>
          <h1 className="mt-3 break-words text-2xl font-semibold">
            {item.name}
          </h1>
          <p className="mt-2 text-sm text-zinc-400">{item.slug}</p>
        </div>
        {item.is_owner && item.status === "draft" && (
          <Link className={primaryClass} href={`/hub/registry/${slug}/edit`}>
            <Pencil className="size-4" />
            编辑草稿
          </Link>
        )}
      </header>
      <GovernanceControls key={item.revision} item={item} />
      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="min-w-0 space-y-6">
          <section className="rounded-xl border border-zinc-200 bg-white p-6">
            <h2 className="font-semibold">能力介绍</h2>
            <p className="mt-4 whitespace-pre-wrap break-words text-sm leading-7 text-zinc-600">
              {item.description}
            </p>
          </section>
          <section className="rounded-xl border border-zinc-200 bg-white p-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h2 className="font-semibold">版本内容</h2>
              <select
                aria-label="查看版本"
                value={version?.id ?? ""}
                onChange={(e) => setSelected(e.target.value)}
                className="max-w-full rounded-lg border border-zinc-200 px-3 py-2 text-sm"
              >
                {item.versions.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.version}
                    {v.id === item.current_version_id
                      ? " · 上线版本"
                      : v.id === item.draft_version_id
                        ? item.status === "in_review"
                          ? " · 待审版本"
                          : " · 当前草稿"
                        : " · 历史版本"}
                  </option>
                ))}
              </select>
            </div>
            {version?.changelog && (
              <p className="mt-4 whitespace-pre-wrap text-sm text-zinc-500">
                {version.changelog}
              </p>
            )}
            {item.type === "mcp" ? (
              <div className="mt-5 space-y-3">
                {version?.tools?.length ? (
                  version.tools.map((tool) => (
                    <details
                      key={tool.name}
                      className="rounded-xl border border-zinc-200 p-4"
                    >
                      <summary className="cursor-pointer text-sm font-medium">
                        {tool.name}
                      </summary>
                      <p className="mt-3 whitespace-pre-wrap text-sm text-zinc-500">
                        {tool.description}
                      </p>
                      <pre className="mt-3 overflow-x-auto rounded-lg bg-zinc-50 p-3 text-xs">
                        {JSON.stringify(tool.inputSchema, null, 2)}
                      </pre>
                    </details>
                  ))
                ) : (
                  <p className="text-sm text-zinc-400">尚未声明工具。</p>
                )}
              </div>
            ) : (
              <div className="mt-5 space-y-4">
                {version?.has_bundle && (
                  <SkillFiles slug={slug} version={version.id} />
                )}
                {version?.has_bundle && (
                  <a
                    href={skillBundleURL(slug, version.id)}
                    className={secondaryClass}
                  >
                    <Download className="size-4" />
                    下载 Skill 包
                  </a>
                )}
              </div>
            )}
          </section>
          {item.published &&
            item.draft_version_id &&
            item.draft_version_id !== item.current_version_id && (
              <section className="rounded-xl border border-zinc-200 bg-white p-6">
                <h2 className="font-semibold">本次提交与上线版本</h2>
                <p className="mt-2 text-xs leading-6 text-zinc-500">
                  对照元信息、接入规格或实际文件。审批后本次提交的版本与可见范围一并生效。
                </p>
                <div className="mt-4 grid min-w-0 gap-4 md:grid-cols-2">
                  <div className="min-w-0">
                    <h3 className="mb-3 text-sm font-medium">当前上线</h3>
                    <pre className="whitespace-pre-wrap break-words rounded bg-zinc-50 p-3 text-xs leading-6">
                      {JSON.stringify(item.published, null, 2)}
                    </pre>
                    {item.type === "skill" && item.current_version_id ? (
                      <SkillFiles
                        slug={slug}
                        version={item.current_version_id}
                      />
                    ) : (
                      <pre className="mt-3 whitespace-pre-wrap break-all rounded bg-zinc-50 p-3 text-xs leading-6">
                        {JSON.stringify(
                          item.versions.find(
                            (v) => v.id === item.current_version_id,
                          ),
                          null,
                          2,
                        )}
                      </pre>
                    )}
                  </div>
                  <div className="min-w-0">
                    <h3 className="mb-3 text-sm font-medium">本次提交</h3>
                    <pre className="whitespace-pre-wrap break-words rounded bg-zinc-50 p-3 text-xs leading-6">
                      {JSON.stringify(
                        {
                          name: item.name,
                          description: item.description,
                          department: item.department,
                          visibility: item.visibility,
                          allowlist: item.allowlist,
                        },
                        null,
                        2,
                      )}
                    </pre>
                    {item.type === "skill" ? (
                      <SkillFiles slug={slug} version={item.draft_version_id} />
                    ) : (
                      <pre className="mt-3 whitespace-pre-wrap break-all rounded bg-zinc-50 p-3 text-xs leading-6">
                        {JSON.stringify(
                          item.versions.find(
                            (v) => v.id === item.draft_version_id,
                          ),
                          null,
                          2,
                        )}
                      </pre>
                    )}
                  </div>
                </div>
              </section>
            )}
          <section className="rounded-xl border border-zinc-200 bg-white p-6">
            <h2 className="font-semibold">版本历史</h2>
            <ul className="mt-4 divide-y divide-zinc-100">
              {item.versions.map((v) => (
                <li
                  key={v.id}
                  className="flex flex-wrap justify-between gap-2 py-3 text-sm"
                >
                  <button
                    className="font-medium text-zinc-600 hover:underline"
                    onClick={() => setSelected(v.id)}
                  >
                    {v.version}
                    {v.id === item.current_version_id
                      ? " · 上线版本"
                      : v.id === item.draft_version_id
                        ? item.status === "in_review"
                          ? " · 待审版本"
                          : " · 当前草稿"
                        : ""}
                  </button>
                  <span className="text-xs text-zinc-400">
                    {new Date(v.created_at).toLocaleString("zh-CN")}
                  </span>
                </li>
              ))}
            </ul>
          </section>
        </div>
        <aside className="min-w-0 space-y-6">
          {item.type === "mcp" && (
            <section className="rounded-xl border border-zinc-200 bg-white p-5">
              <h2 className="font-semibold">接入信息</h2>
              <dl className="mt-4 space-y-4 text-sm">
                <div>
                  <dt className="text-xs text-zinc-400">Endpoint</dt>
                  <dd className="mt-2 break-all rounded-lg bg-zinc-50 p-3 font-mono text-xs">
                    {version?.mcp_endpoint}
                  </dd>
                </div>
                <div>
                  <dt className="text-xs text-zinc-400">传输协议</dt>
                  <dd className="mt-1">{version?.mcp_transport}</dd>
                </div>
                <div>
                  <dt className="text-xs text-zinc-400">鉴权方式</dt>
                  <dd className="mt-1">{version?.mcp_auth_scheme}</dd>
                </div>
              </dl>
              <p className="mt-5 border-t border-zinc-100 pt-4 text-xs leading-5 text-zinc-400">
                接入凭证与健康检查尚未开放。
              </p>
            </section>
          )}
          <section className="rounded-xl border border-zinc-200 bg-white p-5">
            <h2 className="font-semibold">发布信息</h2>
            {item.published && (
              <details className="mt-4 rounded-lg bg-zinc-50 p-3">
                <summary className="cursor-pointer text-xs font-medium">
                  查看已生效的上线元信息与范围
                </summary>
                <pre className="mt-3 whitespace-pre-wrap break-words text-xs leading-6">
                  {JSON.stringify(item.published, null, 2)}
                </pre>
              </details>
            )}
            <dl className="mt-4 space-y-4 text-sm">
              <div>
                <dt className="text-xs text-zinc-400">上线后可见范围</dt>
                <dd className="mt-1">
                  {visibilityLabels[item.visibility]}
                  {item.visibility === "allowlist" && (
                    <ul className="mt-2 space-y-1 break-all text-xs">
                      {item.allowlist?.map((id) => (
                        <li key={id}>{id}</li>
                      ))}
                    </ul>
                  )}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-400">部门</dt>
                <dd className="mt-1">{item.department || "未设置"}</dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-400">负责人 Open ID</dt>
                <dd className="mt-1 break-all text-xs">{item.owner_open_id}</dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-400">更新时间</dt>
                <dd className="mt-1 text-xs">
                  {new Date(item.updated_at).toLocaleString("zh-CN")}
                </dd>
              </div>
            </dl>
            <p className="mt-5 text-xs leading-5 text-zinc-600">
              {item.is_owner || item.is_admin
                ? "管理视图：包含草稿、审核状态和历史版本。"
                : "当前展示已审核上线且你有权访问的版本。"}
            </p>
          </section>
        </aside>
      </div>
    </div>
  );
}
