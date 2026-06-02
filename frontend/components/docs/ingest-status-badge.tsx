import { Badge } from "@/components/ui/badge";
import type { DocStatus } from "@/lib/schemas";

const variantMap: Record<DocStatus, { label: string; className: string }> = {
  pending:   { label: "排队中",   className: "bg-gray-200 text-gray-800" },
  parsing:   { label: "解析中",   className: "bg-blue-100 text-blue-800" },
  chunking:  { label: "切片中",   className: "bg-blue-100 text-blue-800" },
  embedding: { label: "向量化中", className: "bg-blue-100 text-blue-800" },
  ready:     { label: "就绪",     className: "bg-green-100 text-green-800" },
  failed:    { label: "失败",     className: "bg-red-100 text-red-800" },
};

export function IngestStatusBadge({ status }: { status: DocStatus }) {
  const v = variantMap[status];
  return (
    <Badge variant="secondary" className={v.className}>
      {v.label}
    </Badge>
  );
}
