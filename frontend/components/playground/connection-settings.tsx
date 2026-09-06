"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import { playgroundApi, type ModelProtocol } from "@/lib/api/playground";
import { Eye, EyeOff, KeyRound, Settings2, Unplug } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  configurationError,
  normalizeBaseUrl,
  parseModels,
  useModelConnection,
} from "./playground-provider";

export function ConnectionSettings({ compact = false }: { compact?: boolean }) {
  const {
    connection,
    loading,
    error: loadError,
    reload,
    clearConnection,
  } = useModelConnection();
  const [open, setOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  async function remove() {
    setDeleting(true);
    setDeleteError("");
    try {
      await clearConnection();
    } catch (error) {
      setDeleteError(configurationError(error));
    } finally {
      setDeleting(false);
    }
  }
  return (
    <>
      {compact ? (
        <Button
          variant="outline"
          disabled={loading || Boolean(loadError)}
          onClick={() => setOpen(true)}
        >
          <Settings2 className="size-4" />
          接口配置
        </Button>
      ) : (
        <section className="rounded-xl border border-zinc-200 bg-white p-6">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div className="flex gap-3">
              <span className="rounded-xl bg-zinc-50 p-2.5 text-zinc-500">
                <KeyRound className="size-5" />
              </span>
              <div>
                <h2 className="font-semibold">模型接口</h2>
                <p className="mt-1 text-xs leading-6 text-zinc-500">
                  使用自己的 API Key，连接 Token Hub、OpenAI 兼容服务或 Claude
                  原生接口。
                </p>
              </div>
            </div>
            <Button
              variant="outline"
              disabled={loading || Boolean(loadError)}
              onClick={() => setOpen(true)}
            >
              {connection ? "编辑接口配置" : "配置模型接口"}
            </Button>
          </div>
          {connection ? (
            <dl className="mt-5 space-y-3 rounded-xl bg-zinc-50 p-4 text-sm">
              <div>
                <dt className="text-xs text-zinc-500">API 地址</dt>
                <dd className="mt-1 break-all">{connection.baseUrl}</dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-500">默认模型</dt>
                <dd className="mt-1 break-all">{connection.defaultModel}</dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-500">接口协议</dt>
                <dd className="mt-1">
                  {connection.protocol === "anthropic"
                    ? "Claude（Anthropic）"
                    : "OpenAI 兼容"}
                </dd>
              </div>
              <div>
                <dt className="text-xs text-zinc-500">API Key</dt>
                <dd className="mt-1">已加密保存</dd>
              </div>
            </dl>
          ) : (
            <p className="mt-5 rounded-xl bg-zinc-50 p-4 text-sm text-zinc-500">
              {loading ? "正在加载配置…" : "尚未配置模型接口"}
            </p>
          )}
          <p className="mt-4 text-xs leading-6 text-zinc-500">
            配置保存在你的账号下，API Key
            加密保存且不会回显。对话内容不保存到服务器。
          </p>
          {connection && (
            <Button
              className="mt-3"
              variant="ghost"
              disabled={deleting}
              onClick={() => void remove()}
            >
              <Unplug className="size-4" />
              删除接口配置
            </Button>
          )}
        </section>
      )}
      {(loadError || deleteError) && (
        <div role="alert" className="text-sm text-red-600">
          {loadError || deleteError}
          {loadError && (
            <Button variant="ghost" onClick={reload}>
              重新加载配置
            </Button>
          )}
        </div>
      )}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto p-6 sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>模型接口配置</DialogTitle>
            <DialogDescription>
              填写接口 → 获取模型 → 选择默认模型。保存后即可在游乐场使用。
            </DialogDescription>
          </DialogHeader>
          {open && <ConnectionForm onDone={() => setOpen(false)} />}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ConnectionForm({ onDone }: { onDone: () => void }) {
  const { connection, saveConnection } = useModelConnection();
  const [protocol, setProtocol] = useState<ModelProtocol>(
    connection?.protocol ?? "openai",
  );
  const [baseUrl, setBaseUrl] = useState(
    connection?.baseUrl ?? "https://tokenhub.robosense.cn/v1",
  );
  const [apiKey, setApiKey] = useState("");
  const [modelText, setModelText] = useState(
    connection?.models.join("\n") ?? "",
  );
  const [defaultModel, setDefaultModel] = useState(
    connection?.defaultModel ?? "",
  );
  const [revealed, setRevealed] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [discovered, setDiscovered] = useState<string[] | null>(null);
  const [discovering, setDiscovering] = useState(false);
  const [discoveryError, setDiscoveryError] = useState("");
  const [modelSearch, setModelSearch] = useState("");
  const discovery = useRef<AbortController | null>(null);
  useEffect(() => () => discovery.current?.abort(), []);
  function resetDiscovery() {
    discovery.current?.abort();
    setDiscovering(false);
    setDiscovered(null);
    setDiscoveryError("");
  }
  async function fetchModels() {
    const normalized = normalizeBaseUrl(baseUrl, protocol);
    if (
      !normalized ||
      (!apiKey.trim() &&
        (!connection?.hasKey ||
          normalized !== connection.baseUrl ||
          protocol !== connection.protocol))
    ) {
      setDiscoveryError(
        "请先填写 API 地址和 Key。地址不变时可使用已保存的 Key。",
      );
      return;
    }
    discovery.current?.abort();
    const controller = new AbortController();
    discovery.current = controller;
    setDiscovering(true);
    setDiscoveryError("");
    try {
      const result = await playgroundApi.models(
        { baseUrl: normalized, apiKey: apiKey.trim(), protocol },
        controller.signal,
      );
      if (!controller.signal.aborted) setDiscovered(result.models);
    } catch {
      if (!controller.signal.aborted)
        setDiscoveryError(
          "获取模型失败，请检查接口地址和 Key；服务不提供模型列表时可手动填写。",
        );
    } finally {
      if (!controller.signal.aborted) setDiscovering(false);
    }
  }
  const models = parseModels(modelText);
  const selected = models.includes(defaultModel)
    ? defaultModel
    : (models[0] ?? "");

  async function apply(event: FormEvent) {
    event.preventDefault();
    if (saving) return;
    const normalized = normalizeBaseUrl(baseUrl, protocol);
    if (!normalized)
      return setError(
        "请输入有效的 HTTP(S) API 基础地址，不包含账号密码、查询参数或片段。",
      );
    if (
      !apiKey.trim() &&
      (!connection?.hasKey ||
        normalized !== connection.baseUrl ||
        protocol !== connection.protocol)
    )
      return setError("请填写 API Key。修改地址或协议时需重新填写密钥。");
    if (!models.length || models.length > 100)
      return setError("请选择或填写 1–100 个模型 ID。");
    setSaving(true);
    setError("");
    try {
      await saveConnection({
        protocol,
        baseUrl: normalized,
        apiKey: apiKey.trim(),
        models,
        defaultModel: selected,
        version: connection?.version ?? 0,
      });
      setApiKey("");
      onDone();
    } catch (error) {
      setError(configurationError(error));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form onSubmit={apply} noValidate className="space-y-5">
      <div className="space-y-2">
        <label htmlFor="model-protocol" className="text-sm font-medium">
          接口协议
        </label>
        <select
          id="model-protocol"
          value={protocol}
          className="h-9 w-full rounded-lg border border-zinc-200 bg-white px-3 text-sm"
          onChange={(e) => {
            const next = e.target.value as ModelProtocol;
            resetDiscovery();
            setProtocol(next);
            if (
              baseUrl === "https://tokenhub.robosense.cn/v1" &&
              next === "anthropic"
            )
              setBaseUrl("https://api.anthropic.com");
            else if (
              baseUrl === "https://api.anthropic.com" &&
              next === "openai"
            )
              setBaseUrl("https://tokenhub.robosense.cn/v1");
          }}
        >
          <option value="openai">OpenAI 兼容</option>
          <option value="anthropic">Claude（Anthropic）</option>
        </select>
      </div>
      <div className="space-y-2">
        <label htmlFor="model-base-url" className="text-sm font-medium">
          API 基础地址
        </label>
        <Input
          id="model-base-url"
          type="url"
          value={baseUrl}
          onChange={(event) => {
            resetDiscovery();
            setBaseUrl(event.target.value);
          }}
          autoComplete="off"
          aria-describedby="base-url-help"
        />
        <p id="base-url-help" className="text-xs leading-5 text-zinc-500">
          {protocol === "anthropic"
            ? "填写 Claude 原生接口根地址，例如 https://api.anthropic.com，不需要 /v1；平台自动拼接 /v1/models 和 /v1/messages。"
            : "填写 OpenAI 兼容基础地址，通常以 /v1 结尾；平台追加 /models 和 /chat/completions。"}
        </p>
      </div>
      <div className="space-y-2">
        <label htmlFor="model-api-key" className="text-sm font-medium">
          API Key
        </label>
        <div className="relative">
          <Input
            id="model-api-key"
            type={revealed ? "text" : "password"}
            value={apiKey}
            onChange={(event) => {
              resetDiscovery();
              setApiKey(event.target.value);
            }}
            autoComplete="off"
            spellCheck={false}
            className="pr-10"
            placeholder={
              connection?.hasKey ? "留空保留已保存的 Key" : "填写你的 API Key"
            }
          />
          <button
            type="button"
            aria-label={revealed ? "隐藏 API Key" : "显示 API Key"}
            aria-pressed={revealed}
            onClick={() => setRevealed(!revealed)}
            className="absolute inset-y-0 right-0 rounded-lg px-3 text-zinc-500"
          >
            {revealed ? (
              <EyeOff className="size-4" />
            ) : (
              <Eye className="size-4" />
            )}
          </button>
        </div>
      </div>
      <div className="space-y-2">
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm font-medium">可用模型</span>
          <Button
            type="button"
            variant="outline"
            disabled={discovering || saving}
            onClick={fetchModels}
          >
            {discovering ? "正在获取…" : "获取模型"}
          </Button>
        </div>
        {discoveryError && (
          <p role="alert" className="text-xs text-red-600">
            {discoveryError}
          </p>
        )}
        {discovered !== null && (
          <div className="space-y-2 rounded-xl border border-zinc-200 p-3">
            <p role="status" className="text-xs text-zinc-500">
              获取到 {discovered.length} 个模型，已选择 {models.length}{" "}
              个。请选择用于对话的模型。
            </p>
            {discovered.length > 0 && (
              <>
                <Input
                  aria-label="搜索可用模型"
                  placeholder="搜索模型"
                  value={modelSearch}
                  onChange={(e) => setModelSearch(e.target.value)}
                />
                <div className="max-h-40 space-y-1 overflow-y-auto">
                  {discovered
                    .filter((id) =>
                      id.toLowerCase().includes(modelSearch.toLowerCase()),
                    )
                    .map((id) => (
                      <label
                        key={id}
                        className="flex cursor-pointer items-center gap-2 rounded px-1 py-1.5 text-xs"
                      >
                        <input
                          type="checkbox"
                          checked={models.includes(id)}
                          disabled={
                            !models.includes(id) && models.length >= 100
                          }
                          onChange={(e) =>
                            setModelText(
                              (e.target.checked
                                ? [...models, id]
                                : models.filter((model) => model !== id)
                              ).join("\n"),
                            )
                          }
                        />
                        <span className="break-all">{id}</span>
                      </label>
                    ))}
                </div>
              </>
            )}
          </div>
        )}
        <label htmlFor="model-ids" className="text-sm font-medium">
          可用模型 ID
        </label>
        <Textarea
          id="model-ids"
          value={modelText}
          onChange={(event) => setModelText(event.target.value)}
          placeholder={"例如：your-chat-model\nyour-reasoning-model"}
          rows={3}
          spellCheck={false}
          aria-describedby="model-help"
        />
        <p id="model-help" className="text-xs leading-5 text-zinc-500">
          点击「获取模型」后勾选，或手动填写准确
          ID，每行一个或用逗号分隔。模型列表可能包含不支持对话的模型，请按服务说明选择。
        </p>
      </div>
      <div className="space-y-2">
        <label htmlFor="default-model" className="text-sm font-medium">
          默认模型
        </label>
        <select
          id="default-model"
          value={selected}
          onChange={(event) => setDefaultModel(event.target.value)}
          disabled={!models.length}
          className="h-9 w-full rounded-lg border border-zinc-200 bg-white px-3 text-sm disabled:text-zinc-400"
        >
          {!models.length && <option value="">先选择或填写模型</option>}
          {models.map((model) => (
            <option key={model} value={model}>
              {model}
            </option>
          ))}
        </select>
      </div>
      <p className="rounded-xl bg-zinc-50 p-3 text-xs leading-6 text-zinc-800">
        API Key 将加密保存。修改接口地址或协议时必须重新填写
        Key，保存配置不会自动发起模型调用。
      </p>
      {error && (
        <p role="alert" className="text-sm text-red-600">
          {error}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="outline" type="button" onClick={onDone}>
          取消
        </Button>
        <Button type="submit" disabled={saving}>
          {saving ? "正在保存…" : "保存配置"}
        </Button>
      </div>
    </form>
  );
}
