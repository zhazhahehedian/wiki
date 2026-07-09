"use client";

import Link from "next/link";
import { ArrowLeft, Files } from "lucide-react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { chatApi } from "@/lib/api/chat";
import { cn } from "@/lib/utils";
import type { Conversation } from "@/lib/schemas";

const MODES: { value: Conversation["mode"]; label: string }[] = [
  { value: "rag", label: "RAG" },
  { value: "react", label: "Agent" },
];

export function ChatHeader({
  kbId,
  conversation,
  disabled = false,
}: {
  kbId: string;
  conversation?: Conversation | null;
  disabled?: boolean;
}) {
  const queryClient = useQueryClient();
  const modeMutation = useMutation({
    mutationFn: (mode: Conversation["mode"]) =>
      chatApi.updateConversationMode(conversation!.id, mode),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["conversations", kbId] }),
    onError: (error) => toast.error(`Failed to switch mode: ${(error as Error).message}`),
  });

  const subtitle = conversation
    ? conversation.mode === "react"
      ? "ReAct Agent"
      : "Deterministic RAG"
    : "Select or create a chat";

  return (
    <header className="flex min-h-14 items-center justify-between gap-3 border-b px-4">
      <div className="min-w-0">
        <h1 className="truncate text-base font-semibold">{conversation?.title ?? "Chat"}</h1>
        <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {conversation && (
          <div className="flex items-center rounded-md border border-border p-0.5" role="group" aria-label="Chat mode">
            {MODES.map(({ value, label }) => (
              <button
                key={value}
                type="button"
                disabled={disabled || modeMutation.isPending || conversation.mode === value}
                onClick={() => modeMutation.mutate(value)}
                className={cn(
                  "h-7 rounded px-2.5 text-xs font-medium transition-colors",
                  conversation.mode === value
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-muted disabled:opacity-50",
                )}
              >
                {label}
              </button>
            ))}
          </div>
        )}
        <Link
          href={`/kbs/${kbId}/docs`}
          className="inline-flex h-8 items-center gap-1 rounded-md border border-border bg-background px-2.5 text-sm font-medium hover:bg-muted"
        >
          <Files className="size-4" />
          Docs
        </Link>
        <Link
          href="/"
          className="inline-flex h-8 items-center gap-1 rounded-md border border-border bg-background px-2.5 text-sm font-medium hover:bg-muted"
        >
          <ArrowLeft className="size-4" />
          KBs
        </Link>
      </div>
    </header>
  );
}
