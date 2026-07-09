"use client";

import type { Citation } from "@/lib/schemas";

export function CitationChip({ citation, onClick }: { citation: Citation; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={citation.snippet}
      className="inline-flex max-w-60 items-center gap-1.5 rounded-md border border-primary/30 bg-primary/5 px-2 py-1 text-xs text-primary transition-colors hover:bg-primary/10"
    >
      <span className="font-medium">[{citation.id.replace(/^c/, "")}]</span>
      <span className="truncate">{citation.document_title}</span>
      <span className="shrink-0 text-primary/70">{citation.score.toFixed(2)}</span>
    </button>
  );
}
