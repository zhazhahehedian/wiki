import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { SkillBuilderPage } from "./skill-builder-page";
import {
  generateSkill,
  saveGeneratedSkill,
  type BuilderReply,
  type SkillDraft,
} from "@/lib/api/skill-builder";
import type { CapabilityDetail } from "@/lib/api/registry";
import { APIError } from "@/lib/api/client";
const { push, connectionState } = vi.hoisted(() => ({
  push: vi.fn(),
  connectionState: {
    connection: null as null | { models: string[]; defaultModel: string },
    loading: false,
    error: "",
    reload: vi.fn(),
  },
}));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("@/components/playground/playground-provider", () => ({
  useModelConnection: () => connectionState,
}));
vi.mock("@/lib/api/skill-builder", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/skill-builder")>()),
  generateSkill: vi.fn(),
  saveGeneratedSkill: vi.fn(),
}));
const draft: SkillDraft = {
  slug: "weekly-report",
  name: "技术周报",
  description: "整理团队周报",
  instructions: "# 周报\n按模板输出。",
  files: [{ path: "references/template.md", content: "## 进展" }],
};
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui = (
    <QueryClientProvider client={client}>
      <SkillBuilderPage />
    </QueryClientProvider>
  );
  return { ...render(ui), ui };
}
async function send(text = "帮我写周报") {
  fireEvent.change(screen.getByLabelText("你的需求或修改意见"), {
    target: { value: text },
  });
  await userEvent.click(
    screen.getByRole("button", { name: /发送需求|继续修改/ }),
  );
}
async function generate() {
  vi.mocked(generateSkill).mockResolvedValue({
    message: "已整理草案",
    draft: structuredClone(draft),
  });
  await send();
  await screen.findByLabelText("Skill 名称");
}
beforeEach(() => {
  vi.mocked(generateSkill).mockReset();
  vi.mocked(saveGeneratedSkill).mockReset();
  push.mockReset();
  connectionState.connection = {
    models: ["fixture-model"],
    defaultModel: "fixture-model",
  };
  connectionState.loading = false;
  connectionState.error = "";
});
it("requires a configured model before sending", async () => {
  connectionState.connection = null;
  mount();
  fireEvent.change(screen.getByLabelText("你的需求或修改意见"), {
    target: { value: "周报" },
  });
  expect(screen.getByRole("button", { name: "发送需求" })).toBeDisabled();
  expect(screen.getByRole("link", { name: "设置 → 模型接口" })).toHaveAttribute(
    "href",
    "/hub/settings",
  );
  expect(generateSkill).not.toHaveBeenCalled();
});
it("uses the Nuwa mode, carries clarification history and edited draft, previews files and saves only on confirmation", async () => {
  const user = userEvent.setup();
  mount();
  await user.selectOptions(screen.getByLabelText("创建方式"), "perspective");
  vi.mocked(generateSkill).mockResolvedValueOnce({
    message: "周报给谁看？",
    draft: null,
  });
  await send();
  await screen.findByText("周报给谁看？");
  expect(saveGeneratedSkill).not.toHaveBeenCalled();
  expect(screen.getByLabelText("创建方式")).toBeDisabled();
  vi.mocked(generateSkill).mockResolvedValueOnce({ message: "已整理", draft });
  await send("给研发负责人看");
  await screen.findByLabelText("Skill 名称");
  expect(generateSkill).toHaveBeenLastCalledWith(
    expect.objectContaining({
      mode: "perspective",
      messages: [
        { role: "user", content: "帮我写周报" },
        { role: "assistant", content: "周报给谁看？" },
        { role: "user", content: "给研发负责人看" },
      ],
    }),
    expect.any(AbortSignal),
  );
  fireEvent.change(screen.getByLabelText("Skill 名称"), {
    target: { value: "我的周报" },
  });
  vi.mocked(generateSkill).mockResolvedValueOnce({
    message: "还需要哪些内容？",
    draft: null,
  });
  await send("增加风险章节");
  await screen.findByText("还需要哪些内容？");
  expect(generateSkill).toHaveBeenLastCalledWith(
    expect.objectContaining({
      draft: expect.objectContaining({ name: "我的周报" }),
    }),
    expect.any(AbortSignal),
  );
  expect(screen.getByLabelText("Skill 名称")).toHaveValue("我的周报");
  await user.click(screen.getByRole("button", { name: "文件预览" }));
  expect(screen.getByText(/name: "weekly-report"/)).toBeInTheDocument();
  await user.selectOptions(
    screen.getByLabelText("预览文件"),
    "references/template.md",
  );
  expect(screen.getByText("## 进展")).toBeInTheDocument();
  vi.mocked(saveGeneratedSkill).mockResolvedValue({
    slug: draft.slug,
  } as CapabilityDetail);
  await user.click(screen.getByRole("button", { name: "保存到注册中心" }));
  await waitFor(() =>
    expect(push).toHaveBeenCalledWith("/hub/registry/weekly-report"),
  );
  expect(saveGeneratedSkill).toHaveBeenCalledWith(
    { ...draft, name: "我的周报" },
    expect.any(AbortSignal),
  );
});
it("preserves edits and input when generation or save fails", async () => {
  mount();
  await generate();
  fireEvent.change(screen.getByLabelText("SKILL.md 正文"), {
    target: { value: "我的未保存修改" },
  });
  vi.mocked(generateSkill).mockRejectedValue(
    new APIError(502, "skill_output_invalid", "生成失败"),
  );
  await send("补充风险");
  expect(await screen.findByRole("alert")).toHaveTextContent("生成失败");
  expect(screen.getByLabelText("SKILL.md 正文")).toHaveValue("我的未保存修改");
  expect(screen.getByLabelText("你的需求或修改意见")).toHaveValue("补充风险");
  vi.mocked(saveGeneratedSkill).mockRejectedValue(
    new APIError(409, "capability_conflict", "标识已存在"),
  );
  await userEvent.click(screen.getByRole("button", { name: "保存到注册中心" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("标识已存在");
  expect(screen.getByLabelText("SKILL.md 正文")).toHaveValue("我的未保存修改");
  expect(push).not.toHaveBeenCalled();
});
it("aborts generation and ignores late results while retaining the previous draft", async () => {
  mount();
  await generate();
  let resolve!: (reply: BuilderReply) => void;
  vi.mocked(generateSkill).mockImplementation(
    () =>
      new Promise((r) => {
        resolve = r;
      }),
  );
  await send("增加风险");
  const signal = vi.mocked(generateSkill).mock.lastCall![1];
  await userEvent.click(screen.getByRole("button", { name: "停止生成" }));
  expect(signal.aborted).toBe(true);
  await act(async () =>
    resolve({ message: "过期结果", draft: { ...draft, name: "不应出现" } }),
  );
  expect(screen.getByLabelText("Skill 名称")).toHaveValue(draft.name);
  expect(screen.queryByText("过期结果")).not.toBeInTheDocument();
  expect(screen.getByLabelText("你的需求或修改意见")).toHaveValue("增加风险");
});
it("cancels requests on configuration changes and unmount", async () => {
  const view = mount();
  vi.mocked(generateSkill).mockImplementation(() => new Promise(() => {}));
  await send();
  const signal = vi.mocked(generateSkill).mock.lastCall![1];
  connectionState.connection = {
    models: ["new-model"],
    defaultModel: "new-model",
  };
  view.rerender(
    <QueryClientProvider client={new QueryClient()}>
      <SkillBuilderPage />
    </QueryClientProvider>,
  );
  expect(signal.aborted).toBe(true);
  expect(await screen.findByText(/模型配置已变化/)).toBeInTheDocument();
  await send("使用新模型");
  const next = vi.mocked(generateSkill).mock.lastCall![1];
  view.unmount();
  expect(next.aborted).toBe(true);
});
it("bounds imported materials and supports editing text attachments", async () => {
  const user = userEvent.setup();
  mount();
  await generate();
  fireEvent.change(screen.getByLabelText("导入文本素材"), {
    target: { files: [new File(["x".repeat(49 * 1024)], "large.md")] },
  });
  expect(await screen.findByText(/请选择不超过 48 KiB/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "添加附件" }));
  expect(screen.getByLabelText("附件 2 路径")).toHaveValue(
    "references/note-1.md",
  );
  fireEvent.change(screen.getByLabelText("附件 2 内容"), {
    target: { value: "自定义说明" },
  });
  await user.click(screen.getByRole("button", { name: "移除附件 1" }));
  expect(screen.getByLabelText("附件 1 内容")).toHaveValue("自定义说明");
});
