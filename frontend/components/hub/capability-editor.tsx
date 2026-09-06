"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Save } from "lucide-react";
import {
  getCapability,
  saveCapability,
  type CapabilityDetail,
  type CapabilityInput,
  type MCPTool,
} from "@/lib/api/registry";
import { APIError } from "@/lib/api/client";
import {
  Field,
  fieldClass,
  primaryClass,
  secondaryClass,
  RegistryError,
} from "./registry-shared";

const blank: CapabilityInput = {
  slug: "",
  type: "mcp",
  name: "",
  description: "",
  department: "",
  visibility: "org",
  allowlist: [],
  revision: 0,
  version: "1.0.0",
  changelog: "",
  mcp_endpoint: "",
  mcp_transport: "streamable-http",
  mcp_auth_scheme: "bearer",
  tools: [],
};
function initial(detail?: CapabilityDetail): CapabilityInput {
  if (!detail) return { ...blank };
  const version =
    detail.versions.find((v) => v.id === detail.draft_version_id) ??
    detail.versions.find((v) => v.id === detail.current_version_id);
  return {
    slug: detail.slug,
    type: detail.type,
    name: detail.name,
    description: detail.description,
    department: detail.department,
    visibility: detail.visibility,
    allowlist: detail.allowlist ?? [],
    revision: detail.revision,
    version: version?.published_at ? "" : (version?.version ?? "1.0.0"),
    changelog: version?.changelog ?? "",
    mcp_endpoint: version?.mcp_endpoint ?? "",
    mcp_transport: version?.mcp_transport ?? "streamable-http",
    mcp_auth_scheme: version?.mcp_auth_scheme ?? "bearer",
    tools: version?.tools ?? [],
  };
}
export function CapabilityEditorPage({ slug }: { slug?: string }) {
  const query = useQuery({
    queryKey: ["registry", "detail", slug],
    queryFn: ({ signal }) => getCapability(slug!, signal),
    enabled: !!slug,
  });
  if (slug && query.isPending) return <p role="status">正在加载草稿…</p>;
  if (slug && query.isError)
    return (
      <RegistryError error={query.error} retry={() => void query.refetch()} />
    );
  if (slug && (query.data?.status !== "draft" || !query.data?.is_owner))
    return (
      <p>
        当前状态不可编辑。
        <Link href={`/hub/registry/${slug}`} className="ml-2 text-zinc-600">
          返回详情
        </Link>
      </p>
    );
  // Existing edits retain their loaded revision on refetch, so a conflict cannot
  // silently replace user input. Explicit reload is offered below.
  return <CapabilityEditor key={slug ?? "new"} detail={query.data} />;
}
export function CapabilityEditor({ detail }: { detail?: CapabilityDetail }) {
  const router = useRouter();
  const client = useQueryClient();
  const [form, setForm] = useState(() => initial(detail));
  const [toolsText, setToolsText] = useState(() =>
    JSON.stringify(form.tools, null, 2),
  );
  const [allowlist, setAllowlist] = useState(form.allowlist.join("\n"));
  const [files, setFiles] = useState<File[]>([]);
  const [error, setError] = useState<unknown>();
  const [localError, setLocalError] = useState("");
  const [saving, setSaving] = useState(false);
  function set<K extends keyof CapabilityInput>(
    key: K,
    value: CapabilityInput[K],
  ) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }
  function reload() {
    if (window.confirm("重新加载将丢弃当前未保存的表单内容，是否继续？"))
      window.location.reload();
  }
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (saving) return;
    setError(undefined);
    setLocalError("");
    if (form.slug === "new" || form.slug === "create-skill") {
      setLocalError("new 和 create-skill 是保留标识，请使用其他名称。");
      return;
    }
    let tools: MCPTool[] = [];
    if (form.type === "mcp") {
      try {
        tools = JSON.parse(toolsText);
        if (
          !Array.isArray(tools) ||
          tools.some(
            (t) =>
              !t ||
              typeof t.name !== "string" ||
              typeof t.description !== "string" ||
              !t.inputSchema ||
              t.inputSchema.type !== "object",
          )
        )
          throw new Error();
      } catch {
        setLocalError(
          "工具清单需要 JSON 数组，每项包含 name、description 和 type 为 object 的 inputSchema。",
        );
        return;
      }
    }
    if (
      files.length > 50 ||
      files.reduce((n, f) => n + f.size, 0) > 10 * 1024 * 1024
    ) {
      setLocalError("附件最多 50 个，总大小不能超过 10 MB。");
      return;
    }
    const current = detail?.versions.find(
      (v) => v.id === detail.draft_version_id,
    );
    if (
      form.type === "skill" &&
      (!current?.has_bundle ||
        current.version !== form.version ||
        files.length > 0) &&
      !files.some((f) => f.name === "SKILL.md")
    ) {
      setLocalError(
        "请选择 SKILL.md，可同时选择附件；新版本需要上传完整文件包。",
      );
      return;
    }
    const input: CapabilityInput = {
      ...form,
      tools,
      allowlist:
        form.visibility === "allowlist"
          ? allowlist.split(/[\s,，]+/).filter(Boolean)
          : [],
    };
    if (form.type === "skill")
      Object.assign(input, {
        mcp_endpoint: "",
        mcp_transport: "",
        mcp_auth_scheme: "",
        tools: [],
      });
    setSaving(true);
    try {
      const saved = await saveCapability(
        input,
        form.type === "skill" ? files : [],
        detail?.slug,
      );
      client.setQueryData(["registry", "detail", saved.slug], saved);
      await client.invalidateQueries({ queryKey: ["registry"] });
      router.push(`/hub/registry/${saved.slug}`);
    } catch (err) {
      setError(err);
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <Link
        href={detail ? `/hub/registry/${detail.slug}` : "/hub/registry"}
        className="inline-flex items-center gap-2 text-sm text-zinc-500"
      >
        <ArrowLeft className="size-4" />
        {detail ? "返回详情" : "注册中心"}
      </Link>
      <header>
        {!detail && (
          <Link
            href="/hub/registry/create-skill"
            className="mb-3 inline-flex text-sm text-zinc-600 hover:underline"
          >
            用对话创建 Skill →
          </Link>
        )}
        <h1 className="text-2xl font-semibold">
          {detail ? "编辑能力" : "发布能力"}
        </h1>
        <p className="mt-2 text-sm text-zinc-500">
          完善信息并保存草稿，再从详情页提交审核。已上线版本在新版审核期间保持可用。
        </p>
      </header>
      <form onSubmit={(e) => void submit(e)} className="space-y-6">
        <fieldset disabled={saving} className="space-y-6 disabled:opacity-70">
          <section className="space-y-5 rounded-xl border border-zinc-200 bg-white p-6">
            <h2 className="font-semibold">基本信息</h2>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field label="名称">
                <input
                  required
                  maxLength={120}
                  className={fieldClass}
                  value={form.name}
                  onChange={(e) => set("name", e.target.value)}
                  placeholder="例如：研发知识检索"
                />
              </Field>
              <Field
                label="唯一标识"
                hint="小写字母、数字和连字符；Skill 最长 64，MCP 最长 80。不能使用 new 或 create-skill，创建后不可修改。"
              >
                <input
                  required
                  pattern="[a-z0-9]+(-[a-z0-9]+)*"
                  maxLength={form.type === "skill" ? 64 : 80}
                  disabled={!!detail}
                  className={fieldClass}
                  value={form.slug}
                  onChange={(e) => set("slug", e.target.value)}
                  placeholder="engineering-search"
                />
              </Field>
              <Field label="能力类型">
                <select
                  disabled={!!detail}
                  className={fieldClass}
                  value={form.type}
                  onChange={(e) =>
                    set("type", e.target.value as CapabilityInput["type"])
                  }
                >
                  <option value="mcp">MCP 服务</option>
                  <option value="skill">Skill</option>
                </select>
              </Field>
              <Field label="部门">
                <input
                  required={form.visibility === "department"}
                  maxLength={120}
                  className={fieldClass}
                  value={form.department}
                  onChange={(e) => set("department", e.target.value)}
                  placeholder="例如：研发平台"
                />
              </Field>
            </div>
            <Field label="能力描述">
              <textarea
                required
                maxLength={10000}
                rows={4}
                className={fieldClass}
                value={form.description}
                onChange={(e) => set("description", e.target.value)}
                placeholder="说明能力的用途、适用场景和使用方式。"
              />
            </Field>
          </section>
          <section className="space-y-5 rounded-xl border border-zinc-200 bg-white p-6">
            <h2 className="font-semibold">
              {form.type === "mcp" ? "MCP 服务规格" : "Skill 文件"}
            </h2>
            {form.type === "mcp" ? (
              <>
                <Field
                  label="Endpoint"
                  hint="HTTP / HTTPS 地址，不含凭证、查询参数或片段。平台仅记录接入信息。"
                >
                  <input
                    required
                    type="url"
                    maxLength={2048}
                    className={fieldClass}
                    value={form.mcp_endpoint}
                    onChange={(e) => set("mcp_endpoint", e.target.value)}
                    placeholder="https://mcp.example.com/mcp"
                  />
                </Field>
                <div className="grid gap-5 sm:grid-cols-2">
                  <Field label="传输协议">
                    <input
                      readOnly
                      className={fieldClass}
                      value="streamable-http"
                    />
                  </Field>
                  <Field label="鉴权方式">
                    <select
                      className={fieldClass}
                      value={form.mcp_auth_scheme}
                      onChange={(e) => set("mcp_auth_scheme", e.target.value)}
                    >
                      <option value="bearer">Bearer</option>
                      <option value="none">无需鉴权</option>
                    </select>
                  </Field>
                </div>
                <Field
                  label="工具清单 JSON"
                  hint={
                    '示例：[{"name":"search","description":"搜索内容","inputSchema":{"type":"object","properties":{"query":{"type":"string"}}}}]'
                  }
                >
                  <textarea
                    spellCheck={false}
                    rows={8}
                    maxLength={250000}
                    className={`${fieldClass} font-mono text-xs`}
                    value={toolsText}
                    onChange={(e) => setToolsText(e.target.value)}
                  />
                </Field>
              </>
            ) : (
              <>
                <Field
                  label="SKILL.md 与附件"
                  hint="同时选择 SKILL.md 和全部附件，最多 50 个文件、合计 10 MB。SKILL.md 需有 YAML 文件头：name 与标识一致、description 说明用途；提交时检查包内引用。编辑当前草稿时不选文件可保留原包，选择文件会替换整个包。"
                >
                  <input
                    type="file"
                    multiple
                    className={fieldClass}
                    onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
                  />
                  {files.length > 0 && (
                    <span className="mt-2 block text-xs text-zinc-500">
                      {files
                        .map((f) => f.webkitRelativePath || f.name)
                        .join("、")}
                    </span>
                  )}
                </Field>
                <Field
                  label="Skill 文件夹"
                  hint="也可以选择整个文件夹，保留子目录。文件夹根目录需要有 SKILL.md；此选择会替换上面的文件选择。"
                >
                  <input
                    type="file"
                    multiple
                    {...{ webkitdirectory: "", directory: "" }}
                    className={fieldClass}
                    onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
                  />
                </Field>
              </>
            )}
          </section>
          <section className="space-y-5 rounded-xl border border-zinc-200 bg-white p-6">
            <h2 className="font-semibold">版本与可见范围</h2>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field
                label="版本号"
                hint="修改当前版本号会创建新版本；历史版本不可覆盖。"
              >
                <input
                  required
                  pattern="[a-zA-Z0-9][a-zA-Z0-9._+\-]{0,63}"
                  maxLength={64}
                  className={fieldClass}
                  value={form.version}
                  onChange={(e) => set("version", e.target.value)}
                />
              </Field>
              <Field label="上线后可见范围">
                <select
                  className={fieldClass}
                  value={form.visibility}
                  onChange={(e) =>
                    set(
                      "visibility",
                      e.target.value as CapabilityInput["visibility"],
                    )
                  }
                >
                  <option value="org">全公司</option>
                  <option value="department">指定部门</option>
                  <option value="allowlist">指定用户</option>
                </select>
              </Field>
            </div>
            {form.visibility === "allowlist" && (
              <Field
                label="用户 Open ID"
                hint="每行一个飞书 open_id，最多 200 人。"
              >
                <textarea
                  required
                  rows={3}
                  className={fieldClass}
                  value={allowlist}
                  onChange={(e) => setAllowlist(e.target.value)}
                />
              </Field>
            )}
            <Field label="版本说明">
              <textarea
                rows={3}
                maxLength={10000}
                className={fieldClass}
                value={form.changelog}
                onChange={(e) => set("changelog", e.target.value)}
              />
            </Field>
            <p className="text-xs text-zinc-600">
              草稿供你维护与管理员审核，保存不会向消费者公开。
            </p>
          </section>
        </fieldset>
        {localError && (
          <p
            role="alert"
            className="rounded-xl bg-red-50 p-4 text-sm text-red-700"
          >
            {localError}
          </p>
        )}
        {!!error && <RegistryError error={error} />}
        {error instanceof APIError && error.status === 409 && (
          <button type="button" className={secondaryClass} onClick={reload}>
            重新加载最新草稿
          </button>
        )}
        <div className="flex items-center justify-end gap-3 pb-8">
          <Link
            className={secondaryClass}
            href={detail ? `/hub/registry/${detail.slug}` : "/hub/registry"}
          >
            取消
          </Link>
          <button disabled={saving} className={primaryClass}>
            <Save className="size-4" />
            {saving ? "正在保存…" : "保存草稿"}
          </button>
        </div>
      </form>
    </div>
  );
}
