import { render, screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  PlaygroundProvider,
  normalizeBaseUrl,
  parseModels,
} from "./playground-provider";
import { ConnectionSettings } from "./connection-settings";
import { PlaygroundPage } from "./playground-page";
import {
  playgroundApi,
  streamModelChat,
  type ModelConnection,
} from "@/lib/api/playground";

vi.mock("@/lib/api/playground", () => ({
  playgroundApi: {
    get: vi.fn(),
    save: vi.fn(),
    delete: vi.fn(),
    models: vi.fn(),
  },
  streamModelChat: vi.fn(),
}));
let saved: ModelConnection | null;
beforeEach(() => {
  vi.resetAllMocks();
  saved = null;
  vi.mocked(playgroundApi.get).mockImplementation(async () => saved);
  vi.mocked(playgroundApi.save).mockImplementation(
    async ({ apiKey: _key, ...data }) =>
      (saved = { ...data, version: data.version + 1, hasKey: true }),
  );
  vi.mocked(playgroundApi.delete).mockImplementation(async () => {
    saved = null;
  });
  vi.mocked(streamModelChat).mockImplementation(
    async (_input, _signal, emit) => {
      emit("模型回复");
    },
  );
});
afterEach(() => vi.restoreAllMocks());
function mount() {
  return render(
    <PlaygroundProvider>
      <PlaygroundPage />
      <ConnectionSettings />
    </PlaygroundProvider>,
  );
}
async function configure() {
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "接口配置" })).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  await user.type(
    screen.getByLabelText("API Key", { exact: true }),
    "test-only-key",
  );
  await user.type(
    screen.getByLabelText("可用模型 ID"),
    "team-chat,team-reasoning,team-chat",
  );
  await user.selectOptions(screen.getByLabelText("默认模型"), "team-reasoning");
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  return user;
}
it("saves manual models and a key, reloads redacted metadata, and deletes server configuration", async () => {
  const storageWrite = vi.spyOn(Storage.prototype, "setItem");
  const view = mount();
  const user = await configure();
  expect(playgroundApi.save).toHaveBeenCalledWith(
    expect.objectContaining({
      apiKey: "test-only-key",
      defaultModel: "team-reasoning",
      models: ["team-chat", "team-reasoning"],
    }),
    expect.any(AbortSignal),
  );
  expect(screen.getByLabelText("对话模型")).toHaveValue("team-reasoning");
  expect(storageWrite).not.toHaveBeenCalled();
  expect(streamModelChat).not.toHaveBeenCalled();
  view.unmount();
  mount();
  await waitFor(() =>
    expect(screen.getByText("已加密保存")).toBeInTheDocument(),
  );
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  expect(screen.getByLabelText("API Key", { exact: true })).toHaveValue("");
  await user.click(screen.getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("button", { name: "删除接口配置" }));
  await waitFor(() => expect(screen.getByLabelText("对话模型")).toBeDisabled());
});
it("validates fields and discards cancelled secret drafts", async () => {
  const user = userEvent.setup();
  mount();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "接口配置" })).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  expect(screen.getByRole("alert")).toHaveTextContent("请填写 API Key");
  await user.type(
    screen.getByLabelText("API Key", { exact: true }),
    "test-only-key",
  );
  await user.click(screen.getByRole("button", { name: "显示 API Key" }));
  expect(screen.getByLabelText("API Key", { exact: true })).toHaveAttribute(
    "type",
    "text",
  );
  await user.click(screen.getByRole("button", { name: "取消" }));
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  expect(screen.getByLabelText("API Key", { exact: true })).toHaveValue("");
  expect(screen.getByLabelText("API Key", { exact: true })).toHaveAttribute(
    "type",
    "password",
  );
});
it("sends conversation history, renders streamed text and regenerates without duplicating the user message", async () => {
  mount();
  const user = await configure();
  await user.type(screen.getByLabelText("消息"), "hello");
  await user.click(screen.getByRole("button", { name: "发送消息" }));
  await waitFor(() => expect(screen.getByText("模型回复")).toBeInTheDocument());
  expect(streamModelChat).toHaveBeenCalledWith(
    expect.objectContaining({
      model: "team-reasoning",
      messages: [{ role: "user", content: "hello" }],
    }),
    expect.any(AbortSignal),
    expect.any(Function),
  );
  await user.click(screen.getByRole("button", { name: "重新生成" }));
  await waitFor(() => expect(streamModelChat).toHaveBeenCalledTimes(2));
  expect(screen.getAllByRole("article", { name: "我的消息" })).toHaveLength(1);
});
it("cancels an in-flight model request and marks the partial answer", async () => {
  vi.mocked(streamModelChat).mockImplementation(
    (_input, signal, emit) =>
      new Promise((_resolve, reject) => {
        emit("partial");
        signal.addEventListener("abort", () =>
          reject(new DOMException("Aborted", "AbortError")),
        );
      }),
  );
  mount();
  const user = await configure();
  await user.type(screen.getByLabelText("消息"), "hello");
  await user.click(screen.getByRole("button", { name: "发送消息" }));
  await user.click(await screen.findByRole("button", { name: "停止生成" }));
  await waitFor(() =>
    expect(screen.getByText("已停止生成")).toBeInTheDocument(),
  );
  expect(screen.getByText("partial")).toBeInTheDocument();
});
it("retains the saved profile and reports a failed save without claiming success", async () => {
  mount();
  const user = await configure();
  vi.mocked(playgroundApi.save).mockRejectedValue(new Error("private error"));
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  expect(
    await within(screen.getByRole("dialog")).findByRole("alert"),
  ).toHaveTextContent("配置服务暂时不可用");
  expect(screen.queryByText("private error")).not.toBeInTheDocument();
});
it("normalizes model lists and rejects URLs with credentials, query keys or completion paths", () => {
  expect(normalizeBaseUrl("https://api.anthropic.com/v1/", "anthropic")).toBe(
    "https://api.anthropic.com",
  );
  expect(
    normalizeBaseUrl("https://api.anthropic.com/v1/messages", "anthropic"),
  ).toBeNull();
  expect(parseModels(" alpha\nbeta，alpha,, ")).toEqual(["alpha", "beta"]);
  for (const url of [
    "https://user:secret@models.example/v1",
    "https://models.example/v1?key=x",
    "javascript:alert(1)",
    "https://models.example/v1/chat/completions",
  ])
    expect(normalizeBaseUrl(url)).toBeNull();
});
it("uses the selected Claude protocol for discovery and saved configuration", async () => {
  vi.mocked(playgroundApi.models).mockResolvedValue({
    models: ["claude-test"],
  });
  mount();
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "接口配置" })).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  await user.selectOptions(screen.getByLabelText("接口协议"), "anthropic");
  expect(screen.getByLabelText("API 基础地址")).toHaveValue(
    "https://api.anthropic.com",
  );
  await user.type(
    screen.getByLabelText("API Key", { exact: true }),
    "claude-test-key",
  );
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await user.click(
    await screen.findByRole("checkbox", { name: "claude-test" }),
  );
  expect(playgroundApi.models).toHaveBeenCalledWith(
    expect.objectContaining({
      protocol: "anthropic",
      baseUrl: "https://api.anthropic.com",
    }),
    expect.any(AbortSignal),
  );
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() =>
    expect(playgroundApi.save).toHaveBeenCalledWith(
      expect.objectContaining({
        protocol: "anthropic",
        baseUrl: "https://api.anthropic.com",
        models: ["claude-test"],
      }),
      expect.any(AbortSignal),
    ),
  );
});
it("fetches available models before saving and persists only selected models", async () => {
  vi.mocked(playgroundApi.models).mockResolvedValue({
    models: ["chat-alpha", "chat-beta"],
  });
  mount();
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "接口配置" })).toBeEnabled(),
  );
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  await user.type(
    screen.getByLabelText("API Key", { exact: true }),
    "draft-key",
  );
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await user.click(await screen.findByRole("checkbox", { name: "chat-beta" }));
  expect(playgroundApi.save).not.toHaveBeenCalled();
  expect(screen.getByLabelText("默认模型")).toHaveValue("chat-beta");
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() =>
    expect(playgroundApi.save).toHaveBeenCalledWith(
      expect.objectContaining({ models: ["chat-beta"], apiKey: "draft-key" }),
      expect.any(AbortSignal),
    ),
  );
});
it("keeps manual models when discovery fails and cancels discovery on close", async () => {
  mount();
  const user = await configure();
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  vi.mocked(playgroundApi.models).mockRejectedValueOnce(
    new Error("private upstream failure"),
  );
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("可手动填写");
  expect(screen.getByLabelText("可用模型 ID")).toHaveValue(
    "team-chat\nteam-reasoning",
  );
  let signal: AbortSignal | undefined;
  vi.mocked(playgroundApi.models).mockImplementation((_input, value) => {
    signal = value;
    return new Promise(() => {});
  });
  await user.click(screen.getByRole("button", { name: "获取模型" }));
  await user.click(screen.getByRole("button", { name: "取消" }));
  expect(signal?.aborted).toBe(true);
});

it("omits disabled parameters and transmits explicit zero values with system instructions", async () => {
  mount();
  const user = await configure();
  await user.type(screen.getByLabelText("消息"), "first");
  await user.click(screen.getByRole("button", { name: "发送消息" }));
  await waitFor(() => expect(streamModelChat).toHaveBeenCalledTimes(1));
  expect(vi.mocked(streamModelChat).mock.calls[0][0]).toEqual({
    model: "team-reasoning",
    messages: [{ role: "user", content: "first" }],
  });
  await user.click(screen.getByRole("button", { name: "模型参数" }));
  await user.click(await screen.findByRole("switch", { name: "启用频率惩罚" }));
  await user.click(screen.getByRole("switch", { name: "启用最大 Tokens" }));
  await user.clear(screen.getByLabelText("最大 Tokens", { exact: true }));
  await user.type(
    screen.getByLabelText("最大 Tokens", { exact: true }),
    "1024",
  );
  await user.type(screen.getByLabelText("系统提示词"), "Answer briefly");
  await user.click(screen.getByRole("button", { name: "关闭参数" }));
  await user.type(screen.getByLabelText("消息"), "second");
  await user.click(screen.getByRole("button", { name: "发送消息" }));
  await waitFor(() => expect(streamModelChat).toHaveBeenCalledTimes(2));
  const payload = vi.mocked(streamModelChat).mock.calls[1][0];
  expect(payload).toMatchObject({ frequency_penalty: 0, max_tokens: 1024 });
  expect(payload.messages[0]).toEqual({
    role: "system",
    content: "Answer briefly",
  });
  expect(payload).not.toHaveProperty("temperature");
  await user.click(screen.getByRole("button", { name: "模型参数" }));
  await user.click(await screen.findByRole("button", { name: "恢复默认参数" }));
  expect(
    screen.getByRole("switch", { name: "启用频率惩罚" }),
  ).not.toBeChecked();
  expect(screen.getByLabelText("系统提示词")).toHaveValue("");
});

it("uses only one sampling parameter and hides OpenAI penalties for Claude", async () => {
  saved = {
    protocol: "anthropic",
    baseUrl: "https://api.anthropic.com",
    models: ["claude-test"],
    defaultModel: "claude-test",
    hasKey: true,
    version: 1,
  };
  mount();
  const user = userEvent.setup();
  await waitFor(() =>
    expect(screen.getByLabelText("对话模型")).toHaveValue("claude-test"),
  );
  await user.click(screen.getByRole("button", { name: "模型参数" }));
  await user.click(await screen.findByRole("switch", { name: "启用温度" }));
  expect(screen.getByLabelText("温度", { exact: true })).toHaveAttribute(
    "max",
    "1",
  );
  await user.click(screen.getByRole("switch", { name: "启用Top P" }));
  expect(screen.getByRole("switch", { name: "启用温度" })).not.toBeChecked();
  expect(
    screen.queryByRole("switch", { name: "启用频率惩罚" }),
  ).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "关闭参数" }));
  await user.type(screen.getByLabelText("消息"), "hello");
  await user.click(screen.getByRole("button", { name: "发送消息" }));
  await waitFor(() => expect(streamModelChat).toHaveBeenCalledTimes(1));
  expect(vi.mocked(streamModelChat).mock.calls[0][0]).toEqual({
    model: "claude-test",
    messages: [{ role: "user", content: "hello" }],
    top_p: 1,
  });
});

it("drops OpenAI-only parameters when changing the saved connection to Claude", async () => {
  mount();
  const user = await configure();
  await user.click(screen.getByRole("button", { name: "模型参数" }));
  await user.click(await screen.findByRole("switch", { name: "启用频率惩罚" }));
  await user.click(screen.getByRole("switch", { name: "启用存在惩罚" }));
  await user.click(screen.getByRole("switch", { name: "启用温度" }));
  await user.clear(screen.getByLabelText("温度", { exact: true }));
  await user.type(screen.getByLabelText("温度", { exact: true }), "2");
  await user.click(screen.getByRole("button", { name: "关闭参数" }));
  await user.click(screen.getByRole("button", { name: "接口配置" }));
  await user.selectOptions(screen.getByLabelText("接口协议"), "anthropic");
  await user.type(
    screen.getByLabelText("API Key", { exact: true }),
    "claude-test-key",
  );
  await user.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
  await user.type(screen.getByLabelText("消息"), "hello");
  await user.click(screen.getByRole("button", { name: "发送消息" }));
  await waitFor(() => expect(streamModelChat).toHaveBeenCalledTimes(1));
  expect(vi.mocked(streamModelChat).mock.calls[0][0]).toEqual({
    model: "team-reasoning",
    messages: [{ role: "user", content: "hello" }],
    temperature: 1,
  });
});
