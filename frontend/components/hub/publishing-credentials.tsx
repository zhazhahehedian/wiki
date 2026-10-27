"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { createPublishingCredential, listPublishingCredentials, revokePublishingCredential, publishingScopes, type PublishingScope } from "@/lib/api/publishing";
import { Field, fieldClass, primaryClass, secondaryClass, RegistryError } from "./registry-shared";

const labels:Record<PublishingScope,string> = {"capabilities:read":"读取能力与下载", "capabilities:write":"创建、更新、上传和新版本草稿", "capabilities:submit":"提交审核"};
export function PublishingCredentials() {
  const client=useQueryClient();
  const query=useQuery({queryKey:["publishing","credentials"],queryFn:({signal})=>listPublishingCredentials(signal)});
  const [name,setName]=useState("");
  const [days,setDays]=useState(30);
  const [scopes,setScopes]=useState<PublishingScope[]>(["capabilities:read"]);
  const [token,setToken]=useState("");
  const [busy,setBusy]=useState(false);
  const [error,setError]=useState<unknown>();
  const [copied,setCopied]=useState(false);
  async function create(){
    setBusy(true);setError(undefined);setToken("");setCopied(false);
    try {
      const result=await createPublishingCredential({name,scopes,expires_at:new Date(Date.now()+days*86400000).toISOString()});
      setToken(result.token);setName("");await client.invalidateQueries({queryKey:["publishing","credentials"]});
    }catch(e){setError(e)}finally{setBusy(false)}
  }
  async function revoke(id:string){
    setBusy(true);setError(undefined);
    try{await revokePublishingCredential(id);setToken("");await client.invalidateQueries({queryKey:["publishing","credentials"]});}catch(e){setError(e)}finally{setBusy(false)}
  }
  return <section id="credentials" className="space-y-5 rounded-xl border border-zinc-200 bg-white p-5 sm:p-6">
    <div><h2 className="text-lg font-semibold">我的 API 凭证</h2><p className="mt-2 text-sm leading-6 text-zinc-500">凭证绑定当前飞书登录账号。只授予所选操作权限，能力归属和发布审核仍由服务端校验。创建后仅显示一次完整密钥。</p></div>
    <form className="space-y-4" onSubmit={e=>{e.preventDefault();void create();}}>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="凭证名称"><input required maxLength={80} className={fieldClass} value={name} onChange={e=>setName(e.target.value)} placeholder="例如：团队 CI 发布" /></Field>
        <Field label="有效天数（1–365）"><input type="number" min={1} max={365} required className={fieldClass} value={days} onChange={e=>setDays(Number(e.target.value))} /></Field>
      </div>
      <fieldset className="space-y-2"><legend className="mb-2 text-sm font-medium">允许的操作</legend>{publishingScopes.map(scope=><label key={scope} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={scopes.includes(scope)} onChange={e=>setScopes(e.target.checked?[...scopes,scope]:scopes.filter(s=>s!==scope))} />{labels[scope]}<code className="hidden text-xs text-zinc-400 lg:inline">{scope}</code></label>)}</fieldset>
      <button className={primaryClass} disabled={busy||scopes.length===0}>{busy?"正在处理…":"创建凭证"}</button>
    </form>
    {token&&<div className="space-y-3 rounded-lg border border-amber-200 bg-amber-50 p-4" role="status">
      <p className="text-sm font-medium text-amber-900">请立即保存，离开此页后无法再次查看。</p>
      <code className="block break-all text-sm" aria-label="新 API 密钥">{token}</code>
      <div className="flex gap-3"><button type="button" className={secondaryClass} onClick={async()=>{try{await navigator.clipboard.writeText(token);setCopied(true)}catch{setError(new Error("复制失败，请手动复制密钥。"))}}}>{copied?"已复制":"复制密钥"}</button><button type="button" className={secondaryClass} onClick={()=>setToken("")}>已保存，隐藏密钥</button></div>
    </div>}
    {!!error&&<RegistryError error={error}/>}
    {query.isPending?<p role="status">正在加载凭证…</p>:query.isError?<RegistryError error={query.error} retry={()=>void query.refetch()}/>:<div className="space-y-3">
      {query.data.items.length===0&&<p className="text-sm text-zinc-500">尚未创建凭证。</p>}
      {query.data.items.map(c=>{
        const expired=new Date(c.expires_at).getTime()<=Date.now();
        return <article key={c.id} className="flex flex-wrap items-start justify-between gap-3 border-t border-zinc-100 pt-4">
          <div className="min-w-0 space-y-1"><p className="break-all font-medium">{c.name} <span className="text-xs text-zinc-500">{c.revoked_at?"已撤销":expired?"已过期":"有效"}</span></p><p className="text-xs text-zinc-500">{c.prefix}… · 到期 {new Date(c.expires_at).toLocaleString("zh-CN")}</p><p className="break-all text-xs text-zinc-500">{c.scopes.map(s=>labels[s]).join(" · ")}</p><p className="text-xs text-zinc-400">最近使用：{c.last_used_at?new Date(c.last_used_at).toLocaleString("zh-CN"):"尚未使用"}</p></div>
          {!c.revoked_at&&<button type="button" disabled={busy} className={secondaryClass} onClick={()=>void revoke(c.id)}>撤销 {c.name}</button>}
        </article>;
      })}
    </div>}
  </section>;
}
