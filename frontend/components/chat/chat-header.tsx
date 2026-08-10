"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { SidebarTrigger } from "@/components/ui/sidebar";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { chatApi } from "@/lib/api/chat";
import type { Conversation } from "@/lib/schemas";

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
    onError: (error) => toast.error(`切换模式失败：${(error as Error).message}`),
  });

  const subtitle = conversation
    ? conversation.mode === "react"
      ? "ReAct Agent"
      : "确定性 RAG"
    : "选择或新建会话";

  return (
    <header className="flex min-h-14 items-center justify-between gap-3 border-b px-4">
      <div className="flex min-w-0 items-center gap-2">
        <SidebarTrigger className="md:hidden" />
        <div className="min-w-0">
          <h1 className="truncate text-base font-semibold">{conversation?.title || "新对话"}</h1>
          <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
        </div>
      </div>
      {conversation && (
        <Tabs
          value={conversation.mode}
          onValueChange={(value) => {
            if (value !== conversation.mode && !disabled && !modeMutation.isPending) {
              modeMutation.mutate(value as Conversation["mode"]);
            }
          }}
        >
          <TabsList>
            <TabsTrigger value="rag" disabled={disabled || modeMutation.isPending}>
              RAG
            </TabsTrigger>
            <TabsTrigger value="react" disabled={disabled || modeMutation.isPending}>
              Agent
            </TabsTrigger>
          </TabsList>
        </Tabs>
      )}
    </header>
  );
}
