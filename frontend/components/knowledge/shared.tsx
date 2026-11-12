"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Building2,
  CircleAlert,
  CircleCheck,
  CircleX,
  Clock,
  FileText,
  FileType,
  Loader2,
  Lock,
  Network,
  Users,
  type LucideIcon,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { knowledge } from "@/lib/api/knowledge";
import { cn } from "@/lib/utils";

export const panel = "rounded-xl border border-zinc-200 bg-white p-5";

export const documentStatusLabels: Record<string, string> = {
  pending: "等待索引",
  processing: "正在索引",
  ready: "可检索",
  failed: "处理失败",
  legacy_requires_reindex: "旧资料：请上传新版本完成质量核验",
};

const statusStyles: Record<
  string,
  { variant: "success" | "muted" | "info" | "destructive" | "warning"; icon: LucideIcon; spin?: boolean }
> = {
  ready: { variant: "success", icon: CircleCheck },
  pending: { variant: "muted", icon: Clock },
  processing: { variant: "info", icon: Loader2, spin: true },
  failed: { variant: "destructive", icon: CircleX },
  legacy_requires_reindex: { variant: "warning", icon: CircleAlert },
};

export function StatusBadge({ status }: { status: string }) {
  const style = statusStyles[status] ?? { variant: "muted", icon: Clock };
  const Icon = style.icon;
  return (
    <Badge variant={style.variant} className="whitespace-normal">
      <Icon className={cn(style.spin && "animate-spin")} />
      {documentStatusLabels[status] ?? status}
    </Badge>
  );
}

const visibilityStyles: Record<string, { label: string; icon: LucideIcon }> = {
  private: { label: "仅 Owner / Admin", icon: Lock },
  company: { label: "全公司", icon: Building2 },
  department: { label: "指定部门", icon: Network },
  members: { label: "指定成员", icon: Users },
};

export function VisibilityBadge({
  visibility,
  department,
}: {
  visibility: string;
  department?: string;
}) {
  const style = visibilityStyles[visibility] ?? {
    label: visibility,
    icon: Lock,
  };
  const Icon = style.icon;
  return (
    <Badge variant="outline">
      <Icon />
      {visibility === "department" && department ? department : style.label}
    </Badge>
  );
}

export function FileIcon({ title, className }: { title: string; className?: string }) {
  const ext = title.split(".").pop()?.toLowerCase() ?? "";
  const tone =
    ext === "pdf"
      ? "bg-red-50 text-red-600"
      : ext === "docx"
        ? "bg-blue-50 text-blue-600"
        : ext === "md" || ext === "markdown"
          ? "bg-violet-50 text-violet-600"
          : "bg-zinc-100 text-zinc-600";
  const Icon = ext === "pdf" || ext === "docx" ? FileType : FileText;
  return (
    <span
      className={cn(
        "flex size-9 shrink-0 items-center justify-center rounded-lg",
        tone,
        className,
      )}
    >
      <Icon className="size-4.5" />
    </span>
  );
}

export function errorText(error: unknown) {
  return error instanceof Error ? error.message : "操作失败，请重试";
}

export function ErrorMessage({ error }: { error: unknown }) {
  return error ? (
    <p
      role="alert"
      className="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2.5 text-sm text-red-700"
    >
      <CircleAlert className="mt-0.5 size-4 shrink-0" />
      {errorText(error)}
    </p>
  ) : null;
}

export function useKnowledgeBases() {
  return useQuery({
    queryKey: ["knowledge", "bases"],
    queryFn: ({ signal }) => knowledge.bases(signal),
  });
}

export function useInvalidateKnowledge() {
  const client = useQueryClient();
  return () => client.invalidateQueries({ queryKey: ["knowledge"] });
}
