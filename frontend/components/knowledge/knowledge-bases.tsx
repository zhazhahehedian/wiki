"use client";

import Link from "next/link";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { knowledge, type KnowledgeBase } from "@/lib/api/knowledge";
import { Select } from "@/components/ui/select";
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

const emptyBase = {
  name: "",
  description: "",
  visibility: "private",
  department: "",
  application_shared: false,
};

export function KnowledgeBases() {
  const bases = useKnowledgeBases();
  const invalidate = useInvalidateKnowledge();
  const [editing, setEditing] = useState<string | null>(null);
  const [form, setForm] = useState(emptyBase);
  const [memberText, setMemberText] = useState("");
  const save = useMutation({
    mutationFn: () => {
      const { department, ...body } = form;
      return knowledge.saveBase(editing ?? "", {
        ...body,
        // Department and members only apply to their own visibility mode.
        ...(form.visibility === "department" ? { department } : {}),
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
      setEditing(null);
      return invalidate();
    },
  });
  const remove = useMutation({
    mutationFn: knowledge.deleteBase,
    onSuccess: invalidate,
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
            application_shared: b.application_shared,
          }
        : emptyBase,
    );
    save.reset();
    remove.reset();
  }
  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-8">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">知识库</h1>
          <p className="mt-2 text-sm text-zinc-500">
            管理文档、检查解析质量，向人员和应用提供授权检索。
          </p>
        </div>
        <button className={primaryClass} onClick={() => edit()}>
          创建知识库
        </button>
      </div>
      <div className="flex gap-5 text-sm text-blue-700">
        <Link href="/hub/knowledge/search">人员检索测试</Link>
        <Link href="/hub/knowledge/applications">应用接入</Link>
      </div>
      <ErrorMessage error={bases.error || save.error || remove.error} />
      {bases.isPending && <p role="status">正在加载知识库…</p>}
      {bases.isError && (
        <button className={secondaryClass} onClick={() => void bases.refetch()}>
          重试加载
        </button>
      )}
      {editing !== null && (
        <form
          className={`${panel} space-y-4`}
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <h2 className="font-semibold">
            {editing ? "编辑知识库" : "创建知识库"}
          </h2>
          <label className="block">
            名称
            <input
              required
              maxLength={120}
              className={fieldClass}
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </label>
          <label className="block">
            说明
            <textarea
              className={fieldClass}
              value={form.description}
              onChange={(e) =>
                setForm({ ...form, description: e.target.value })
              }
            />
          </label>
          <label className="block">
            人员可见范围
            <Select
              aria-label="人员可见范围"
              value={form.visibility}
              onValueChange={(value) =>
                setForm({
                  ...form,
                  visibility: value,
                  application_shared:
                    value === "private" ? false : form.application_shared,
                })
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
            <label className="block">
              部门名称
              <input
                required
                className={fieldClass}
                value={form.department}
                onChange={(e) =>
                  setForm({ ...form, department: e.target.value })
                }
              />
            </label>
          )}
          {form.visibility === "members" && (
            <label className="block">
              成员飞书 Open ID（逗号分隔）
              <input
                className={fieldClass}
                value={memberText}
                onChange={(e) => setMemberText(e.target.value)}
              />
            </label>
          )}
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              disabled={form.visibility === "private"}
              checked={form.application_shared}
              onChange={(e) =>
                setForm({ ...form, application_shared: e.target.checked })
              }
            />
            <span className="text-sm">
              允许管理员显式授权给应用。确认整库文档适合共享；全公司可见不会自动授权应用。
            </span>
          </label>
          <div className="flex gap-3">
            <button disabled={save.isPending} className={primaryClass}>
              {save.isPending ? "保存中…" : "保存"}
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
      {!bases.isPending && bases.data?.length === 0 && (
        <div className={panel}>
          还没有可见知识库。创建后可上传文档并检查解析结果。
        </div>
      )}
      <div className="grid gap-4 md:grid-cols-2">
        {bases.data?.map((b) => (
          <article key={b.id} className={panel}>
            <Link
              className="text-lg font-semibold text-blue-700"
              href={`/hub/knowledge/${b.id}`}
            >
              {b.name}
            </Link>
            <p className="my-3 text-sm text-zinc-600">
              {b.description || "暂无说明"}
            </p>
            <p className="text-xs text-zinc-500">
              {b.visibility} ·{" "}
              {b.application_shared ? "可授权应用" : "未开放应用共享"}
            </p>
            {b.manage && (
              <div className="mt-4 flex gap-3">
                <button className={secondaryClass} onClick={() => edit(b)}>
                  编辑权限
                </button>
                <button
                  className={secondaryClass}
                  onClick={() => {
                    if (
                      window.confirm(
                        `删除知识库“${b.name}”？所有文档会立即不可检索。`,
                      )
                    )
                      remove.mutate(b.id);
                  }}
                >
                  删除
                </button>
              </div>
            )}
          </article>
        ))}
      </div>
    </div>
  );
}
