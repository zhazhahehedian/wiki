"use client";

import {
  Children,
  cloneElement,
  isValidElement,
  useId,
  type ReactNode,
  type ReactElement,
} from "react";
import { APIError } from "@/lib/api/client";

export const fieldClass =
  "mt-2 w-full rounded-lg border border-zinc-200 bg-white px-3 py-2.5 text-sm outline-none focus:border-zinc-400 focus:ring-2 focus:ring-blue-100 disabled:bg-zinc-50 disabled:text-zinc-500";
export const primaryClass =
  "inline-flex items-center justify-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50";
export const secondaryClass =
  "inline-flex items-center justify-center gap-2 rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm hover:bg-zinc-50 disabled:opacity-40";
export const statusLabels = {
  draft: "草稿",
  in_review: "审核中",
  published: "已上线",
  offline: "已下线",
};
export const visibilityLabels = {
  org: "全公司",
  department: "指定部门",
  allowlist: "指定用户",
};
export function StatusBadge({ status }: { status: keyof typeof statusLabels }) {
  return (
    <span
      className={`rounded-md px-2 py-1 text-xs font-medium ${status === "published" ? "bg-emerald-50 text-emerald-700" : status === "in_review" ? "bg-amber-50 text-amber-800" : "bg-zinc-100 text-zinc-600"}`}
    >
      {statusLabels[status]}
    </span>
  );
}
export function RegistryError({
  error,
  retry,
}: {
  error: unknown;
  retry?: () => void;
}) {
  return (
    <div
      role="alert"
      className="rounded-xl border border-red-100 bg-red-50 p-4 text-sm text-red-700"
    >
      {error instanceof APIError ? error.message : "请求失败，请稍后重试。"}
      {retry && (
        <button onClick={retry} className="ml-3 underline">
          重新加载
        </button>
      )}
    </div>
  );
}
export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  const id = useId();
  return (
    <div className="block text-sm font-medium text-zinc-700">
      <label htmlFor={id}>{label}</label>
      {Children.map(children, (child) => {
        if (
          !isValidElement(child) ||
          !["input", "textarea", "select"].includes(String(child.type))
        )
          return child;
        return cloneElement(
          child as ReactElement<{ id?: string; "aria-describedby"?: string }>,
          { id, "aria-describedby": hint ? `${id}-hint` : undefined },
        );
      })}
      {hint && (
        <p
          id={`${id}-hint`}
          className="mt-2 text-xs font-normal leading-5 text-zinc-400"
        >
          {hint}
        </p>
      )}
    </div>
  );
}
