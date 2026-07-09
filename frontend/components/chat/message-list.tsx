"use client";

import { useEffect, useRef } from "react";
import { MessageSquarePlus } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { MessageBubble } from "@/components/chat/message-bubble";
import type { LocalChatMessage } from "@/lib/hooks/use-chat-stream";
import type { Citation } from "@/lib/schemas";

export function MessageList({
  messages,
  onCitationClick,
}: {
  messages: LocalChatMessage[];
  onCitationClick: (citation: Citation) => void;
}) {
  const endRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: "end" });
  }, [messages]);

  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
      {messages.length === 0 ? (
        <EmptyState
          icon={MessageSquarePlus}
          title="向这个知识库提问吧"
          description="从左侧选择或新建会话后输入问题，回答会流式输出并附带可点击的引用来源。"
          className="h-full py-0"
        />
      ) : (
        <div className="mx-auto max-w-3xl space-y-5">
          {messages.map((message) => (
            <MessageBubble key={message.client_key} message={message} onCitationClick={onCitationClick} />
          ))}
          <div ref={endRef} />
        </div>
      )}
    </div>
  );
}
