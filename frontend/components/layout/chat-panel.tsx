"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronsUpDown, Plus } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { chatApi } from "@/lib/api/chat";
import { useKbs } from "@/lib/hooks/use-kbs";
import { cn } from "@/lib/utils";

export function ChatPanel() {
  const params = useParams<{ kbId?: string; conversationId?: string }>();
  const kbId = params.kbId ?? "";
  const selectedConversationId = params.conversationId;
  const router = useRouter();
  const queryClient = useQueryClient();

  const kbs = useKbs();
  const currentKb = kbs.data?.items.find((kb) => kb.id === kbId) ?? null;

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
    onError: (error) => toast.error(`新建会话失败：${(error as Error).message}`),
  });

  return (
    <div className="flex h-full flex-col">
      <div className="border-b p-3">
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button variant="outline" className="w-full justify-between font-medium">
                <span className="truncate">{currentKb?.name ?? "选择知识库"}</span>
                <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
              </Button>
            }
          />
          <DropdownMenuContent className="w-56">
            {kbs.data?.items.map((kb) => (
              <DropdownMenuItem key={kb.id} onClick={() => router.push(`/kbs/${kb.id}/chats`)}>
                <span className="truncate">{kb.name}</span>
                {kb.id === kbId && <Check className="ml-auto size-4" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          className="mt-2 w-full"
          onClick={() => createConversation.mutate()}
          disabled={!kbId || createConversation.isPending}
        >
          <Plus className="size-4" />
          新会话
        </Button>
      </div>

      <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
        {conversations.isLoading && (
          <div className="space-y-2 p-1">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        )}
        {conversations.isError && (
          <p className="p-2 text-xs text-destructive">{(conversations.error as Error).message}</p>
        )}
        {conversations.data?.items.length === 0 && (
          <p className="p-2 text-xs leading-relaxed text-muted-foreground">还没有会话，点击「新会话」开始提问。</p>
        )}
        {conversations.data?.items.map((conversation) => (
          <Link
            key={conversation.id}
            href={`/kbs/${kbId}/chats/${conversation.id}`}
            className={cn(
              "block rounded-md border-l-2 border-transparent px-3 py-2 text-sm transition-colors hover:bg-accent",
              conversation.id === selectedConversationId && "border-primary bg-accent",
            )}
          >
            <span className="block truncate font-medium">{conversation.title || "新对话"}</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              {new Date(conversation.updated_at).toLocaleString("zh-CN")}
            </span>
          </Link>
        ))}
      </nav>
    </div>
  );
}
