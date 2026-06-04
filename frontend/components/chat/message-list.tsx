"use client";

import { useEffect, useRef } from "react";

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
    <div className="min-h-0 flex-1 overflow-y-auto p-4">
      {messages.length === 0 ? (
        <div className="mx-auto flex h-full max-w-md flex-col justify-center text-center">
          <h2 className="text-lg font-semibold">Ask from this KB</h2>
          <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
            Start a chat from the sidebar, then ask a question. Answers stream with citations from ready documents.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {messages.map((message) => (
            <MessageBubble key={message.id} message={message} onCitationClick={onCitationClick} />
          ))}
          <div ref={endRef} />
        </div>
      )}
    </div>
  );
}
