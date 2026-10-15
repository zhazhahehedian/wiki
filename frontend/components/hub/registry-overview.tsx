"use client";

import { Select } from "@/components/ui/select";

import { useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  Grid2X2,
  List,
  Plus,
  Library,
  ArrowUpRight,
  Search,
} from "lucide-react";
import {
  listCapabilities,
  listCatalog,
  type RegistryFilter,
} from "@/lib/api/registry";
import { cn } from "@/lib/utils";
import {
  primaryClass,
  secondaryClass,
  StatusBadge,
  RegistryError,
} from "./registry-shared";

export function RegistryOverview({ mine = false }: { mine?: boolean }) {
  const [view, setView] = useState<"grid" | "table">("grid");
  const [filter, setFilter] = useState<RegistryFilter>({ offset: 0 });
  const [search, setSearch] = useState("");
  const [department, setDepartment] = useState("");
  const query = useQuery({
    queryKey: ["registry", mine ? "owned" : "catalog", filter],
    queryFn: ({ signal }) =>
      (mine ? listCapabilities : listCatalog)(filter, signal),
  });
  function change(next: RegistryFilter) {
    setFilter({ ...filter, ...next, offset: 0 });
  }
  const items = query.data?.items ?? [];
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {mine ? "我的发布" : "能力市场"}
          </h1>
          <p className="mt-2 text-sm text-zinc-500">
            {mine
              ? "管理你的能力草稿与版本。"
              : "发现团队已发布的 Skill 和 MCP，下载使用，或基于现成 Skill 创建自己的版本。"}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link href="/hub/registry/create-skill" className={secondaryClass}>
            对话创建 Skill
          </Link>
          <Link href="/hub/registry/new" className={primaryClass}>
            <Plus className="size-4" />
            发布能力
          </Link>
        </div>
      </div>
      <p className="rounded-xl border border-zinc-100 bg-zinc-50/60 px-4 py-3 text-xs leading-6 text-zinc-700">
        {mine
          ? "维护草稿、提交审核与发布新版本；审核新版期间，已上线版本继续可用。"
          : "这里展示已上线且你有权访问的能力。新建或待审内容请前往「我的发布」。"}
      </p>
      {!mine && (
        <div
          className="flex flex-wrap items-center gap-2"
          role="group"
          aria-label="场景快捷搜索"
        >
          <span className="mr-1 text-xs text-zinc-500">按场景找 Skill</span>
          {["周报", "需求", "代码", "数据", "文档"].map((scene) => (
            <button
              key={scene}
              aria-pressed={filter.q === scene && filter.type === "skill"}
              className={secondaryClass}
              onClick={() => {
                setSearch(scene);
                change({ type: "skill", q: scene });
              }}
            >
              {scene}
            </button>
          ))}
          <button
            className={secondaryClass}
            onClick={() => {
              setSearch("");
              setDepartment("");
              setFilter({ offset: 0 });
            }}
          >
            清除筛选
          </button>
        </div>
      )}
      <section className="overflow-hidden rounded-xl border border-zinc-200/80 bg-white shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-4 border-b border-zinc-100 px-5 py-4">
          <div
            role="group"
            aria-label="能力类型"
            className="flex gap-1 rounded-lg bg-zinc-100 p-1"
          >
            {[
              ["", "全部能力"],
              ["mcp", "MCP 服务"],
              ["skill", "Skill"],
            ].map(([value, label]) => (
              <button
                key={value}
                onClick={() => change({ type: value })}
                aria-pressed={(filter.type ?? "") === value}
                className={cn(
                  "rounded-md px-3 py-1.5 text-xs",
                  (filter.type ?? "") === value
                    ? "bg-white font-medium shadow-sm"
                    : "text-zinc-500",
                )}
              >
                {label}
              </button>
            ))}
          </div>
          <div
            role="group"
            aria-label="显示方式"
            className="flex rounded-lg border border-zinc-200 p-1"
          >
            {(
              [
                { value: "grid", label: "卡片视图", icon: Grid2X2 },
                { value: "table", label: "表格视图", icon: List },
              ] as const
            ).map(({ value, label, icon: Icon }) => (
              <button
                key={value}
                aria-label={label}
                aria-pressed={view === value}
                onClick={() => setView(value)}
                className={cn(
                  "rounded-md p-2",
                  view === value ? "bg-zinc-50 text-zinc-600" : "text-zinc-400",
                )}
              >
                <Icon className="size-4" />
              </button>
            ))}
          </div>
        </div>
        <form
          className="flex flex-wrap gap-3 border-b border-zinc-100 p-5"
          onSubmit={(e) => {
            e.preventDefault();
            change({ q: search.trim(), department: department.trim() });
          }}
        >
          <input
            aria-label="搜索能力"
            maxLength={200}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索名称、描述、标识…"
            className="min-w-0 basis-full rounded-lg border border-zinc-200 px-3 py-2 text-sm sm:flex-1 sm:basis-0"
          />
          <input
            aria-label="筛选部门"
            maxLength={120}
            value={department}
            onChange={(e) => setDepartment(e.target.value)}
            placeholder="部门"
            className="w-32 rounded-lg border border-zinc-200 px-3 py-2 text-sm"
          />
          {mine && (
            <Select
              aria-label="筛选状态"
              value={filter.status ?? ""}
              onValueChange={(value) => change({ status: value })}
              className="w-auto rounded-lg border border-zinc-200 px-3 py-2 text-sm"
              options={[
                { value: "", label: "全部状态" },
                { value: "draft", label: "草稿" },
                { value: "in_review", label: "审核中" },
                { value: "published", label: "已上线" },
                { value: "offline", label: "已下线" },
              ]}
            />
          )}
          <button className={secondaryClass}>
            <Search className="size-4" />
            搜索
          </button>
        </form>
        {query.isPending ? (
          <p role="status" className="p-12 text-center text-sm text-zinc-500">
            正在加载能力…
          </p>
        ) : query.isError ? (
          <div className="p-6">
            <RegistryError
              error={query.error}
              retry={() => void query.refetch()}
            />
          </div>
        ) : (
          <>
            {view === "table" && (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[620px] text-left text-sm">
                  <caption className="sr-only">能力列表</caption>
                  <thead className="bg-zinc-50 text-xs text-zinc-500">
                    <tr>
                      {["能力名称", "类型", "部门", "状态", "更新时间"].map(
                        (label) => (
                          <th key={label} className="px-5 py-3 font-medium">
                            {label}
                          </th>
                        ),
                      )}
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((item) => (
                      <tr key={item.id} className="border-t border-zinc-100">
                        <td className="px-5 py-4">
                          <Link
                            href={`/hub/registry/${encodeURIComponent(item.slug)}${mine ? "" : "/market"}`}
                            className="font-medium text-zinc-600 hover:underline"
                          >
                            {item.name}
                          </Link>
                          <p className="mt-1 text-xs text-zinc-400">
                            {item.slug}
                          </p>
                        </td>
                        <td className="px-5 py-4 uppercase">{item.type}</td>
                        <td className="px-5 py-4">{item.department || "—"}</td>
                        <td className="px-5 py-4">
                          <StatusBadge status={item.status} />
                        </td>
                        <td className="px-5 py-4 text-xs text-zinc-500">
                          {new Date(item.updated_at).toLocaleString("zh-CN")}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            {view === "grid" && items.length > 0 && (
              <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,18rem),1fr))] gap-4 p-5">
                {items.map((item) => (
                  <Link
                    key={item.id}
                    href={`/hub/registry/${encodeURIComponent(item.slug)}${mine ? "" : "/market"}`}
                    className="group flex min-w-0 flex-col rounded-xl border border-zinc-200 p-5 transition hover:border-zinc-300 hover:shadow-md"
                  >
                    <div className="flex items-center justify-between">
                      <span className="rounded-lg bg-zinc-50 px-2.5 py-1.5 text-xs font-semibold uppercase text-zinc-600">
                        {item.type}
                      </span>
                      {mine ? (
                        <StatusBadge status={item.status} />
                      ) : (
                        <span className="text-xs text-zinc-500">
                          {item.published_version
                            ? `v${item.published_version}`
                            : "已上线"}
                        </span>
                      )}
                    </div>
                    <h2 className="mt-4 break-words font-semibold">
                      {item.name}
                    </h2>
                    <p className="mt-2 line-clamp-3 min-h-16 text-sm leading-6 text-zinc-500">
                      {item.description}
                    </p>
                    <div className="mt-5 flex items-center justify-between border-t border-zinc-100 pt-4 text-xs text-zinc-400">
                      <span className="truncate">
                        {item.department || "未设置部门"} · {item.slug}
                      </span>
                      <ArrowUpRight className="ml-2 size-4 shrink-0 group-hover:text-zinc-600" />
                    </div>
                    {!mine && (
                      <p
                        className="mt-3 truncate text-xs text-zinc-500"
                        title={item.publisher_name || item.owner_open_id}
                      >
                        发布者 · {item.publisher_name || item.owner_open_id}
                      </p>
                    )}
                  </Link>
                ))}
              </div>
            )}
            {items.length === 0 && (
              <div className="flex min-h-72 flex-col items-center justify-center px-6 py-12 text-center">
                <Library className="size-10 text-zinc-300" />
                <h2 className="mt-5 font-medium">
                  {filter.q ||
                  filter.type ||
                  filter.department ||
                  filter.status ||
                  filter.offset
                    ? "没有匹配的能力"
                    : mine
                      ? "从第一个能力开始"
                      : "暂无可用能力"}
                </h2>
                <p className="mt-2 text-sm text-zinc-400">
                  {mine
                    ? "创建 MCP 或 Skill 草稿，在这里维护版本和接入信息。"
                    : "试试其他关键词或清除筛选；审核上线且对你可见的能力会出现在这里。"}
                </p>
              </div>
            )}
            <div className="flex items-center justify-between border-t border-zinc-100 px-5 py-4 text-xs text-zinc-500">
              <span>本页 {items.length} 项</span>
              <div className="flex gap-2">
                <button
                  className={secondaryClass}
                  disabled={!filter.offset}
                  onClick={() =>
                    setFilter({
                      ...filter,
                      offset: Math.max(0, (filter.offset ?? 0) - 24),
                    })
                  }
                >
                  上一页
                </button>
                <button
                  className={secondaryClass}
                  disabled={!query.data?.has_more}
                  onClick={() =>
                    setFilter({ ...filter, offset: (filter.offset ?? 0) + 24 })
                  }
                >
                  下一页
                </button>
              </div>
            </div>
          </>
        )}
      </section>
    </div>
  );
}
