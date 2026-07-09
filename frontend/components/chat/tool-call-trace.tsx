"use client";

import { CircleAlert, LoaderCircle, Wrench } from "lucide-react";

import { cn } from "@/lib/utils";
import type { LocalToolStep } from "@/lib/hooks/use-chat-stream";

function argsSummary(args: unknown): string {
  const text = typeof args === "string" ? args : JSON.stringify(args ?? {});
  return text.length > 60 ? text.slice(0, 57) + "..." : text;
}

function StepRow({ step }: { step: LocalToolStep }) {
  return (
    <details className="group rounded-md border bg-muted/40 text-xs">
      <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-1.5 [&::-webkit-details-marker]:hidden">
        {step.running ? (
          <LoaderCircle className="size-3.5 shrink-0 animate-spin text-muted-foreground" />
        ) : step.error ? (
          <CircleAlert className="size-3.5 shrink-0 text-destructive" />
        ) : (
          <Wrench className="size-3.5 shrink-0 text-muted-foreground" />
        )}
        <span className="font-medium">{step.name}</span>
        <span className="truncate text-muted-foreground">{argsSummary(step.arguments)}</span>
        <span className={cn("ml-auto shrink-0 text-muted-foreground", step.error && "text-destructive")}>
          {step.running ? "running..." : step.error ? "failed" : `${step.duration_ms ?? 0} ms`}
        </span>
      </summary>
      <div className="space-y-2 border-t px-2.5 py-2">
        {step.thought && <p className="whitespace-pre-wrap text-muted-foreground">{step.thought}</p>}
        <div>
          <p className="mb-1 font-medium text-muted-foreground">arguments</p>
          <pre className="overflow-x-auto rounded bg-muted p-2">{JSON.stringify(step.arguments, null, 2)}</pre>
        </div>
        {step.error ? (
          <div>
            <p className="mb-1 font-medium text-destructive">error</p>
            <pre className="overflow-x-auto rounded bg-muted p-2 text-destructive">{step.error}</pre>
          </div>
        ) : (
          step.result !== undefined && (
            <div>
              <p className="mb-1 font-medium text-muted-foreground">result</p>
              <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-muted p-2">{step.result}</pre>
            </div>
          )
        )}
      </div>
    </details>
  );
}

export function ToolCallTrace({ steps }: { steps: LocalToolStep[] }) {
  if (steps.length === 0) return null;
  return (
    <div className="mb-2 space-y-1">
      {steps.map((step) => (
        <StepRow key={step.id || step.step} step={step} />
      ))}
    </div>
  );
}
