"use client";

import ReactMarkdown from "react-markdown";
import rehypeHighlight from "rehype-highlight";

import { CitationCard } from "@/components/chat/citation-card";
import { ToolCallTrace } from "@/components/chat/tool-call-trace";
import { cn } from "@/lib/utils";
import type { Citation } from "@/lib/schemas";
import type { LocalChatMessage } from "@/lib/hooks/use-chat-stream";

export function MessageBubble({
  message,
  onCitationClick,
}: {
  message: LocalChatMessage;
  onCitationClick: (citation: Citation) => void;
}) {
  const isUser = message.role === "user";

  return (
    <article className={cn("flex", isUser ? "justify-end" : "justify-start")}>
      <div
        className={cn(
          "max-w-[min(760px,calc(100vw-3rem))] rounded-md border p-3 text-sm",
          isUser ? "bg-primary text-primary-foreground" : "bg-background",
          message.error && "border-destructive",
        )}
      >
        {isUser ? (
          <p className="whitespace-pre-wrap leading-relaxed">{message.content}</p>
        ) : (
          <>
            <ToolCallTrace steps={message.tool_calls} />
            <div className="leading-relaxed [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-muted [&_pre]:p-3">
              <ReactMarkdown rehypePlugins={[rehypeHighlight]}>
                {message.content || (message.pending ? "Thinking..." : "")}
              </ReactMarkdown>
            </div>
          </>
        )}
        {message.error && <p className="mt-2 text-xs text-destructive">{message.error}</p>}
        {!isUser && message.citations.length > 0 && (
          <div className="mt-3 flex gap-2 overflow-x-auto pb-2">
            {message.citations.map((citation) => (
              <CitationCard key={citation.id} citation={citation} onClick={() => onCitationClick(citation)} />
            ))}
          </div>
        )}
      </div>
    </article>
  );
}
