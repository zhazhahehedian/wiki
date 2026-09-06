"use client";
import { useQuery } from "@tanstack/react-query";
import { getSkillFiles } from "@/lib/api/registry";
import { RegistryError } from "./registry-shared";

export function SkillFiles({
  slug,
  version,
}: {
  slug: string;
  version: string;
}) {
  const query = useQuery({
    queryKey: ["registry", "files", slug, version],
    queryFn: ({ signal }) => getSkillFiles(slug, version, signal),
  });
  if (query.isPending)
    return (
      <p role="status" className="text-sm text-zinc-400">
        正在读取 Skill 文件…
      </p>
    );
  if (query.isError)
    return (
      <RegistryError error={query.error} retry={() => void query.refetch()} />
    );
  return (
    <div className="space-y-3">
      {query.data.files.map((file) => (
        <details
          key={file.name}
          open={file.name === "SKILL.md"}
          className="min-w-0 rounded-lg border border-zinc-200 p-3"
        >
          <summary className="cursor-pointer break-all text-xs font-medium">
            {file.name}{" "}
            <span className="text-zinc-400">· {file.size} 字节</span>
          </summary>
          {file.content !== undefined ? (
            <pre className="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-words rounded bg-zinc-50 p-3 text-xs leading-6">
              {file.content}
            </pre>
          ) : (
            <p className="mt-3 text-xs text-zinc-500">
              此文件为二进制或大于 256 KiB，请下载包查看。
            </p>
          )}
        </details>
      ))}
    </div>
  );
}
