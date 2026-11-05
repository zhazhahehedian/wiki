"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { FileText, TriangleAlert } from "lucide-react";
import {
  getSkillFiles,
  type BundleFinding,
  type CapabilityVersion,
} from "@/lib/api/registry";
import { RegistryError } from "./registry-shared";

export function formatBytes(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  return `${(size / 1024 / 1024).toFixed(1)} MiB`;
}

const findingLabels: Record<BundleFinding["kind"], string> = {
  env_file: "环境变量文件",
  private_key: "疑似私钥或证书",
  credential_file: "凭证配置文件",
  vcs_directory: "Git 仓库目录",
  cloud_access_key: "疑似云账号 AccessKey",
  api_token: "疑似 API Token",
};

// Package size, digest and upload-time credential hints for one version.
export function BundleFacts({ version }: { version?: CapabilityVersion }) {
  if (!version?.has_bundle) return null;
  const findings = version.bundle_findings ?? [];
  return (
    <div className="space-y-3">
      <p className="break-all text-xs leading-6 text-zinc-500">
        {version.bundle_sha256 && version.bundle_size
          ? `程序包 ${formatBytes(version.bundle_size)} · SHA-256 ${version.bundle_sha256}`
          : "该程序包上传于校验值记录之前，未记录大小与 SHA-256。"}
      </p>
      {findings.length > 0 && (
        <div
          role="note"
          className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs leading-6 text-amber-900"
        >
          <p className="flex items-center gap-1 font-medium">
            <TriangleAlert className="size-4" />
            上传扫描发现 {findings.length} 处可能包含凭证的内容，请审核确认
          </p>
          <ul className="mt-2 space-y-1">
            {findings.map((f) => (
              <li key={`${f.path}:${f.kind}`} className="break-all">
                <code>{f.path}</code> · {findingLabels[f.kind] ?? f.kind}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

const readme = (name: string) => /(^|\/)readme(\.md|\.markdown|\.txt)?$/i.test(name);

// Read-only view of an MCP ZIP: README first, then every file with inline
// text where the server provided it. Nothing is extracted or executed.
export function McpPackageFiles({ slug, version }: { slug: string; version: string }) {
  const query = useQuery({
    queryKey: ["registry", "files", slug, version],
    queryFn: ({ signal }) => getSkillFiles(slug, version, signal),
  });
  const [selected, setSelected] = useState<string>();
  if (query.isPending)
    return <p role="status" className="text-sm text-zinc-400">正在读取程序包内容…</p>;
  if (query.isError)
    return <RegistryError error={query.error} retry={() => void query.refetch()} />;
  const files = query.data.files;
  const docs = files
    .filter((f) => readme(f.name) && f.content !== undefined)
    .sort((a, b) => a.name.split("/").length - b.name.split("/").length);
  const file = files.find((f) => f.name === selected) ?? docs[0];
  return (
    <div className="space-y-4">
      {file ? (
        <article className="min-w-0 rounded-lg border border-zinc-200 p-4">
          <p className="mb-3 break-all text-xs text-zinc-400">{file.name}</p>
          {file.content === undefined ? (
            <p className="text-xs text-zinc-500">此文件为二进制或大于 256 KiB，请下载程序包查看。</p>
          ) : /\.(md|markdown)$/i.test(file.name) ? (
            <div className="prose prose-sm max-w-none break-words">
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{file.content}</ReactMarkdown>
            </div>
          ) : (
            <pre className="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded bg-zinc-50 p-3 text-xs leading-6">
              {file.content}
            </pre>
          )}
        </article>
      ) : (
        <p className="text-sm text-zinc-500">程序包中没有 README，可在下方文件清单中查看具体文件。</p>
      )}
      <details className="rounded-lg border border-zinc-200 p-3">
        <summary className="cursor-pointer text-sm font-medium">文件清单（{files.length}）</summary>
        <ul className="mt-3 max-h-80 space-y-1 overflow-auto text-xs">
          {files.map((f) => (
            <li key={f.name}>
              <button
                type="button"
                onClick={() => setSelected(f.name)}
                aria-pressed={file?.name === f.name}
                className={`w-full break-all rounded px-2 py-1 text-left ${file?.name === f.name ? "bg-blue-50 text-blue-800" : "text-zinc-600 hover:bg-zinc-50"}`}
              >
                <FileText className="mr-1 inline size-3" />
                {f.name} <span className="text-zinc-400">· {formatBytes(f.size)}</span>
              </button>
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}
