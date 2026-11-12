"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  ArrowRight,
  BookOpen,
  KeyRound,
  MoreHorizontal,
  Plus,
  RotateCw,
  Search,
  Settings2,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { knowledge, type KnowledgeBase } from "@/lib/api/knowledge";
import { Button, buttonVariants } from "@/components/ui/button";
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
import { Select } from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { DirectoryPicker } from "@/components/hub/directory-picker";
import { fieldClass } from "@/components/hub/registry-shared";
import {
  ErrorMessage,
  errorText,
  useInvalidateKnowledge,
  useKnowledgeBases,
  VisibilityBadge,
} from "./shared";

const emptyBase = {
  name: "",
  description: "",
  visibility: "private",
  department: "",
  department_id: "",
};

export function KnowledgeBases() {
  const bases = useKnowledgeBases();
  const invalidate = useInvalidateKnowledge();
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState(emptyBase);
  const [memberText, setMemberText] = useState("");
  const save = useMutation({
    mutationFn: () => {
      const { department, department_id, ...body } = form;
      return knowledge.saveBase(editing ?? "", {
        ...body,
        // Department and members only apply to their own visibility mode.
        ...(form.visibility === "department"
          ? { department, department_id }
          : {}),
        ...(form.visibility === "members"
          ? {
              members: memberText
                .split(",")
                .map((s) => s.trim())
                .filter(Boolean),
            }
          : {}),
      });
    },
    onSuccess: () => {
      toast.success(editing ? "知识库设置已保存" : "知识库已创建");
      setEditing(null);
      return invalidate();
    },
  });
  const remove = useMutation({
    mutationFn: knowledge.deleteBase,
    onSuccess: () => {
      toast.success("知识库已删除");
      return invalidate();
    },
    onError: (e) => toast.error(`删除失败：${errorText(e)}`),
  });
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
            department_id: b.department_id ?? "",
          }
        : emptyBase,
    );
    save.reset();
  }
  async function removeBase(b: KnowledgeBase) {
    if (
      await confirm({
        title: `删除知识库“${b.name}”？`,
        description:
          "库内所有文档会立即不可检索，接入了该库的应用也会同时失去访问。",
        confirmText: "删除",
        destructive: true,
      })
    )
      remove.mutate(b.id);
  }
  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">知识库</h1>
          <p className="mt-1.5 text-sm text-zinc-500">
            管理文档、检查解析质量，向人员和应用提供授权检索。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Link
            href="/hub/knowledge/search"
            className={buttonVariants({ variant: "outline", size: "lg" })}
          >
            <Search />
            检索测试
          </Link>
          <Link
            href="/hub/knowledge/applications"
            className={buttonVariants({ variant: "outline", size: "lg" })}
          >
            <KeyRound />
            应用接入
          </Link>
          <Button size="lg" className="px-3.5" onClick={() => edit()}>
            <Plus />
            创建知识库
          </Button>
        </div>
      </header>

      {bases.isError && (
        <div className="flex flex-wrap items-center gap-3">
          <ErrorMessage error={bases.error} />
          <Button variant="outline" onClick={() => void bases.refetch()}>
            <RotateCw />
            重试加载
          </Button>
        </div>
      )}

      {bases.isPending && (
        <div
          role="status"
          aria-label="正在加载知识库"
          className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
        >
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-40 rounded-xl" />
          ))}
        </div>
      )}

      {bases.data?.length === 0 && (
        <div className="flex flex-col items-center rounded-xl border border-dashed border-zinc-300 bg-white px-6 py-14 text-center">
          <span className="flex size-12 items-center justify-center rounded-full bg-primary/10 text-primary">
            <BookOpen className="size-6" />
          </span>
          <h2 className="mt-4 font-semibold">还没有可见的知识库</h2>
          <p className="mt-1.5 max-w-sm text-sm text-zinc-500">
            创建后可上传文档、检查解析结果，并授权给人员或应用检索。
          </p>
          <Button size="lg" className="mt-5 px-3.5" onClick={() => edit()}>
            <Plus />
            新建第一个知识库
          </Button>
        </div>
      )}

      {!!bases.data?.length && (
        <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {bases.data.map((b) => (
            <li
              key={b.id}
              className="group relative flex min-h-40 flex-col rounded-xl border border-zinc-200 bg-white p-5 transition hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-md has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-primary/30"
            >
              <div className="flex items-start gap-3">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <BookOpen className="size-5" />
                </span>
                <div className="min-w-0 flex-1">
                  <h2 className="truncate font-semibold text-zinc-900 group-hover:text-primary">
                    {/* Stretched link: the whole card opens the knowledge base. */}
                    <Link
                      href={`/hub/knowledge/${b.id}`}
                      className="outline-none after:absolute after:inset-0 after:rounded-xl"
                    >
                      {b.name}
                    </Link>
                  </h2>
                  <p className="mt-1 line-clamp-2 text-sm text-zinc-500">
                    {b.description || "暂无说明"}
                  </p>
                </div>
                {b.manage && (
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          className="relative z-10 -mt-1 -mr-2 text-zinc-500"
                          aria-label={`管理知识库 ${b.name}`}
                        />
                      }
                    >
                      <MoreHorizontal />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent>
                      <DropdownMenuItem onClick={() => edit(b)}>
                        <Settings2 />
                        编辑设置与权限
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => void removeBase(b)}
                      >
                        <Trash2 />
                        删除知识库
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </div>
              <div className="mt-auto flex items-center gap-2 pt-4">
                <div className="flex min-w-0 flex-1 flex-wrap gap-1.5">
                  <VisibilityBadge
                    visibility={b.visibility}
                    department={b.department}
                  />
                </div>
                <ArrowRight className="size-4 shrink-0 text-zinc-300 transition group-hover:translate-x-0.5 group-hover:text-primary" />
              </div>
            </li>
          ))}
        </ul>
      )}

      <Dialog
        open={editing !== null}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      >
        <DialogContent className="max-h-[90vh] overflow-y-auto p-5 sm:max-w-lg">
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <DialogHeader>
              <DialogTitle>{editing ? "编辑知识库" : "创建知识库"}</DialogTitle>
              <DialogDescription>
                可见范围决定哪些同事能浏览和检索。要接入业务系统，请到「应用接入」创建应用并勾选本库。
              </DialogDescription>
            </DialogHeader>
            <label className="block text-sm font-medium">
              名称
              <input
                required
                maxLength={120}
                className={fieldClass}
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </label>
            <label className="block text-sm font-medium">
              说明
              <textarea
                className={fieldClass}
                rows={3}
                value={form.description}
                onChange={(e) =>
                  setForm({ ...form, description: e.target.value })
                }
              />
            </label>
            <label className="block text-sm font-medium">
              人员可见范围
              <Select
                aria-label="人员可见范围"
                value={form.visibility}
                onValueChange={(value) =>
                  setForm({ ...form, visibility: value })
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
              <div className="space-y-2 text-sm">
                <p className="font-medium">可见部门（含所有下级部门）</p>
                <DirectoryPicker
                  kind="department"
                  value={form.department_id}
                  label={form.department}
                  onChange={(id, name) =>
                    setForm({ ...form, department_id: id, department: name })
                  }
                />
              </div>
            )}
            {form.visibility === "members" && (
              <div className="space-y-2 text-sm">
                <p className="font-medium">可见成员</p>
                <DirectoryPicker
                  kind="users"
                  max={100}
                  value={memberText.split(",").filter(Boolean)}
                  onChange={(ids) => setMemberText(ids.join(","))}
                />
              </div>
            )}
            <ErrorMessage error={save.error} />
            <DialogFooter className="-mx-5 -mb-5">
              <Button
                type="button"
                variant="outline"
                size="lg"
                onClick={() => setEditing(null)}
              >
                取消
              </Button>
              <Button
                type="submit"
                size="lg"
                className="px-4"
                disabled={save.isPending}
              >
                {save.isPending ? "保存中…" : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
