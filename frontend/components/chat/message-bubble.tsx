"use client";

import { Sparkles } from "lucide-react";

import { AgentTimeline } from "@/components/chat/agent-timeline";
import { CitationChip } from "@/components/chat/citation-chip";
import { MarkdownContent } from "@/components/chat/markdown-content";
import { StreamingCursor } from "@/components/chat/streaming-cursor";
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

  if (isUser) {
    return (
      <article className="flex justify-end">
        <div className="max-w-[min(560px,85%)] rounded-2xl rounded-br-sm bg-primary px-4 py-2.5 text-sm text-primary-foreground">
          <p className="whitespace-pre-wrap leading-relaxed">{message.content}</p>
        </div>
      </article>
    );
  }

  // 时间线在整个流式过程中保持展开，done 事件清掉 pending 后才自动折叠
  //（spec 验收项 6「回答完成 → 时间线自动折叠」；历史回放 pending 为 undefined → 默认折叠）。
  const answerStarted = !message.pending;
  const showCursorOnly = !!message.pending && message.content.trim().length === 0;

  return (
    <article className="flex justify-start">
      <div className="flex w-full gap-3">
        <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Sparkles className="size-4" />
        </div>
        <div className="min-w-0 flex-1 pt-0.5 text-sm">
          {message.tool_calls.length > 0 && (
            <AgentTimeline steps={message.tool_calls} answerStarted={answerStarted} />
          )}
          {showCursorOnly ? (
            <StreamingCursor />
          ) : (
            message.content && (
              <div className="leading-relaxed">
                <MarkdownContent content={message.content} />
                {message.pending && <StreamingCursor />}
              </div>
            )
          )}
          {message.error && (
            <p className="mt-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {message.error}
            </p>
          )}
          {message.citations.length > 0 && (
            <div className="mt-3 border-t border-border/60 pt-2">
              <span className="mr-2 text-xs text-muted-foreground">来源</span>
              <span className="inline-flex flex-wrap gap-1.5 align-middle">
                {message.citations.map((citation) => (
                  <CitationChip key={citation.id} citation={citation} onClick={() => onCitationClick(citation)} />
                ))}
              </span>
            </div>
          )}
        </div>
      </div>
    </article>
  );
}
