"use client";

import { useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  MessageSquarePlus,
  Code2,
  FileText,
  Lightbulb,
  Trash2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { MarkdownContent } from "@/components/chat/markdown-content";
import {
  streamModelChat,
  type ChatMessage,
  type ChatParameters,
} from "@/lib/api/playground";
import { APIError } from "@/lib/api/client";
import { ConnectionSettings } from "./connection-settings";
import { useModelConnection } from "./playground-provider";

import {
  ChatParameterSettings,
  parametersForProtocol,
} from "./chat-parameters";

const starters = [
  {
    icon: Lightbulb,
    label: "一起想点子",
    prompt: "帮我为团队内部的技术分享想几个有趣的选题。",
  },
  {
    icon: Code2,
    label: "聊聊代码",
    prompt: "我想让一段代码更清晰，请帮我分析可以改进的地方。",
  },
  {
    icon: FileText,
    label: "整理思路",
    prompt: "请帮我把接下来的零散笔记整理成结构清晰的提纲。",
  },
];

export function PlaygroundPage() {
  const {
    connection,
    loading,
    error: configurationError,
  } = useModelConnection();
  type DisplayMessage = ChatMessage & {
    status?: "complete" | "stopped" | "failed";
  };
  const [messages, setMessages] = useState<DisplayMessage[]>([]);
  const [generating, setGenerating] = useState(false);
  const [streamError, setStreamError] = useState("");
  const request = useRef<AbortController | null>(null);
  const lastHistory = useRef<DisplayMessage[]>([]);
  const [model, setModel] = useState("");
  const [draft, setDraft] = useState("");
  const [parameters, setParameters] = useState<ChatParameters>({});
  const [systemPrompt, setSystemPrompt] = useState("");
  useEffect(() => {
    request.current?.abort();
    setMessages([]);
    lastHistory.current = [];
    setStreamError("");
    setModel(connection?.defaultModel ?? "");
  }, [connection]);
  const activeModel = connection?.models.includes(model)
    ? model
    : (connection?.defaultModel ?? "");
  useEffect(() => () => request.current?.abort(), []);
  async function generate(history: DisplayMessage[]) {
    if (
      !connection ||
      !activeModel ||
      request.current ||
      loading ||
      configurationError
    )
      return;
    const controller = new AbortController();
    request.current = controller;
    lastHistory.current = history;
    setGenerating(true);
    setStreamError("");
    setMessages([
      ...history,
      { role: "assistant", content: "", status: "complete" },
    ]);
    const payload: ChatMessage[] = history.map(({ role, content }) => ({
      role,
      content,
    }));
    if (systemPrompt.trim())
      payload.unshift({ role: "system", content: systemPrompt.trim() });
    try {
      await streamModelChat(
        {
          model: activeModel,
          messages: payload,
          ...parametersForProtocol(parameters, connection.protocol),
        },
        controller.signal,
        (text) => {
          if (!controller.signal.aborted)
            setMessages((current) =>
              current.map((message, index) =>
                index === history.length
                  ? { ...message, content: message.content + text }
                  : message,
              ),
            );
        },
      );
    } catch (error) {
      setMessages((current) =>
        current.map((message, index) =>
          index === history.length
            ? {
                ...message,
                status: controller.signal.aborted ? "stopped" : "failed",
              }
            : message,
        ),
      );
      if (!controller.signal.aborted)
        setStreamError(
          error instanceof APIError && error.status === 400
            ? "模型或参数无效，请检查配置。"
            : "模型响应失败，请检查配置或重试。已收到的部分内容保留如下。",
        );
    } finally {
      if (request.current === controller) {
        request.current = null;
        setGenerating(false);
      }
    }
  }
  function send() {
    if (!draft.trim() || generating) return;
    const history = messages.filter(
      (message) =>
        (!message.status || message.status === "complete") && message.content,
    );
    void generate([
      ...history,
      { role: "user", content: draft.trim(), status: "complete" },
    ]);
    setDraft("");
  }
  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">游乐场</h1>
          <p className="mt-2 text-sm text-zinc-500">
            选择模型，发送消息，按需调整参数。
          </p>
        </div>
        <div className="flex gap-2">
          <ConnectionSettings compact />
        </div>
      </div>
      <div className="flex flex-col gap-5 xl:flex-row">
        <section
          aria-label="模型对话"
          className="flex min-h-[min(680px,calc(100dvh-190px))] min-w-0 flex-1 flex-col overflow-hidden rounded-xl border border-zinc-200 bg-white"
        >
          {messages.length > 0 ? (
            <div
              aria-label="对话消息"
              className="flex-1 space-y-6 overflow-x-hidden px-5 py-6 sm:px-8"
            >
              {messages.map((message, index) => (
                <article
                  key={index}
                  aria-label={message.role === "user" ? "我的消息" : "模型回复"}
                  className={
                    message.role === "user"
                      ? "ml-auto max-w-3xl rounded-xl bg-zinc-50 p-4"
                      : "max-w-3xl rounded-xl border border-zinc-100 p-4"
                  }
                >
                  <p className="mb-3 text-xs font-medium text-zinc-400">
                    {message.role === "user" ? "你" : activeModel}
                  </p>
                  <MarkdownContent
                    content={
                      message.content ||
                      (generating ? "正在思考…" : "未收到回复")
                    }
                  />
                  {message.status === "stopped" && (
                    <p className="mt-3 text-xs text-zinc-400">已停止生成</p>
                  )}
                  {message.status === "failed" && (
                    <p className="mt-3 text-xs text-red-500">响应未完成</p>
                  )}
                </article>
              ))}
            </div>
          ) : (
            <div className="flex flex-1 flex-col items-center justify-center px-6 py-12 text-center">
              <span className="flex size-11 items-center justify-center rounded-xl border border-zinc-100 bg-zinc-50 text-zinc-500">
                <MessageSquarePlus className="size-5" strokeWidth={1.5} />
              </span>
              <h2 className="mt-5 text-xl font-semibold tracking-tight">
                开始一场对话
              </h2>
              <p className="mt-3 max-w-md text-sm leading-7 text-zinc-500">
                选择下方示例，或直接输入你的问题。
              </p>
              <div className="mt-6 grid w-full max-w-2xl gap-3 sm:grid-cols-3">
                {starters.map(({ icon: Icon, label, prompt }) => (
                  <button
                    type="button"
                    key={label}
                    onClick={() => setDraft(prompt)}
                    className="flex items-center gap-3 rounded-xl border border-zinc-200 px-4 py-3 text-left text-sm text-zinc-600 transition hover:border-zinc-200 hover:bg-zinc-50/50"
                  >
                    <Icon className="size-4 shrink-0 text-zinc-500" />
                    {label}
                  </button>
                ))}
              </div>
            </div>
          )}
          <div className="px-4 pb-4 sm:px-6 sm:pb-6">
            {streamError && (
              <p role="alert" className="mb-4 text-sm text-red-600">
                {streamError}
              </p>
            )}
            {messages.length > 0 && (
              <div className="mb-3 flex justify-end gap-2">
                <Button
                  variant="ghost"
                  disabled={generating}
                  onClick={() => {
                    setMessages([]);
                    setStreamError("");
                    lastHistory.current = [];
                  }}
                >
                  新对话
                </Button>
                <Button
                  variant="outline"
                  disabled={generating}
                  onClick={() => void generate(lastHistory.current)}
                >
                  重新生成
                </Button>
              </div>
            )}
            <div className="rounded-xl border border-zinc-200 bg-zinc-50/50 p-3 focus-within:border-zinc-300">
              <Textarea
                aria-label="消息"
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (
                    event.key === "Enter" &&
                    !event.shiftKey &&
                    !event.nativeEvent.isComposing
                  ) {
                    event.preventDefault();
                    if (connection && !loading && !configurationError) send();
                  }
                }}
                placeholder="输入你的问题…"
                className="min-h-24 max-h-60 resize-none border-0 bg-transparent shadow-none focus-visible:ring-0"
              />
              <div className="mt-2 flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-1">
                  <ChatParameterSettings
                    parameters={parameters}
                    onChange={setParameters}
                    systemPrompt={systemPrompt}
                    onSystemPromptChange={setSystemPrompt}
                    protocol={connection?.protocol ?? "openai"}
                    disabled={generating || loading}
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="清空输入"
                    disabled={!draft}
                    onClick={() => setDraft("")}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
                <div className="ml-auto flex min-w-0 items-center gap-2">
                  <label htmlFor="chat-model" className="sr-only">
                    对话模型
                  </label>
                  <select
                    id="chat-model"
                    value={activeModel}
                    disabled={!connection || generating || loading}
                    onChange={(event) => setModel(event.target.value)}
                    className="w-44 max-w-full truncate rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm font-medium disabled:text-zinc-400"
                  >
                    {!connection && <option value="">请先配置模型接口</option>}
                    {connection?.models.map((value) => (
                      <option key={value} value={value}>
                        {value}
                      </option>
                    ))}
                  </select>
                  <Button
                    size="icon"
                    aria-label={generating ? "停止生成" : "发送消息"}
                    disabled={
                      !generating &&
                      (!connection ||
                        !draft.trim() ||
                        loading ||
                        Boolean(configurationError))
                    }
                    onClick={() =>
                      generating ? request.current?.abort() : send()
                    }
                    aria-describedby="chat-availability"
                  >
                    {generating ? (
                      <span className="size-3 rounded-sm bg-current" />
                    ) : (
                      <ArrowUp className="size-4" />
                    )}
                  </Button>
                </div>
              </div>
            </div>
            <p
              id="chat-availability"
              className="mt-3 text-center text-xs leading-5 text-zinc-500"
            >
              {connection
                ? "对话仅在当前页面保留；Enter 发送，Shift + Enter 换行。"
                : "先在右上角配置 API 地址、API Key 和模型。"}
            </p>
          </div>
        </section>
      </div>
    </div>
  );
}
