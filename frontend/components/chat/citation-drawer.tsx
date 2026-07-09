"use client";

import { useQuery } from "@tanstack/react-query";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
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
    <Sheet open={!!citation} onOpenChange={(open) => !open && onOpenChange(null)}>
      <SheetContent side="right" className="flex w-full flex-col sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{citation?.document_title ?? "引用"}</SheetTitle>
          <SheetDescription>{citation ? `切片 #${citation.seq} 及相邻上下文` : ""}</SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
          {query.isLoading && (
            <div className="space-y-3">
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
            </div>
          )}
          {query.isError && <p className="text-sm text-destructive">{(query.error as Error).message}</p>}
          {query.data && query.data.chunks.length === 0 && (
            <p className="text-sm text-muted-foreground">没有相邻切片。</p>
          )}
          {query.data && query.data.chunks.length > 0 && (
            <div className="space-y-3">
              {query.data.chunks.map((chunk) => (
                <article
                  key={chunk.id}
                  className={cn(
                    "rounded-lg border bg-card p-3 text-sm shadow-sm",
                    chunk.is_primary && "border-primary bg-primary/5",
                  )}
                >
                  <div className="mb-2 text-xs text-muted-foreground">#{chunk.seq}</div>
                  <pre className="whitespace-pre-wrap font-sans leading-relaxed">{chunk.content}</pre>
                </article>
              ))}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
