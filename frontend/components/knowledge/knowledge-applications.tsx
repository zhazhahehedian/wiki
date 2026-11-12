"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  Check,
  ChevronRight,
  Copy,
  KeyRound,
  MoreHorizontal,
  Plus,
  RefreshCw,
  Settings2,
  ShieldAlert,
  Unplug,
} from "lucide-react";
import { toast } from "sonner";
import { knowledge, type Application } from "@/lib/api/knowledge";
import { getGovernanceIdentity } from "@/lib/api/registry";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { confirm } from "@/components/ui/confirm-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { fieldClass } from "@/components/hub/registry-shared";
import { cn } from "@/lib/utils";
import {
  ErrorMessage,
  errorText,
  useInvalidateKnowledge,
  useKnowledgeBases,
} from "./shared";

const time = (v?: string) => (v ? new Date(v).toLocaleString() : "—");

function AuditLog({ names }: { names: Map<string, string> }) {
  const entries = useQuery({
    queryKey: ["knowledge", "audit"],
    queryFn: ({ signal }) => knowledge.audit(signal),
  });
  return (
    <section className="rounded-xl border border-zinc-200 bg-white">
      <div className="border-b border-zinc-100 px-5 py-4">
        <h2 className="font-semibold">最近调用记录</h2>
        <p className="mt-1 text-xs text-zinc-500">
          最近 100 次应用调用；只记录请求编号、知识库、状态、耗时和结果数，不记录凭证、问题或正文。
        </p>
      </div>
      <div className="p-5">
        <ErrorMessage error={entries.error} />
        {entries.data?.length === 0 && (
          <p className="text-sm text-zinc-500">尚无调用记录。</p>
        )}
        <ul className="max-h-72 divide-y divide-zinc-100 overflow-auto text-xs">
          {entries.data?.map((e, i) => (
            <li
              key={`${e.request_id}-${i}`}
              className="grid gap-1 py-2 sm:grid-cols-[10rem_minmax(0,1fr)_auto] sm:gap-3"
            >
              <span className="text-zinc-500">{time(e.created_at)}</span>
              <span className="min-w-0 break-all">
                {(e.application_id && names.get(e.application_id)) || e.application_id || "人员检索"}
                <span className="text-zinc-400"> · {e.request_id}</span>
              </span>
              <span className="flex gap-2 text-zinc-500">
                <Badge variant={e.status === "ok" ? "success" : "muted"}>
                  {e.status}
                </Badge>
                {e.duration_ms} ms · {e.result_count} 条
              </span>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}

export function KnowledgeApplications() {
  const apps = useQuery({
    queryKey: ["knowledge", "applications"],
    queryFn: ({ signal }) => knowledge.applications(signal),
  });
  const identity = useQuery({
    queryKey: ["registry", "identity"],
    queryFn: ({ signal }) => getGovernanceIdentity(signal),
  });
  const isAdmin = !!identity.data?.is_admin;
  const bases = useKnowledgeBases();
  const invalidate = useInvalidateKnowledge();
  const [editing, setEditing] = useState<Application | "new" | null>(null);
  const [name, setName] = useState("");
  const [enabled, setEnabled] = useState(true);
  const [ids, setIDs] = useState<string[]>([]);
  const [token, setToken] = useState("");
  const [copied, setCopied] = useState(false);
  const save = useMutation({
    mutationFn: () =>
      knowledge.saveApplication(editing === "new" || !editing ? "" : editing.id, {
        name,
        enabled,
        knowledge_base_ids: ids,
      }),
    onSuccess: () => {
      toast.success(editing === "new" ? "应用已创建，下一步签发凭证" : "应用已保存");
      setEditing(null);
      return invalidate();
    },
  });
  const action = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: invalidate,
    onError: (e) => toast.error(errorText(e)),
  });
  const managed = bases.data?.filter((b) => b.manage) ?? [];
  const baseName = new Map(bases.data?.map((b) => [b.id, b.name] as const));
  const appName = new Map(apps.data?.map((a) => [a.id, a.name] as const));
  // Grants this user cannot add (another manager's base, or one later deleted)
  // can be kept or removed while editing, never newly added.
  const fixed = ids.filter((id) => !managed.some((b) => b.id === id));

  function edit(a?: Application) {
    setEditing(a ?? "new");
    setName(a?.name ?? "");
    setEnabled(a?.enabled ?? true);
    setIDs(a?.knowledge_base_ids ?? []);
    save.reset();
  }
  function toggle(id: string, checked: boolean) {
    setIDs(checked ? [...ids, id] : ids.filter((x) => x !== id));
  }
  async function rotate(a: Application) {
    if (
      a.credential_prefix &&
      !(await confirm({
        title: `轮换“${a.name}”的凭证？`,
        description: "旧凭证会立即失效，调用方需要换上新凭证。",
        confirmText: "轮换",
        destructive: true,
      }))
    )
      return;
    action.mutate(async () => {
      setToken("");
      setCopied(false);
      setToken((await knowledge.rotate(a.id)).token);
    });
  }
  async function revoke(a: Application) {
    if (
      await confirm({
        title: `撤销“${a.name}”的凭证？`,
        description: "使用该凭证的调用会立即被拒绝，之后可重新签发。",
        confirmText: "撤销",
        destructive: true,
      })
    )
      action.mutate(() => knowledge.revoke(a.id), {
        onSuccess: () => toast.success("凭证已撤销"),
      });
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6 p-4 md:p-8">
      <nav aria-label="面包屑" className="flex items-center gap-1 text-sm text-zinc-500">
        <Link href="/hub/knowledge" className="rounded px-1 hover:bg-zinc-100 hover:text-zinc-900">
          知识库
        </Link>
        <ChevronRight className="size-3.5" />
        <span className="px-1 text-zinc-900">应用接入</span>
      </nav>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">应用接入</h1>
          <p className="mt-1.5 max-w-2xl text-sm text-zinc-500">
            为业务系统创建应用，选择你管理的知识库，签发凭证后即可通过接口检索。
            {isAdmin && "你是管理员，可查看并停用全部应用。"}
          </p>
        </div>
        <Button size="lg" className="px-3.5" onClick={() => edit()}>
          <Plus />
          创建应用
        </Button>
      </header>

      <ol className="grid gap-3 text-sm sm:grid-cols-3">
        {[
          ["创建应用", "选择要开放检索的知识库"],
          ["签发凭证", "仅显示一次，有效期 90 天"],
          ["接口调用", "业务后端带凭证调用检索接口"],
        ].map(([title, text], i) => (
          <li key={title} className="flex gap-3 rounded-xl border border-zinc-200 bg-white p-4">
            <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">
              {i + 1}
            </span>
            <span>
              <span className="font-medium">{title}</span>
              <span className="mt-0.5 block text-xs text-zinc-500">{text}</span>
            </span>
          </li>
        ))}
      </ol>

      <ErrorMessage error={apps.error || bases.error} />

      {token && (
        <section className="space-y-3 rounded-xl border border-amber-300 bg-amber-50 p-5">
          <h2 className="flex items-center gap-2 font-semibold text-amber-900">
            <ShieldAlert className="size-4" />
            请立即复制并保存凭证，关闭后无法再次查看
          </h2>
          <div className="flex gap-2">
            <code className="min-w-0 flex-1 break-all rounded-lg bg-white px-3 py-2 font-mono text-sm ring-1 ring-amber-200">
              {token}
            </code>
            <Button
              variant="outline"
              size="lg"
              onClick={() =>
                void navigator.clipboard.writeText(token).then(() => {
                  setCopied(true);
                  toast.success("凭证已复制");
                })
              }
            >
              {copied ? <Check /> : <Copy />}
              {copied ? "已复制" : "复制"}
            </Button>
          </div>
          <Button variant="outline" onClick={() => setToken("")}>
            已保存，隐藏凭证
          </Button>
        </section>
      )}

      {apps.isPending && (
        <div role="status" aria-label="正在加载应用" className="space-y-3">
          <Skeleton className="h-24 rounded-xl" />
          <Skeleton className="h-24 rounded-xl" />
        </div>
      )}
      {apps.data?.length === 0 && (
        <div className="flex flex-col items-center rounded-xl border border-dashed border-zinc-300 bg-white px-6 py-12 text-center">
          <KeyRound className="size-8 text-zinc-300" />
          <p className="mt-3 font-medium">还没有应用</p>
          <p className="mt-1 text-sm text-zinc-500">创建应用、选择知识库，再签发凭证。</p>
        </div>
      )}
      <ul className="space-y-3">
        {apps.data?.map((a) => (
          <li key={a.id} className="rounded-xl border border-zinc-200 bg-white p-5">
            <div className="flex flex-wrap items-start gap-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <KeyRound className="size-5" />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="font-semibold">{a.name}</h2>
                  <Badge variant={a.enabled ? "success" : "muted"}>
                    {a.enabled ? "已启用" : "已停用"}
                  </Badge>
                  {isAdmin && (
                    <Badge variant="outline">创建人：{a.owner_name || "管理员（旧应用）"}</Badge>
                  )}
                </div>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {a.knowledge_base_ids.length === 0 && (
                    <span className="text-xs text-zinc-500">尚未授权知识库</span>
                  )}
                  {a.knowledge_base_ids.map((id) => (
                    <Badge key={id} variant="info">
                      {baseName.get(id) ?? "不可见或已删除的知识库"}
                    </Badge>
                  ))}
                </div>
                <p className="mt-2 text-xs text-zinc-500">
                  凭证 {a.credential_prefix ? <code>{a.credential_prefix}…</code> : "未签发"} · 到期{" "}
                  {time(a.expires_at)} · 最后使用 {time(a.last_used_at)}
                </p>
              </div>
              <div className="flex items-center gap-1.5">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={action.isPending}
                  onClick={() => void rotate(a)}
                >
                  <RefreshCw />
                  {a.credential_prefix ? "轮换凭证" : "签发凭证"}
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        className="text-zinc-500"
                        aria-label={`更多操作：${a.name}`}
                      />
                    }
                  >
                    <MoreHorizontal />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent>
                    <DropdownMenuItem onClick={() => edit(a)}>
                      <Settings2 />
                      编辑授权 / 停用
                    </DropdownMenuItem>
                    {a.credential_prefix && (
                      <>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem variant="destructive" onClick={() => void revoke(a)}>
                          <Unplug />
                          撤销凭证
                        </DropdownMenuItem>
                      </>
                    )}
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>
          </li>
        ))}
      </ul>

      <AuditLog names={appName} />

      <section className="rounded-xl border border-zinc-200 bg-white p-5">
        <h2 className="font-semibold">接口调用示例</h2>
        <pre className="mt-3 overflow-auto rounded-lg bg-zinc-950 p-4 text-xs leading-relaxed text-zinc-100">{`POST /api/v1/knowledge/search
Authorization: Bearer <应用凭证>
Content-Type: application/json

{"knowledge_base_ids":["<知识库 ID>"],"query":"报销需要哪些材料？","top_k":5}`}</pre>
        <p className="mt-3 text-sm text-zinc-500">
          凭证由业务后端保管，不要放在网页前端或 URL 中。接口只返回授权库内的片段和出处，对话与回答生成由业务系统负责。知识库 ID 可在知识库页面地址栏中找到。
        </p>
      </section>

      <Dialog open={editing !== null} onOpenChange={(open) => !open && setEditing(null)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto p-5 sm:max-w-lg">
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <DialogHeader>
              <DialogTitle>{editing === "new" ? "创建应用" : "编辑应用"}</DialogTitle>
              <DialogDescription>
                勾选即表示你确认该库全部文档可以提供给这个应用检索，与人员可见范围无关。
              </DialogDescription>
            </DialogHeader>
            <label className="block text-sm font-medium">
              应用名称
              <input
                className={fieldClass}
                required
                maxLength={120}
                value={name}
                placeholder="例如：WMS 智能客服"
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <fieldset className="space-y-2">
              <legend className="mb-2 text-sm font-medium">授权知识库</legend>
              {managed.length === 0 && fixed.length === 0 && (
                <p className="rounded-lg bg-zinc-50 p-3 text-sm text-zinc-500">
                  你还没有可管理的知识库，请先创建知识库并上传文档。
                </p>
              )}
              {managed.map((b) => (
                <label
                  key={b.id}
                  className={cn(
                    "flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2.5 text-sm transition",
                    ids.includes(b.id)
                      ? "border-primary/50 bg-primary/5"
                      : "border-zinc-200 hover:bg-zinc-50",
                  )}
                >
                  <input
                    type="checkbox"
                    className="size-4 accent-primary"
                    checked={ids.includes(b.id)}
                    onChange={(e) => toggle(b.id, e.target.checked)}
                  />
                  {b.name}
                </label>
              ))}
              {fixed.map((id) => (
                <label
                  key={id}
                  className="flex cursor-pointer items-center gap-3 rounded-lg border border-zinc-200 px-3 py-2.5 text-sm text-zinc-500"
                >
                  <input
                    type="checkbox"
                    className="size-4 accent-primary"
                    checked
                    onChange={(e) => toggle(id, e.target.checked)}
                  />
                  {baseName.get(id) ?? id}（非你管理或已删除，只能移除）
                </label>
              ))}
            </fieldset>
            <label className="flex cursor-pointer items-center gap-3 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-primary"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
              />
              启用应用（停用后凭证立即不可用）
            </label>
            <ErrorMessage error={save.error} />
            <DialogFooter className="-mx-5 -mb-5">
              <Button type="button" variant="outline" size="lg" onClick={() => setEditing(null)}>
                取消
              </Button>
              <Button type="submit" size="lg" className="px-4" disabled={save.isPending}>
                {save.isPending ? "保存中…" : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
