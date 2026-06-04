"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageSquare, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { chatApi } from "@/lib/api/chat";
import { cn } from "@/lib/utils";

export function ChatSidebar({
  kbId,
  selectedConversationId,
}: {
  kbId: string;
  selectedConversationId?: string;
}) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const conversations = useQuery({
    queryKey: ["conversations", kbId],
    queryFn: () => chatApi.listConversations(kbId, 50, 0),
    enabled: !!kbId,
  });
  const createConversation = useMutation({
    mutationFn: () => chatApi.createConversation(kbId),
    onSuccess: (conversation) => {
      queryClient.invalidateQueries({ queryKey: ["conversations", kbId] });
      router.push(`/kbs/${kbId}/chats/${conversation.id}`);
    },
  });

  return (
    <aside className="flex w-full shrink-0 flex-col border-b bg-muted/20 md:min-h-screen md:w-64 md:border-b-0 md:border-r">
      <div className="flex items-center justify-between gap-2 border-b p-3">
        <Link href="/" className="inline-flex min-w-0 items-center gap-2 text-sm font-semibold">
          <MessageSquare className="size-4 shrink-0" />
          <span className="truncate">KB chats</span>
        </Link>
        <Button size="sm" onClick={() => createConversation.mutate()} disabled={createConversation.isPending}>
          <Plus className="size-4" />
          New
        </Button>
      </div>

      <nav className="flex gap-2 overflow-x-auto p-3 md:flex-col md:overflow-y-auto">
        {conversations.isLoading && <p className="text-xs text-muted-foreground">Loading chats...</p>}
        {conversations.isError && (
          <p className="text-xs text-destructive">{(conversations.error as Error).message}</p>
        )}
        {conversations.data?.items.length === 0 && (
          <p className="max-w-56 text-xs leading-relaxed text-muted-foreground">Create a chat to ask this KB.</p>
        )}
        {conversations.data?.items.map((conversation) => (
          <Link
            key={conversation.id}
            href={`/kbs/${kbId}/chats/${conversation.id}`}
            className={cn(
              "min-w-44 rounded-md border bg-background px-3 py-2 text-sm hover:bg-muted md:min-w-0",
              conversation.id === selectedConversationId && "border-primary bg-muted",
            )}
          >
            <span className="block truncate font-medium">{conversation.title || "New chat"}</span>
            <span className="mt-1 block text-xs text-muted-foreground">
              {new Date(conversation.updated_at).toLocaleString()}
            </span>
          </Link>
        ))}
      </nav>
    </aside>
  );
}
