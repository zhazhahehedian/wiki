"use client";

import { CheckCircle2, Circle, CircleAlert, LoaderCircle } from "lucide-react";

export type TaskProgressItem = {
  id: string;
  status: "pending" | "running" | "completed" | "failed";
};

// Adapted from LiveAgent's TaskProgressIndicator to the existing tool-call model.
export function TaskProgressIndicator({
  expanded,
  items,
  onToggle,
}: {
  expanded: boolean;
  items: TaskProgressItem[];
  onToggle: () => void;
}) {
  const completed = items.filter((item) => item.status === "completed").length;
  const failed = items.some((item) => item.status === "failed");
  const running = items.some((item) => item.status === "running");
  const state = failed ? "失败" : running ? "运行中" : completed === items.length ? "已完成" : "等待中";

  return (
    <button
      type="button"
      aria-expanded={expanded}
      aria-label={`调用了 ${items.length} 个工具 · 已完成 ${completed}/${items.length} · ${state}`}
      onClick={onToggle}
      className="liveagent-task-progress max-w-[calc(100vw-2rem)]"
    >
      <span
        role="progressbar"
        aria-label={`已完成 ${completed}/${items.length}`}
        aria-valuemin={0}
        aria-valuemax={items.length}
        aria-valuenow={completed}
        className="inline-flex size-4 shrink-0 items-center justify-center"
      >
        {failed ? (
          <CircleAlert className="size-4 text-destructive" />
        ) : running ? (
          <LoaderCircle className="size-4 animate-spin text-[hsl(var(--liveagent-tool-accent))] motion-reduce:animate-none" />
        ) : completed === items.length ? (
          <CheckCircle2 className="size-4 text-[hsl(var(--liveagent-chat-success))]" />
        ) : (
          <Circle className="size-4 text-muted-foreground/65" />
        )}
      </span>
      <span className="truncate font-medium">调用了 {items.length} 个工具</span>
      <span aria-hidden="true" className="text-muted-foreground/45">·</span>
      <span className="shrink-0 text-muted-foreground tabular-nums">{completed}/{items.length}</span>
      <span className="shrink-0 text-muted-foreground">{state}</span>
    </button>
  );
}
