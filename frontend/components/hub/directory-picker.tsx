"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listDirectoryDepartments, listDirectoryUsers, type DirectoryDepartment } from "@/lib/api/directory";
import { authApi } from "@/lib/api/auth";
import { APIError } from "@/lib/api/client";
import { fieldClass, secondaryClass } from "./registry-shared";

type Props = { kind: "department"; value: string; label?: string; onChange: (id: string, name: string) => void }
  | { kind: "users"; value: string[]; onChange: (ids: string[]) => void; max?: number };

export function DirectoryPicker(props: Props) {
  const [open, setOpen] = useState(false);
  const [trail, setTrail] = useState<DirectoryDepartment[]>([{ open_department_id: "0", name: "组织架构" }]);
  const [departmentPage, setDepartmentPage] = useState("");
  const [userPage, setUserPage] = useState("");
  const [search, setSearch] = useState("");
  const [names, setNames] = useState<Record<string, string>>({});
  const parent = trail[trail.length - 1];
  const departments = useQuery({
    queryKey: ["directory", "departments", parent.open_department_id, departmentPage],
    queryFn: ({ signal }) => listDirectoryDepartments(parent.open_department_id, departmentPage, signal),
    enabled: open, staleTime: 0, retry: false,
  });
  const users = useQuery({
    queryKey: ["directory", "users", parent.open_department_id, userPage],
    queryFn: ({ signal }) => listDirectoryUsers(parent.open_department_id, userPage, signal),
    enabled: open && props.kind === "users", staleTime: 0, retry: false,
  });
  function navigate(next: DirectoryDepartment[]) { setTrail(next); setDepartmentPage(""); setUserPage(""); setSearch(""); }
  const error = departments.error || (props.kind === "users" ? users.error : undefined);
  return <div className="space-y-3">
    <div className="flex flex-wrap items-center gap-2">
      <button type="button" className={secondaryClass} aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? "收起选择" : props.kind === "department" ? "选择飞书部门" : "选择飞书用户"}
      </button>
      {props.kind === "department" && (props.value || props.label) && <>
        <span className="break-all text-sm">{props.label || props.value}</span>
        <button type="button" className="text-xs text-zinc-500 underline" onClick={() => props.onChange("", "")}>清除部门</button>
      </>}
      {props.kind === "users" && <span className="text-xs text-zinc-500">已选 {props.value.length} / {props.max ?? 200} 人</span>}
    </div>
    {props.kind === "users" && props.value.length > 0 && <ul className="flex flex-wrap gap-2">
      {props.value.map(id => <li key={id} className="max-w-full break-all rounded-md bg-blue-50 px-2 py-1 text-xs">
        {names[id] || id} <button type="button" aria-label={`移除 ${names[id] || id}`} onClick={() => props.onChange(props.value.filter(v => v !== id))}>×</button>
      </li>)}
    </ul>}
    {open && <div className="space-y-3 rounded-lg border border-zinc-200 bg-zinc-50 p-4" aria-label="飞书通讯录选择">
      <p className="text-xs leading-5 text-zinc-500">只展示你在飞书中可见的组织与人员。能看见某部门不代表你属于该部门。</p>
      {(departments.data?.limited_root || users.data?.limited_root) && <p className="text-xs leading-5 text-amber-700">飞书限制了组织根节点访问。请从可访问的本人部门进入，继续查看下级与直属人员。</p>}
      <div className="flex flex-wrap gap-2">{trail.map((d, i) => <button type="button" key={d.open_department_id} className="text-sm text-blue-700 underline" onClick={() => navigate(trail.slice(0, i + 1))}>{d.name}{i < trail.length - 1 ? " /" : ""}</button>)}</div>
      <input aria-label="筛选当前页" placeholder="筛选当前页名称或 ID" className={fieldClass} value={search} onChange={e => setSearch(e.target.value)} />
      {error && <div role="alert" className="space-y-2 text-sm text-red-700">
        <p>{error instanceof Error ? error.message : "通讯录加载失败"}</p>
        {error instanceof APIError && error.code === "directory_authorization_required" && <a className="underline" href={authApi.feishuStartURL()}>重新授权飞书</a>}
        <button type="button" className="ml-3 underline" onClick={() => { void departments.refetch(); if (props.kind === "users") void users.refetch(); }}>重试</button>
      </div>}
      {departments.isFetching && <p role="status" className="text-sm">正在加载部门…</p>}
      <ul className="space-y-2">{departments.data?.items.filter(d => `${d.name} ${d.open_department_id}`.toLowerCase().includes(search.toLowerCase())).map(d => <li key={d.open_department_id} className="flex flex-wrap items-center justify-between gap-2 rounded bg-white p-2 text-sm">
        <span>{d.name}<span className="mt-1 block break-all text-xs text-zinc-400">{d.open_department_id}</span></span><span className="flex gap-3">
          <button type="button" className="text-blue-700" onClick={() => navigate([...trail, d])}>查看下级{props.kind === "users" ? "与人员" : ""}</button>
          {props.kind === "department" && <button type="button" className="text-blue-700" onClick={() => { props.onChange(d.open_department_id, d.name); setOpen(false); }}>选择 {d.name}</button>}
        </span>
      </li>)}</ul>
      {departments.data?.items.length === 0 && <p className="text-xs text-zinc-500">当前无可见子部门</p>}
      <PageButtons page={departmentPage} more={departments.data?.has_more} busy={departments.isFetching} next={() => setDepartmentPage(departments.data?.page_token ?? "")} first={() => setDepartmentPage("")} label="部门" />
      {props.kind === "users" && <>
        <p className="text-sm font-medium">{users.data?.limited_root ? "进入上方部门查看人员" : `${parent.name}直属人员`}</p>
        {users.isFetching && <p role="status" className="text-sm">正在加载人员…</p>}
        <ul className="space-y-2">{users.data?.items.filter(u => `${u.name} ${u.open_id}`.toLowerCase().includes(search.toLowerCase())).map(u => <li key={u.open_id}>
          <label className="flex items-center gap-2 rounded bg-white p-2 text-sm"><input type="checkbox" checked={props.value.includes(u.open_id)} disabled={!props.value.includes(u.open_id) && props.value.length >= (props.max ?? 200)} onChange={e => {
            setNames(prev => ({ ...prev, [u.open_id]: u.name }));
            props.onChange(e.target.checked ? [...props.value, u.open_id] : props.value.filter(v => v !== u.open_id));
          }} /><span>{u.name}<span className="ml-2 break-all text-xs text-zinc-400">{u.open_id}</span></span></label>
        </li>)}</ul>
        {users.data?.items.length === 0 && !users.data.limited_root && <p className="text-xs text-zinc-500">当前无可见直属人员</p>}
        <PageButtons page={userPage} more={users.data?.has_more} busy={users.isFetching} next={() => setUserPage(users.data?.page_token ?? "")} first={() => setUserPage("")} label="人员" />
      </>}
    </div>}
  </div>;
}
function PageButtons({page, more, busy, next, first, label}: {page:string; more?:boolean; busy:boolean; next:()=>void; first:()=>void; label:string}) {
  return <div className="flex gap-3 text-xs text-blue-700">{page && <button type="button" disabled={busy} onClick={first}>{label}首页</button>}{more && <button type="button" disabled={busy} onClick={next}>下一页{label}</button>}</div>;
}
