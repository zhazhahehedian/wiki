"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { ArrowLeft, Download, Copy, FileText } from "lucide-react";
import {
  getCatalogCapability,
  getSkillFiles,
  capabilityBundleURL,
} from "@/lib/api/registry";
import { primaryClass, secondaryClass, RegistryError } from "./registry-shared";
import { BundleFacts, McpPackageFiles } from "./mcp-package";

export function MarketSkillFiles({
  slug,
  version,
}: {
  slug: string;
  version: string;
}) {
  const [selected, setSelected] = useState("SKILL.md");
  const [raw, setRaw] = useState(false);
  const query = useQuery({
    queryKey: ["registry", "published-files", slug, version],
    queryFn: ({ signal }) => getSkillFiles(slug, version, signal),
  });
  if (query.isPending) return <p role="status">正在读取文件…</p>;
  if (query.isError)
    return (
      <RegistryError error={query.error} retry={() => void query.refetch()} />
    );
  const file =
    query.data.files.find((item) => item.name === selected) ??
    query.data.files[0];
  const markdown = file?.name.endsWith(".md") && !raw;
  const content =
    file?.name === "SKILL.md" && markdown
      ? file.content?.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, "")
      : file?.content;
  return (
    <div className="space-y-4">
      <div role="group" aria-label="文件清单" className="flex flex-wrap gap-2">
        {query.data.files.map((item) => (
          <button
            key={item.name}
            onClick={() => setSelected(item.name)}
            aria-pressed={file?.name === item.name}
            className={`max-w-full break-all rounded-lg border px-3 py-2 text-left text-xs ${file?.name === item.name ? "border-blue-200 bg-blue-50 text-blue-800" : "border-zinc-200 text-zinc-600"}`}
          >
            <FileText className="mr-1 inline size-3" />
            {item.name}{" "}
            <span className="text-zinc-400">· {item.size} 字节</span>
          </button>
        ))}
      </div>
      {file && (
        <div className="min-w-0 rounded-xl border border-zinc-200 p-4 sm:p-6">
          <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
            <h3 className="break-all text-sm font-medium">{file.name}</h3>
            {file.content !== undefined && (
              <button
                className={secondaryClass}
                aria-pressed={raw}
                onClick={() => setRaw(!raw)}
              >
                {raw ? "阅读预览" : "查看源文件"}
              </button>
            )}
          </div>
          {content === undefined ? (
            <p className="text-sm text-zinc-500">
              此文件不支持在线预览，请下载完整 Skill 包查看。
            </p>
          ) : markdown ? (
            <div className="prose prose-sm max-w-none overflow-x-auto break-words prose-pre:overflow-x-auto">
              <ReactMarkdown
                skipHtml
                remarkPlugins={[remarkGfm]}
                components={{
                  img: ({ alt }) => (
                    <span className="text-zinc-500">
                      [图片：{alt || "请下载查看"}]
                    </span>
                  ),
                  a: ({ href, children }) =>
                    /^https?:\/\//i.test(href ?? "") ||
                    href?.startsWith("mailto:") ? (
                      <a href={href} target="_blank" rel="noopener noreferrer">
                        {children}
                      </a>
                    ) : (
                      <span title="包内引用请通过文件清单查看">{children}</span>
                    ),
                }}
              >
                {content}
              </ReactMarkdown>
            </div>
          ) : (
            <pre className="max-h-[36rem] overflow-auto whitespace-pre-wrap break-words text-xs leading-6">
              {content}
            </pre>
          )}
        </div>
      )}
    </div>
  );
}

export function MarketDetailPage({ slug }: { slug: string }) {
  const query = useQuery({
    queryKey: ["registry", "published", slug],
    queryFn: ({ signal }) => getCatalogCapability(slug, signal),
  });
  if (query.isPending) return <p role="status">正在加载市场详情…</p>;
  if (query.isError)
    return (
      <div className="space-y-4">
        <RegistryError error={query.error} retry={() => void query.refetch()} />
        <Link href="/hub/registry" className={secondaryClass}>
          返回能力市场
        </Link>
      </div>
    );
  const item = query.data;
  const version = item.versions.find((v) => v.id === item.current_version_id);
  const skill = item.type === "skill";
  const templateURL = `/hub/registry/create-skill?${new URLSearchParams({ source: slug, version: version?.id ?? "" })}`;
  return (
    <div className="space-y-6">
      <Link
        href="/hub/registry"
        className="inline-flex items-center gap-2 text-sm text-zinc-500"
      >
        <ArrowLeft className="size-4" />
        能力市场
      </Link>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="text-xs font-semibold uppercase text-blue-700">
            {item.type} · 已审核上线
          </p>
          <h1 className="mt-3 break-words text-2xl font-semibold">
            {item.name}
          </h1>
          <p className="mt-2 break-all text-sm text-zinc-500">
            {item.slug}
            {version && ` · v${version.version}`}
            {` · ${item.download_count ?? 0} 次下载`}
          </p>
        </div>
        {version?.has_bundle && (
          <div className="flex flex-wrap gap-2">
            {skill && <Link href={templateURL} className={primaryClass}>
              <Copy className="size-4" />
              基于此创建
            </Link>}
            <a
              href={capabilityBundleURL(slug, version.id)}
              className={secondaryClass}
            >
              <Download className="size-4" />
              {skill ? "下载 Skill 包" : "下载 MCP 程序包"}
            </a>
          </div>
        )}
      </header>
      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,1fr)_300px]">
        <div className="min-w-0 space-y-6">
          <section className="rounded-xl border border-zinc-200 bg-white p-5 sm:p-6">
            <h2 className="font-semibold">适用场景</h2>
            <p className="mt-4 whitespace-pre-wrap break-words text-sm leading-7 text-zinc-600">
              {item.description}
            </p>
          </section>
          <section className="min-w-0 rounded-xl border border-zinc-200 bg-white p-5 sm:p-6">
            <h2 className="mb-5 font-semibold">
              {skill ? "使用说明与文件" : "工具与接入说明"}
            </h2>
            {skill && version?.has_bundle ? (
              <MarketSkillFiles
                key={version.id}
                slug={slug}
                version={version.id}
              />
            ) : !skill ? (
              <div className="space-y-4">
                <p className="break-all text-sm">{version?.mcp_endpoint || "本版本通过程序包分发，下载后自行运行。"}</p>
                <p className="text-xs text-zinc-500">
                  {version?.mcp_transport} · {version?.mcp_auth_scheme}
                </p>
                <BundleFacts version={version} />
                {version?.has_bundle && <McpPackageFiles key={version.id} slug={slug} version={version.id} />}
                {version?.tools?.map((tool) => (
                  <details key={tool.name} className="rounded-lg border p-4">
                    <summary className="cursor-pointer text-sm font-medium">
                      {tool.name}
                    </summary>
                    <p className="mt-3 whitespace-pre-wrap text-sm">
                      {tool.description}
                    </p>
                    <pre className="mt-3 overflow-auto text-xs">
                      {JSON.stringify(tool.inputSchema, null, 2)}
                    </pre>
                  </details>
                ))}
              </div>
            ) : (
              <p className="text-sm text-zinc-500">
                当前版本没有可下载的 Skill 包。
              </p>
            )}
          </section>
        </div>
        <aside className="min-w-0 space-y-6">
          {!skill && (
            <section className="rounded-xl border border-zinc-200 bg-white p-5">
              <h2 className="font-semibold">如何使用 MCP</h2>
              {version?.has_bundle && <ol className="mt-4 list-decimal space-y-3 pl-5 text-sm leading-6 text-zinc-600">
                <li>下载当前版本的 MCP 程序包（ZIP，无需解压）。</li>
                <li>在 WorkBuddy、ChatGPT 等支持导入的客户端中选择该 ZIP 安装。</li>
                <li>按包内 README 填写自己的账号或配置；有新版本时会收到飞书通知。</li>
              </ol>}
              {version?.mcp_endpoint && <p className="mt-4 text-sm leading-6 text-zinc-600">也可使用发布者提供的 Endpoint 连接远程服务；需要凭证时联系服务提供者。</p>}
              <p className="mt-4 text-xs leading-6 text-zinc-500">平台提供程序包审核、存储与下载，不托管运行。</p>
            </section>
          )}
          {skill && (
            <section className="rounded-xl border border-zinc-200 bg-white p-5">
              <h2 className="font-semibold">如何使用</h2>
              <ol className="mt-4 list-decimal space-y-3 pl-5 text-sm leading-6 text-zinc-600">
                <li>下载并解压 Skill 包，保留整个目录及附件。</li>
                <li>
                  通过所用 AI 工具的 Skill 导入功能，或按其安装说明放入 Skill
                  目录。
                </li>
                <li>
                  阅读 SKILL.md 中的输入要求、示例和依赖，在该工具中选择或调用此
                  Skill。
                </li>
              </ol>
              <p className="mt-4 text-xs leading-6 text-zinc-500">
                需要定制？选择「基于此创建」后编辑或对话修改，再保存为自己的草稿。平台提供发现和复用，不在网页内执行
                Skill。
              </p>
            </section>
          )}
          <section className="rounded-xl border border-zinc-200 bg-white p-5">
            <h2 className="font-semibold">发布信息</h2>
            <dl className="mt-4 space-y-4 text-sm">
              <div>
                <dt className="text-xs text-zinc-400">部门</dt>
                <dd className="mt-1 break-words">
                  {item.department || "未设置"}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-400">发布者</dt>
                <dd className="mt-1 break-all">
                  {item.publisher_name || item.owner_open_id}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-400">当前版本</dt>
                <dd className="mt-1">{version?.version ?? "—"}</dd>
              </div>
              {version?.published_at && (
                <div>
                  <dt className="text-xs text-zinc-400">上线时间</dt>
                  <dd className="mt-1">
                    {new Date(version.published_at).toLocaleString("zh-CN")}
                  </dd>
                </div>
              )}
            </dl>
            {version?.changelog && (
              <p className="mt-4 whitespace-pre-wrap break-words border-t pt-4 text-xs leading-6 text-zinc-500">
                {version.changelog}
              </p>
            )}
          </section>
        </aside>
      </div>
    </div>
  );
}
