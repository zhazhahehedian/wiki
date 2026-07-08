"use client";

import { useQuery } from "@tanstack/react-query";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { chatApi } from "@/lib/api/chat";
import { cn } from "@/lib/utils";
import type { Citation } from "@/lib/schemas";

export function CitationDrawer({
  kbId,
  citation,
  onOpenChange,
}: {
  kbId: string;
  citation: Citation | null;
  onOpenChange: (citation: Citation | null) => void;
}) {
  const query = useQuery({
    queryKey: ["chunk-neighbors", kbId, citation?.chunk_id],
    queryFn: () => chatApi.getNeighbors(kbId, citation!.chunk_id, 1),
    enabled: !!citation,
  });

  return (
    <Dialog open={!!citation} onOpenChange={(open) => !open && onOpenChange(null)}>
      <DialogContent className="left-auto right-0 top-0 h-screen max-h-screen max-w-lg translate-x-0 translate-y-0 overflow-hidden rounded-none border-l sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{citation?.document_title ?? "Citation"}</DialogTitle>
          <DialogDescription>{citation ? `Chunk #${citation.seq}` : ""}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 overflow-y-auto pr-1">
          {query.isLoading && <p className="text-sm text-muted-foreground">Loading chunks...</p>}
          {query.isError && <p className="text-sm text-destructive">{(query.error as Error).message}</p>}
          {query.data && query.data.chunks.length === 0 && (
            <p className="text-sm text-muted-foreground">No neighboring chunks found.</p>
          )}
          {query.data && query.data.chunks.length > 0 && (
            <div className="space-y-3">
              {query.data.chunks.map((chunk) => (
                <article
                  key={chunk.id}
                  className={cn("rounded-md border p-3 text-sm", chunk.is_primary && "border-primary bg-muted")}
                >
                  <div className="mb-2 text-xs text-muted-foreground">#{chunk.seq}</div>
                  <pre className="whitespace-pre-wrap font-sans leading-relaxed">{chunk.content}</pre>
                </article>
              ))}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
