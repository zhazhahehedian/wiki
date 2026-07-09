"use client";

import { use, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { ChatHeader } from "@/components/chat/chat-header";
import { ChatInput } from "@/components/chat/chat-input";
import { ChatSidebar } from "@/components/chat/chat-sidebar";
import { CitationDrawer } from "@/components/chat/citation-drawer";
import { MessageList } from "@/components/chat/message-list";
import { chatApi } from "@/lib/api/chat";
import { useChatStream } from "@/lib/hooks/use-chat-stream";
import type { ChatMessage, Citation } from "@/lib/schemas";

const EMPTY_MESSAGES: ChatMessage[] = [];

export default function ConversationPage({
  params,
}: {
  params: Promise<{ kbId: string; conversationId: string }>;
}) {
  const { kbId, conversationId } = use(params);
  const [selectedCitation, setSelectedCitation] = useState<Citation | null>(null);
  const conversations = useQuery({
    queryKey: ["conversations", kbId],
    queryFn: () => chatApi.listConversations(kbId, 50, 0),
    enabled: !!kbId,
  });
  const messagesQuery = useQuery({
    queryKey: ["messages", conversationId],
    queryFn: () => chatApi.listMessages(conversationId, 100, 0),
    enabled: !!conversationId,
  });
  const conversation = useMemo(
    () => conversations.data?.items.find((item) => item.id === conversationId) ?? null,
    [conversationId, conversations.data?.items],
  );
  const chat = useChatStream(messagesQuery.data?.items ?? EMPTY_MESSAGES);

  return (
    <main className="min-h-screen bg-background">
      <div className="mx-auto flex min-h-screen max-w-7xl flex-col md:flex-row">
        <ChatSidebar kbId={kbId} selectedConversationId={conversationId} />
        <section className="flex min-h-[70vh] min-w-0 flex-1 flex-col border-x">
          <ChatHeader kbId={kbId} conversation={conversation} disabled={chat.isStreaming} />
          {messagesQuery.isError && (
            <p className="border-b px-4 py-2 text-sm text-destructive">{(messagesQuery.error as Error).message}</p>
          )}
          {chat.error && <p className="border-b px-4 py-2 text-sm text-destructive">{chat.error}</p>}
          <MessageList messages={chat.messages} onCitationClick={setSelectedCitation} />
          <ChatInput
            disabled={chat.isStreaming || messagesQuery.isLoading}
            onSend={(content) => chat.send(conversationId, content)}
            onStop={chat.stop}
          />
        </section>
        <CitationDrawer kbId={kbId} citation={selectedCitation} onOpenChange={setSelectedCitation} />
      </div>
    </main>
  );
}
