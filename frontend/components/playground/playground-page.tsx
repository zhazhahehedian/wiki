"use client";

import { useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  ArrowDown,
  MessageSquarePlus,
  Code2,
  FileText,
  Lightbulb,
  Trash2,
} from "lucide-react";
import { Select } from "@/components/ui/select";
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
  const transcript = useRef<HTMLDivElement>(null);
  const composer = useRef<HTMLDivElement>(null);
  const followLatest = useRef(true);
  const [composerHeight, setComposerHeight] = useState(220);
  const [showLatest, setShowLatest] = useState(false);

  function scrollToLatest() {
    followLatest.current = true;
    setShowLatest(false);
    if (transcript.current)
      transcript.current.scrollTop = transcript.current.scrollHeight;
  }

  useEffect(() => {
    const element = composer.current;
    if (!element) return;
    // Keep the final reply above the floating input, including wrapped mobile controls.
    const observer = new ResizeObserver(() => {
      setComposerHeight(element.getBoundingClientRect().height);
      if (followLatest.current) scrollToLatest();
    });
    observer.observe(element);
    if (transcript.current) observer.observe(transcript.current);
    return () => observer.disconnect();
  }, []);

  useEffect(() => {
    if (followLatest.current) scrollToLatest();
  }, [messages, composerHeight]);

  useEffect(() => {
    request.current?.abort();
    setMessages([]);
    lastHistory.current = [];
    setStreamError("");
    setModel(connection?.defaultModel ?? "");
    scrollToLatest();
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
    scrollToLatest();
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
    <section
      aria-label="模型对话"
      className="relative isolate h-full min-h-0 min-w-0 overflow-hidden rounded-2xl border border-zinc-200 bg-white"
    >
      <div className="absolute right-3 top-3 z-20 max-w-[calc(100%-1.5rem)] sm:right-5 sm:top-5">
        <ConnectionSettings compact />
      </div>
      <div
        ref={transcript}
        aria-label="对话消息"
        tabIndex={0}
        onScroll={(event) => {
          const element = event.currentTarget;
          const atBottom =
            element.scrollHeight - element.scrollTop - element.clientHeight <
            48;
          followLatest.current = atBottom;
          setShowLatest(!atBottom);
        }}
        className="absolute inset-0 overflow-x-hidden overflow-y-auto overscroll-contain [scrollbar-gutter:stable_both-edges]"
      >
        <div
          className="mx-auto flex min-h-full w-full max-w-4xl flex-col px-4 pt-20 sm:px-6"
          style={{ paddingBottom: composerHeight + 32 }}
        >
          {messages.length > 0 ? (
            <div className="space-y-8">
              {messages.map((message, index) => (
                <article
                  key={index}
                  aria-label={message.role === "user" ? "我的消息" : "模型回复"}
                  className={
                    message.role === "user"
                      ? "ml-auto w-fit max-w-[90%] break-words rounded-3xl rounded-br-lg bg-zinc-100/80 px-5 py-4"
                      : "min-w-0 break-words px-1 py-2"
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
            <div className="flex flex-1 flex-col items-center justify-center py-8 text-center">
              <span className="flex size-11 items-center justify-center rounded-xl border border-zinc-100 bg-zinc-50 text-zinc-500">
                <MessageSquarePlus className="size-5" strokeWidth={1.5} />
              </span>
              <h1 className="mt-5 text-xl font-semibold tracking-tight">
                开始一场对话
              </h1>
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
        </div>
      </div>
      <div
        ref={composer}
        className="pointer-events-none absolute inset-x-0 bottom-0 z-10 px-3 pb-3 before:absolute before:inset-x-0 before:bottom-0 before:h-12 before:bg-white sm:px-6 sm:pb-4"
      >
        <div className="pointer-events-auto relative mx-auto w-full max-w-4xl">
          {showLatest && (
            <Button
              variant="outline"
              size="icon"
              aria-label="回到最新消息"
              className="absolute -top-12 left-1/2 -translate-x-1/2 rounded-full bg-white shadow-sm"
              onClick={scrollToLatest}
            >
              <ArrowDown className="size-4" />
            </Button>
          )}
          {streamError && (
            <p
              role="alert"
              className="mb-3 rounded-xl border border-red-100 bg-red-50 px-4 py-3 text-sm text-red-600"
            >
              {streamError}
            </p>
          )}
          {messages.length > 0 && (
            <div className="mb-3 flex justify-end gap-2">
              <Button
                variant="outline"
                className="rounded-full bg-white"
                disabled={generating}
                onClick={() => {
                  setMessages([]);
                  setStreamError("");
                  lastHistory.current = [];
                  scrollToLatest();
                }}
              >
                新对话
              </Button>
              <Button
                variant="outline"
                className="rounded-full bg-white"
                disabled={generating}
                onClick={() => void generate(lastHistory.current)}
              >
                重新生成
              </Button>
            </div>
          )}
          <div
            aria-label="消息输入区"
            className="overflow-hidden rounded-[28px] border border-zinc-200/80 bg-zinc-50/95 shadow-[0_12px_40px_-10px_rgba(0,0,0,0.16),0_2px_8px_rgba(0,0,0,0.03)] backdrop-blur-xl transition-[border-color,box-shadow] focus-within:border-zinc-300 focus-within:shadow-[0_12px_40px_-10px_rgba(0,0,0,0.20)]"
          >
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
              placeholder="随便问…"
              className="min-h-24 max-h-[min(15rem,30dvh)] resize-none rounded-none border-0 bg-transparent px-5 py-4 shadow-none focus-visible:ring-0 sm:px-6"
            />
            <div className="flex flex-wrap items-center justify-between gap-2 bg-zinc-100/40 p-3">
              <div className="flex items-center gap-1 rounded-full border border-zinc-100 bg-white p-1 shadow-sm">
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
                  className="rounded-full"
                  aria-label="清空输入"
                  disabled={!draft}
                  onClick={() => setDraft("")}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
              <div className="ml-auto flex min-w-0 max-w-full items-center gap-2">
                <label htmlFor="chat-model" className="sr-only">
                  对话模型
                </label>
                <Select
                  id="chat-model"
                  value={activeModel}
                  disabled={!connection || generating || loading}
                  onValueChange={(value) => setModel(value)}
                  className="w-44 rounded-full border-zinc-200/70 px-4 font-medium sm:w-56"
                  placeholder="请先配置模型接口"
                  options={(connection?.models ?? []).map((value) => ({
                    value,
                    label: value,
                  }))}
                />
                <Button
                  size="icon-lg"
                  className="rounded-full"
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
            className={
              connection
                ? "sr-only"
                : "relative mt-2 text-center text-[11px] leading-4 text-zinc-400"
            }
          >
            {connection
              ? "对话仅在当前页面保留；Enter 发送，Shift + Enter 换行。"
              : "先在右上角配置 API 地址、API Key 和模型。"}
          </p>
        </div>
      </div>
    </section>
  );
}
