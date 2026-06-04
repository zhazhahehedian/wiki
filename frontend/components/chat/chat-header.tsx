"use client";

import Link from "next/link";
import { ArrowLeft, Files } from "lucide-react";

import type { Conversation } from "@/lib/schemas";

export function ChatHeader({ kbId, conversation }: { kbId: string; conversation?: Conversation | null }) {
  return (
    <header className="flex min-h-14 items-center justify-between gap-3 border-b px-4">
      <div className="min-w-0">
        <h1 className="truncate text-base font-semibold">{conversation?.title ?? "Chat"}</h1>
        <p className="truncate text-xs text-muted-foreground">{conversation ? "Deterministic RAG" : "Select or create a chat"}</p>
      </div>
      <div className="flex shrink-0 items-center gap-2">
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
