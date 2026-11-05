"use client";

import { useQuery } from "@tanstack/react-query";
import { listDownloaders, type CapabilityDetail } from "@/lib/api/registry";
import { RegistryError } from "./registry-shared";

// Owners and admins see who downloaded which version, from the audit log.
export function Downloaders({ item }: { item: CapabilityDetail }) {
  const query = useQuery({
    queryKey: ["registry", "downloads", item.slug],
    queryFn: ({ signal }) => listDownloaders(item.slug, 0, signal),
  });
  const versions = new Map(item.versions.map((v) => [v.id, v.version]));
  return (
    <section className="rounded-xl border border-zinc-200 bg-white p-5">
      <h2 className="font-semibold">下载记录</h2>
      <p className="mt-1 text-xs text-zinc-400">共 {item.download_count ?? 0} 次下载</p>
      {query.isPending ? (
        <p role="status" className="mt-4 text-sm text-zinc-400">正在读取…</p>
      ) : query.isError ? (
        <RegistryError error={query.error} retry={() => void query.refetch()} />
      ) : query.data.items.length === 0 ? (
        <p className="mt-4 text-sm text-zinc-500">暂无下载。</p>
      ) : (
        <ul className="mt-4 space-y-3 text-sm">
          {query.data.items.map((d) => (
            <li key={d.open_id} className="min-w-0">
              <p className="truncate font-medium" title={d.open_id}>{d.display_name || d.open_id}</p>
              <p className="text-xs text-zinc-500">
                {d.downloads} 次 · 最近 {new Date(d.last_downloaded_at).toLocaleString("zh-CN")}
                {versions.has(d.last_version_id) && ` · v${versions.get(d.last_version_id)}`}
              </p>
            </li>
          ))}
          {query.data.has_more && <li className="text-xs text-zinc-400">仅显示最近 50 位下载者。</li>}
        </ul>
      )}
    </section>
  );
}
