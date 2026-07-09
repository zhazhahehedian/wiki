"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { streamConversationMessage, type ChatStreamEvent } from "@/lib/api/chat";
import type { ChatMessage, Citation, ToolCallStep } from "@/lib/schemas";

export interface LocalToolStep extends ToolCallStep {
  running?: boolean;
}

export interface LocalChatMessage extends Omit<ChatMessage, "id" | "created_at" | "tool_calls"> {
  id: string;
  created_at: string;
  tool_calls: LocalToolStep[];
  pending?: boolean;
  error?: string;
}

export interface ChatStreamState {
  messages: LocalChatMessage[];
  isStreaming: boolean;
  error: string | null;
}

export function useChatStream(initialMessages: ChatMessage[] = []) {
  const [state, setState] = useState<ChatStreamState>({
    messages: initialMessages,
    isStreaming: false,
    error: null,
  });
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    setState((current) => ({ ...current, messages: initialMessages }));
  }, [initialMessages]);

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setState((current) => ({ ...current, isStreaming: false }));
  }, []);

  useEffect(() => stop, [stop]);

  const send = useCallback(
    async (conversationId: string, content: string) => {
      const trimmed = content.trim();
      if (!trimmed || state.isStreaming) return;

      const controller = new AbortController();
      abortRef.current = controller;
      const now = new Date().toISOString();
      const timestamp = Date.now();
      const draftAssistantId = `draft-assistant-${timestamp}`;

      setState((current) => ({
        messages: [
          ...current.messages,
          localMessage(`draft-user-${timestamp}`, conversationId, "user", trimmed, now),
          { ...localMessage(draftAssistantId, conversationId, "assistant", "", now), pending: true },
        ],
        isStreaming: true,
        error: null,
      }));

      try {
        await streamConversationMessage(conversationId, trimmed, {
          signal: controller.signal,
          onEvent: (event) => {
            setState((current) => applyStreamEvent(current, draftAssistantId, event));
          },
        });
      } catch (error) {
        if ((error as Error).name !== "AbortError") {
          setState((current) => ({ ...current, isStreaming: false, error: (error as Error).message }));
        }
      } finally {
        abortRef.current = null;
        setState((current) => ({ ...current, isStreaming: false }));
      }
    },
    [state.isStreaming],
  );

  return { ...state, send, stop };
}

function localMessage(
  id: string,
  conversationId: string,
  role: "user" | "assistant",
  content: string,
  createdAt: string,
): LocalChatMessage {
  return {
    id,
    conversation_id: conversationId,
    role,
    content,
    citations: [],
    tool_calls: [],
    token_usage: {},
    created_at: createdAt,
  };
}

export function applyStreamEvent(state: ChatStreamState, draftId: string, event: ChatStreamEvent): ChatStreamState {
  if (event.event === "retrieval") {
    return updateDraft(state, draftId, { citations: event.data.citations as Citation[] });
  }
  if (event.event === "token") {
    return {
      ...state,
      messages: state.messages.map((message) =>
        message.id === draftId ? { ...message, content: message.content + event.data.text } : message,
      ),
    };
  }
  if (event.event === "tool_call") {
    return {
      ...state,
      messages: state.messages.map((message) => {
        if (message.id !== draftId) return message;
        const thought = message.content.trim();
        const step: LocalToolStep = {
          step: message.tool_calls.length + 1,
          id: event.data.id,
          name: event.data.name,
          arguments: safeParseJson(event.data.arguments),
          ...(thought ? { thought } : {}),
          running: true,
        };
        // 思考文本已流式展示过，收进轨迹后清空气泡（spec §4.2）
        return { ...message, content: "", tool_calls: [...message.tool_calls, step] };
      }),
    };
  }
  if (event.event === "tool_result") {
    return {
      ...state,
      messages: state.messages.map((message) => {
        if (message.id !== draftId) return message;
        return {
          ...message,
          tool_calls: message.tool_calls.map((step) =>
            step.id === event.data.id
              ? {
                  ...step,
                  result: event.data.result,
                  duration_ms: event.data.duration_ms,
                  error: event.data.error,
                  running: false,
                }
              : step,
          ),
        };
      }),
    };
  }
  if (event.event === "done") {
    return updateDraft(state, draftId, {
      id: event.data.message_id,
      pending: false,
      token_usage: event.data.usage ?? {},
    });
  }
  if (event.event === "error") {
    return updateDraft({ ...state, error: event.data.message, isStreaming: false }, draftId, {
      pending: false,
      error: event.data.message,
    });
  }
  return state;
}

function updateDraft(state: ChatStreamState, draftId: string, patch: Partial<LocalChatMessage>): ChatStreamState {
  return {
    ...state,
    messages: state.messages.map((message) => (message.id === draftId ? { ...message, ...patch } : message)),
  };
}

function safeParseJson(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}
