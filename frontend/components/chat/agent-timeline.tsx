"use client";

import { useState } from "react";
import { Check, CircleAlert, LoaderCircle, Wrench } from "lucide-react";

import { TaskProgressIndicator } from "@/components/liveagent/task-progress-indicator";
import { cn } from "@/lib/utils";
import type { LocalToolStep } from "@/lib/hooks/use-chat-stream";

function argsSummary(args: unknown): string {
  const text = typeof args === "string" ? args : JSON.stringify(args ?? {});
  return text.length > 60 ? text.slice(0, 57) + "..." : text;
}

function StepNode({ step }: { step: LocalToolStep }) {
  return (
    <li className="relative pl-5">
      <span
        className={cn(
          "absolute left-0 top-1.5 size-2 -translate-x-[calc(50%+1px)] rounded-full",
          step.running ? "bg-primary" : step.error ? "bg-destructive" : "bg-primary/40",
        )}
      />
      {step.thought && (
        <p className="mb-1.5 text-xs italic leading-relaxed text-muted-foreground">{step.thought}</p>
      )}
      <details className="group rounded-md border border-border/60 bg-muted/40 text-xs">
        <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-1.5 [&::-webkit-details-marker]:hidden">
          {step.running ? (
            <LoaderCircle className="size-3.5 shrink-0 animate-spin text-primary" />
          ) : step.error ? (
            <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          ) : (
            <Check className="size-3.5 shrink-0 text-emerald-600 dark:text-emerald-400" />
          )}
          <Wrench className="size-3 shrink-0 text-muted-foreground" />
          <span className="font-medium">{step.name}</span>
          <span className="truncate font-mono text-muted-foreground">{argsSummary(step.arguments)}</span>
          <span className={cn("ml-auto shrink-0 text-muted-foreground", step.error && "text-destructive")}>
            {step.running ? "运行中..." : step.error ? "失败" : `${step.duration_ms ?? 0} ms`}
          </span>
        </summary>
        <div className="space-y-2 border-t border-border/60 px-2.5 py-2">
          <div>
            <p className="mb-1 font-medium text-muted-foreground">参数</p>
            <pre className="overflow-x-auto rounded bg-muted p-2 font-mono">{JSON.stringify(step.arguments ?? {}, null, 2)}</pre>
          </div>
          {step.error ? (
            <div>
              <p className="mb-1 font-medium text-destructive">错误</p>
              <pre className="overflow-x-auto rounded bg-muted p-2 font-mono text-destructive">{step.error}</pre>
            </div>
          ) : (
            step.result !== undefined && (
              <div>
                <p className="mb-1 font-medium text-muted-foreground">结果</p>
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-muted p-2 font-mono">{step.result}</pre>
              </div>
            )
          )}
        </div>
      </details>
    </li>
  );
}

export function AgentTimeline({
  steps,
  answerStarted,
}: {
  steps: LocalToolStep[];
  answerStarted: boolean;
}) {
  const [userOpen, setUserOpen] = useState<boolean | null>(null);
  if (steps.length === 0) return null;
  const open = userOpen ?? !answerStarted;

  if (!open) {
    return (
      <div className="mb-2">
        <TaskProgressIndicator
          expanded={false}
          items={steps.map((step) => ({
            id: step.id || String(step.step),
            status: step.running ? "running" : step.error ? "failed" : "completed",
          }))}
          onToggle={() => setUserOpen(true)}
        />
      </div>
    );
  }

  return (
    <div className="mb-3">
      {answerStarted && (
        <button
          type="button"
          onClick={() => setUserOpen(false)}
          className="mb-2 text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          收起过程 ▴
        </button>
      )}
      <ol className="space-y-3 border-l-2 border-primary/25 pl-3">
        {steps.map((step) => (
          <StepNode key={step.id || step.step} step={step} />
        ))}
      </ol>
    </div>
  );
}
