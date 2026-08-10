import { LoaderCircle } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import type { DocStatus } from "@/lib/schemas";

const variantMap: Record<DocStatus, { label: string; className: string; spinning?: boolean }> = {
  pending:   { label: "排队中",   className: "bg-muted text-muted-foreground" },
  parsing:   { label: "解析中",   className: "bg-primary/10 text-primary", spinning: true },
  chunking:  { label: "切片中",   className: "bg-primary/10 text-primary", spinning: true },
  embedding: { label: "向量化中", className: "bg-primary/10 text-primary", spinning: true },
  ready:     { label: "就绪",     className: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400" },
  failed:    { label: "失败",     className: "bg-destructive/10 text-destructive" },
};

export function IngestStatusBadge({ status }: { status: DocStatus }) {
  const v = variantMap[status];
  return (
    <Badge variant="secondary" className={v.className}>
      {v.spinning && <LoaderCircle className="size-3 animate-spin" />}
      {v.label}
    </Badge>
  );
}
