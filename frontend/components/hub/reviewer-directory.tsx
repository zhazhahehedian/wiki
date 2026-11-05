"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listReviewers } from "@/lib/api/registry";
import { RegistryError, secondaryClass } from "./registry-shared";

export function ReviewerDirectory() {
  const [offset, setOffset] = useState(0);
  const query = useQuery({
    queryKey: ["registry", "reviewers", offset],
    queryFn: ({ signal }) => listReviewers(offset, signal),
    refetchInterval: 30_000,
  });
  return (
    <section className="space-y-3 rounded-xl border border-zinc-200 bg-white p-5">
      <h2 className="font-semibold">谁可以审核</h2>
      <p className="text-sm leading-6 text-zinc-500">
        管理员和审核员共同处理全平台 MCP / Skill 待审队列，允许审核自己的发布。以下人员均可处理本次提交。
      </p>
      {query.isPending ? <p role="status">正在加载审核人员…</p> : query.isError ? (
        <RegistryError error={query.error} retry={() => void query.refetch()} />
      ) : (
        <>
          {query.data.items.length ? (
            <ul className="grid gap-3 sm:grid-cols-2">
              {query.data.items.map(person => (
                <li key={person.open_id} className="min-w-0 rounded-lg bg-zinc-50 p-3">
                  <p className="text-sm">{person.display_name || "飞书用户"} <span className="text-xs text-zinc-500">· {person.is_admin ? "管理员" : "审核员"}</span></p>
                  <p className="mt-1 break-all text-xs text-zinc-400">{person.open_id}</p>
                </li>
              ))}
            </ul>
          ) : <p role="status" className="rounded-lg bg-amber-50 p-3 text-sm text-amber-900">{offset ? "本页已无审核人员，请返回上一页。" : "当前没有可审核人员，请联系平台管理员配置审核资格。提交后将等待审核。"}</p>}
          {(offset > 0 || query.data.has_more) && <div className="flex justify-end gap-2">
            <button className={secondaryClass} disabled={!offset} onClick={() => setOffset(Math.max(0, offset - 24))}>上一页</button>
            <button className={secondaryClass} disabled={!query.data.has_more} onClick={() => setOffset(offset + 24)}>下一页</button>
          </div>}
        </>
      )}
    </section>
  );
}
