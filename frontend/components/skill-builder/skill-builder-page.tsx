"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Sparkles,
  Send,
  Square,
  RotateCcw,
  Save,
  FileText,
  Plus,
  Trash2,
} from "lucide-react";
import { useModelConnection } from "@/components/playground/playground-provider";
import {
  Field,
  fieldClass,
  primaryClass,
  secondaryClass,
  RegistryError,
} from "@/components/hub/registry-shared";
import { APIError } from "@/lib/api/client";
import {
  generateSkill,
  saveGeneratedSkill,
  skillMarkdown,
  type BuilderMessage,
  type SkillDraft,
} from "@/lib/api/skill-builder";

const examples = [
  "根据项目进展生成技术周报",
  "按团队规范审查产品需求",
  "用第一性原理分析产品方案",
];
export function SkillBuilderPage() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const {
    connection,
    loading,
    error: connectionError,
    reload,
  } = useModelConnection();
  const [mode, setMode] = useState<"task" | "perspective">("task");
  const [model, setModel] = useState("");
  const [messages, setMessages] = useState<BuilderMessage[]>([]);
  const [input, setInput] = useState("");
  const [material, setMaterial] = useState("");
  const [draft, setDraft] = useState<SkillDraft | null>(null);
  const [preview, setPreview] = useState(false);
  const [fileView, setFileView] = useState("SKILL.md");
  const [busy, setBusy] = useState<"generate" | "save" | null>(null);
  const [error, setError] = useState<unknown>();
  const [notice, setNotice] = useState("");
  const active = useRef<AbortController | null>(null);
  const alive = useRef(true);
  const generating = useRef(false);
  const materialRead = useRef(0);
  const activeModel = connection?.models.includes(model)
    ? model
    : (connection?.defaultModel ?? "");
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      active.current?.abort();
    };
  }, []);
  // A connection update can change both provider and key; never let a late
  // response from the old connection overwrite the current draft.
  useEffect(() => {
    if (active.current && generating.current) {
      active.current.abort();
      active.current = null;
      setBusy(null);
      setNotice("模型配置已变化，生成已停止；原草案已保留。");
    }
  }, [connection]);
  function stop() {
    active.current?.abort();
    active.current = null;
    setBusy(null);
    setNotice("已停止生成，原草案已保留。需求文字保留，可修改后重试。");
  }
  function reset() {
    if (
      (messages.length || draft || input || material) &&
      !window.confirm("开始新建将清除本页未保存的对话与草案，是否继续？")
    )
      return;
    materialRead.current++;
    active.current?.abort();
    active.current = null;
    setBusy(null);
    setMessages([]);
    setDraft(null);
    setInput("");
    setMaterial("");
    setError(undefined);
    setNotice("");
    setFileView("SKILL.md");
  }
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (
      active.current ||
      busy ||
      !activeModel ||
      loading ||
      connectionError ||
      !input.trim()
    )
      return;
    setError(undefined);
    setNotice("");
    const history: BuilderMessage[] = [
      ...messages,
      { role: "user", content: input.trim() },
    ];
    if (history.length > 24) {
      setNotice("这轮对话已达到上限，请先保存草稿，再开始新的创建。");
      return;
    }
    if (new TextEncoder().encode(material).length > 48 * 1024) {
      setNotice("参考素材最多 48 KiB，请精简后重试。");
      return;
    }
    const controller = new AbortController();
    active.current = controller;
    generating.current = true;
    materialRead.current++;
    setBusy("generate");
    try {
      const reply = await generateSkill(
        {
          mode,
          model: activeModel,
          messages: history,
          material,
          ...(draft ? { draft } : {}),
        },
        controller.signal,
      );
      if (controller.signal.aborted || !alive.current) return;
      setMessages([...history, { role: "assistant", content: reply.message }]);
      setInput("");
      if (reply.draft) {
        setDraft({ ...reply.draft, files: reply.draft.files ?? [] });
        setFileView("SKILL.md");
      }
    } catch (reason) {
      if (!controller.signal.aborted && alive.current) setError(reason);
    } finally {
      if (active.current === controller) {
        active.current = null;
        if (alive.current) setBusy(null);
      }
    }
  }
  async function save() {
    if (!draft || busy || active.current) return;
    const controller = new AbortController();
    active.current = controller;
    generating.current = false;
    setBusy("save");
    setError(undefined);
    setNotice("");
    try {
      const saved = await saveGeneratedSkill(draft, controller.signal);
      if (controller.signal.aborted || !alive.current) return;
      queryClient.setQueryData(["registry", "detail", saved.slug], saved);
      await queryClient.invalidateQueries({ queryKey: ["registry"] });
      if (!controller.signal.aborted && alive.current)
        router.push(`/hub/registry/${saved.slug}`);
    } catch (reason) {
      if (!controller.signal.aborted && alive.current) setError(reason);
    } finally {
      if (active.current === controller) {
        active.current = null;
        if (alive.current) setBusy(null);
      }
    }
  }
  async function importMaterial(file?: File) {
    if (!file) return;
    const read = ++materialRead.current;
    if (!/\.(md|txt)$/i.test(file.name) || file.size > 48 * 1024) {
      setNotice("请选择不超过 48 KiB 的 .md 或 .txt 文件。");
      return;
    }
    try {
      const text = await file.text();
      if (alive.current && materialRead.current === read && !active.current) {
        setMaterial(text);
        setNotice(`已导入 ${file.name}，本轮将作为参考素材。`);
      }
    } catch {
      if (alive.current && materialRead.current === read)
        setNotice("素材读取失败，请重试或直接粘贴文本。");
    }
  }
  function change<K extends keyof SkillDraft>(key: K, value: SkillDraft[K]) {
    setDraft((previous) => (previous ? { ...previous, [key]: value } : null));
  }
  function addFile() {
    if (!draft || draft.files.length >= 10) return;
    let n = 1;
    while (draft.files.some((f) => f.path === `references/note-${n}.md`)) n++;
    change("files", [
      ...draft.files,
      { path: `references/note-${n}.md`, content: "" },
    ]);
  }
  return (
    <div className="space-y-6">
      <Link
        href="/hub/registry"
        className="inline-flex items-center gap-2 text-sm text-zinc-500"
      >
        <ArrowLeft className="size-4" />
        注册中心
      </Link>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">Skill 创建助手</h1>
          <p className="mt-2 text-sm text-zinc-500">
            说出你想完成的工作，一起整理成可复用的 Skill。
          </p>
        </div>
        <button
          onClick={reset}
          disabled={busy === "save"}
          className={secondaryClass}
        >
          <RotateCcw className="size-4" />
          开始新建
        </button>
      </header>
      <p className="rounded-xl border border-zinc-100 bg-zinc-50/60 px-4 py-3 text-xs leading-6 text-zinc-700">
        生成后可以编辑并保存为自己的草稿。对话、素材和未保存内容仅在本页保留；离开页面前请保存。
      </p>
      {!!error && <RegistryError error={error} />}
      {error instanceof APIError && error.status === 409 && (
        <p className="text-sm text-amber-700">
          该标识可能已被使用，或上次保存已经成功。请先在“我的发布”检查，再修改标识重试。
        </p>
      )}
      {notice && (
        <p
          role="status"
          className="rounded-xl bg-zinc-100 p-3 text-sm text-zinc-600"
        >
          {notice}
        </p>
      )}
      <div className="grid items-start gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
        <section
          className="min-w-0 space-y-5 rounded-xl border border-zinc-200 bg-white p-5 sm:p-6"
          aria-label="需求对话"
        >
          <fieldset disabled={!!busy} className="grid gap-4 sm:grid-cols-2">
            <Field label="创建方式">
              <select
                className={fieldClass}
                value={mode}
                disabled={!!messages.length}
                onChange={(e) => setMode(e.target.value as typeof mode)}
              >
                <option value="task">通用工作流程</option>
                <option value="perspective">专家思维 · 女娲方法</option>
              </select>
            </Field>
            <Field label="使用模型">
              <select
                className={fieldClass}
                value={activeModel}
                disabled={!connection || loading}
                onChange={(e) => setModel(e.target.value)}
              >
                <option value="" disabled>
                  请选择模型
                </option>
                {connection?.models.map((value) => (
                  <option key={value} value={value}>
                    {value}
                  </option>
                ))}
              </select>
            </Field>
          </fieldset>
          {mode === "perspective" && (
            <p className="text-xs leading-6 text-zinc-500">
              根据人物思维或主题方法论提炼辅助视角，基于你提供的素材与模型知识。采用
              <a
                className="text-zinc-600 underline"
                href="https://github.com/alchaincyf/nuwa-skill"
                target="_blank"
                rel="noreferrer"
              >
                女娲方法的轻量适配
              </a>
              ，没有自动联网调研或人物保真验证。
            </p>
          )}
          {loading ? (
            <p role="status" className="text-sm text-zinc-500">
              正在读取模型配置…
            </p>
          ) : connectionError ? (
            <p role="alert" className="text-sm text-red-600">
              {connectionError}
              <button onClick={reload} className="ml-2 underline">
                重试
              </button>
            </p>
          ) : (
            !connection && (
              <p className="rounded-xl bg-amber-50 p-4 text-sm leading-6 text-amber-800">
                请先在
                <Link href="/hub/settings" className="underline">
                  设置 → 模型接口
                </Link>
                配置可用模型，再开始对话。
              </p>
            )
          )}
          <details className="rounded-xl border border-zinc-200 p-4">
            <summary className="cursor-pointer text-sm font-medium">
              参考素材（可选）
            </summary>
            <fieldset disabled={!!busy} className="mt-3 space-y-3">
              <Field
                label="粘贴模板或说明"
                hint="仅支持文本素材，最多 48 KiB；不会自动打开链接或检索外部资料。"
              >
                <textarea
                  rows={5}
                  maxLength={48000}
                  className={fieldClass}
                  value={material}
                  onChange={(e) => {
                    materialRead.current++;
                    setMaterial(e.target.value);
                  }}
                  placeholder="粘贴现有模板、团队规范、方法笔记…"
                />
              </Field>
              <Field label="导入文本素材">
                <input
                  type="file"
                  accept=".md,.txt,text/plain,text/markdown"
                  className={fieldClass}
                  onChange={(e) => void importMaterial(e.target.files?.[0])}
                />
              </Field>
            </fieldset>
          </details>
          <div
            aria-live="polite"
            className="max-h-[440px] space-y-4 overflow-y-auto"
          >
            {messages.length === 0 ? (
              <div className="py-4">
                <div className="mb-3 inline-flex rounded-xl bg-zinc-50 p-3 text-zinc-500">
                  <Sparkles className="size-6" />
                </div>
                <h2 className="font-medium">你希望这个 Skill 帮你做什么？</h2>
                <p className="mt-2 text-sm leading-6 text-zinc-500">
                  可以从一个具体任务开始。信息不足时，助手会问你几个必要的问题。
                </p>
                <div className="mt-4 flex flex-wrap gap-2">
                  {examples.map((example) => (
                    <button
                      key={example}
                      disabled={!!busy}
                      onClick={() => setInput(example)}
                      className="rounded-lg border border-zinc-200 px-3 py-2 text-left text-xs text-zinc-600 hover:bg-zinc-50"
                    >
                      {example}
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              messages.map((message, i) => (
                <div
                  key={i}
                  className={`rounded-xl p-4 ${message.role === "user" ? "bg-zinc-50" : "bg-zinc-50"}`}
                >
                  <p className="mb-2 text-xs font-medium text-zinc-400">
                    {message.role === "user" ? "你" : "创建助手"}
                  </p>
                  <p className="whitespace-pre-wrap break-words text-sm leading-6">
                    {message.content}
                  </p>
                </div>
              ))
            )}
            {busy === "generate" && (
              <p role="status" className="animate-pulse text-sm text-zinc-600">
                正在整理需求与生成草案…
              </p>
            )}
          </div>
          <form onSubmit={(e) => void submit(e)} className="space-y-3">
            <Field label="你的需求或修改意见">
              <textarea
                rows={3}
                maxLength={6000}
                disabled={!!busy}
                className={fieldClass}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                placeholder={
                  draft
                    ? "例如：输出增加风险清单，语气更简洁…"
                    : "例如：帮我把团队周报模板变成 Skill…"
                }
              />
            </Field>
            <div className="flex justify-end">
              {busy === "generate" ? (
                <button type="button" onClick={stop} className={secondaryClass}>
                  <Square className="size-4" />
                  停止生成
                </button>
              ) : (
                <button
                  disabled={
                    !!busy ||
                    loading ||
                    !!connectionError ||
                    !activeModel ||
                    !input.trim() ||
                    messages.length >= 24
                  }
                  className={primaryClass}
                >
                  <Send className="size-4" />
                  {draft ? "继续修改" : "发送需求"}
                </button>
              )}
            </div>
          </form>
        </section>
        <section
          className="min-w-0 rounded-xl border border-zinc-200 bg-white p-5 sm:p-6"
          aria-label="Skill 草案"
        >
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="font-semibold">Skill 草案</h2>
            {draft && (
              <div
                role="group"
                aria-label="草案显示方式"
                className="flex gap-2"
              >
                <button
                  onClick={() => setPreview(false)}
                  aria-pressed={!preview}
                  className={secondaryClass}
                >
                  编辑
                </button>
                <button
                  onClick={() => setPreview(true)}
                  aria-pressed={preview}
                  className={secondaryClass}
                >
                  文件预览
                </button>
              </div>
            )}
          </div>
          {!draft ? (
            <div className="flex min-h-96 flex-col items-center justify-center text-center">
              <FileText className="size-12 text-zinc-200" />
              <h3 className="mt-5 font-medium text-zinc-600">
                从需求到一份可用的 Skill
              </h3>
              <p className="mt-2 max-w-sm text-sm leading-6 text-zinc-400">
                生成后，名称、使用说明和附件会出现在这里。你可以逐项修改，确认后保存。
              </p>
            </div>
          ) : (
            <>
              {preview ? (
                <div className="mt-5 space-y-4">
                  <Field label="预览文件">
                    <select
                      className={fieldClass}
                      value={fileView}
                      onChange={(e) => setFileView(e.target.value)}
                    >
                      <option value="SKILL.md">SKILL.md</option>
                      {draft.files.map((file, i) => (
                        <option key={i} value={file.path}>
                          {file.path}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <pre className="max-h-[720px] overflow-auto whitespace-pre-wrap break-words rounded-xl bg-zinc-50 p-4 font-mono text-xs leading-6">
                    {fileView === "SKILL.md"
                      ? skillMarkdown(draft)
                      : draft.files.find((file) => file.path === fileView)
                          ?.content}
                  </pre>
                </div>
              ) : (
                <fieldset disabled={!!busy} className="mt-5 space-y-5">
                  <div className="grid gap-4 sm:grid-cols-2">
                    <Field label="Skill 名称">
                      <input
                        required
                        maxLength={120}
                        className={fieldClass}
                        value={draft.name}
                        onChange={(e) => change("name", e.target.value)}
                      />
                    </Field>
                    <Field
                      label="唯一标识"
                      hint="小写字母、数字、单连字符，最多 64 字符。"
                    >
                      <input
                        required
                        maxLength={64}
                        className={fieldClass}
                        value={draft.slug}
                        onChange={(e) => change("slug", e.target.value)}
                      />
                    </Field>
                  </div>
                  <Field label="用途与触发条件">
                    <textarea
                      rows={3}
                      maxLength={1024}
                      className={fieldClass}
                      value={draft.description}
                      onChange={(e) => change("description", e.target.value)}
                    />
                  </Field>
                  <Field
                    label="SKILL.md 正文"
                    hint="名称与描述会自动生成文件头，正文使用 Markdown。"
                  >
                    <textarea
                      rows={16}
                      maxLength={64000}
                      spellCheck={false}
                      className={`${fieldClass} font-mono text-xs leading-6`}
                      value={draft.instructions}
                      onChange={(e) => change("instructions", e.target.value)}
                    />
                  </Field>
                  <div className="flex items-center justify-between">
                    <h3 className="text-sm font-medium">
                      文本附件 · {draft.files.length}/10
                    </h3>
                    <button
                      type="button"
                      onClick={addFile}
                      disabled={draft.files.length >= 10}
                      className={secondaryClass}
                    >
                      <Plus className="size-4" />
                      添加附件
                    </button>
                  </div>
                  {draft.files.map((file, i) => (
                    <div
                      key={i}
                      className="space-y-3 rounded-xl border border-zinc-200 p-4"
                    >
                      <Field
                        label={`附件 ${i + 1} 路径`}
                        hint="references/ 或 assets/ 下的 .md/.txt/.json/.yaml/.yml 文件。"
                      >
                        <input
                          className={fieldClass}
                          value={file.path}
                          onChange={(e) =>
                            change(
                              "files",
                              draft.files.map((f, j) =>
                                j === i ? { ...f, path: e.target.value } : f,
                              ),
                            )
                          }
                        />
                      </Field>
                      <Field label={`附件 ${i + 1} 内容`}>
                        <textarea
                          rows={5}
                          maxLength={32768}
                          className={`${fieldClass} font-mono text-xs`}
                          value={file.content}
                          onChange={(e) =>
                            change(
                              "files",
                              draft.files.map((f, j) =>
                                j === i ? { ...f, content: e.target.value } : f,
                              ),
                            )
                          }
                        />
                      </Field>
                      <button
                        type="button"
                        onClick={() => {
                          change(
                            "files",
                            draft.files.filter((_, j) => j !== i),
                          );
                          setFileView("SKILL.md");
                        }}
                        className="inline-flex items-center gap-2 text-xs text-red-600"
                      >
                        <Trash2 className="size-3" />
                        移除附件 {i + 1}
                      </button>
                    </div>
                  ))}
                </fieldset>
              )}
              <div className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-zinc-100 pt-5">
                <p className="max-w-xs text-xs leading-5 text-zinc-400">
                  将创建 1.0.0
                  草稿，不会立即上线。可在详情页配置可见范围并提交管理员审核。
                </p>
                <button
                  disabled={!!busy}
                  onClick={() => void save()}
                  className={primaryClass}
                >
                  <Save className="size-4" />
                  {busy === "save" ? "正在保存…" : "保存到注册中心"}
                </button>
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
