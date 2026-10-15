"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { getSkillTemplate } from "@/lib/api/skill-builder";
import {
  RegistryError,
  secondaryClass,
} from "@/components/hub/registry-shared";
import { SkillBuilderPage } from "./skill-builder-page";

export function SkillTemplateEntry({
  slug,
  version,
}: {
  slug: string;
  version: string;
}) {
  const query = useQuery({
    queryKey: ["skill-template", slug, version],
    queryFn: ({ signal }) => getSkillTemplate(slug, version, signal),
    gcTime: 0,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  if (query.isPending) return <p role="status">正在读取已发布的 Skill 模板…</p>;
  if (query.isError)
    return (
      <div className="space-y-4">
        <RegistryError error={query.error} retry={() => void query.refetch()} />
        <p className="text-sm text-zinc-500">
          尚未创建草稿。若来源已更新、下线或不可访问，请返回市场重新选择。
        </p>
        <Link
          className={secondaryClass}
          href={`/hub/registry/${encodeURIComponent(slug)}/market`}
        >
          返回市场详情
        </Link>
      </div>
    );
  // Initialize once. Background requests must never overwrite an edited copy.
  return <SkillBuilderPage key={`${slug}:${version}`} template={query.data} />;
}
