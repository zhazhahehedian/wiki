"use client";

import type { Citation } from "@/lib/schemas";

export function CitationCard({ citation, onClick }: { citation: Citation; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="min-w-44 rounded-md border bg-background p-2 text-left text-xs hover:bg-muted"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">[{citation.id.replace(/^c/, "")}]</span>
        <span className="text-muted-foreground">{citation.score.toFixed(2)}</span>
      </div>
      <div className="mt-1 truncate text-muted-foreground">{citation.document_title}</div>
      <div className="mt-1 line-clamp-2 leading-relaxed">{citation.snippet}</div>
    </button>
  );
}
