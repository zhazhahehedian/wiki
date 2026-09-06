import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { CapabilityEditor } from "./capability-editor";
import { saveCapability, type CapabilityDetail } from "@/lib/api/registry";
import { APIError } from "@/lib/api/client";
const { push } = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push }) }));
vi.mock("@/lib/api/registry", () => ({
  saveCapability: vi.fn(),
  getCapability: vi.fn(),
}));
const detail: CapabilityDetail = {
  id: "id",
  slug: "search",
  type: "mcp",
  name: "Search",
  description: "Search team docs",
  owner_open_id: "ou_owner",
  department: "研发",
  status: "draft",
  visibility: "allowlist",
  allowlist: ["ou_friend"],
  revision: 3,
  draft_version_id: "v1",
  created_at: "2026-09-06",
  updated_at: "2026-09-06",
  versions: [
    {
      id: "v1",
      version: "1.0.0",
      changelog: "",
      created_by: "ou_owner",
      created_at: "2026-09-06",
      mcp_endpoint: "https://example.test/mcp",
      mcp_transport: "streamable-http",
      mcp_auth_scheme: "bearer",
      tools: [],
      has_bundle: false,
    },
  ],
};
function mount(d?: CapabilityDetail) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <CapabilityEditor detail={d} />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.mocked(saveCapability).mockReset();
  push.mockReset();
});
it("saves the loaded revision and clears hidden allowlist when visibility changes", async () => {
  vi.mocked(saveCapability).mockResolvedValue({ ...detail, revision: 4 });
  const user = userEvent.setup();
  mount(detail);
  expect(screen.getByLabelText(/唯一标识/)).toBeDisabled();
  await user.selectOptions(screen.getByLabelText("上线后可见范围"), "org");
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  await waitFor(() =>
    expect(saveCapability).toHaveBeenCalledWith(
      expect.objectContaining({
        revision: 3,
        visibility: "org",
        allowlist: [],
        version: "1.0.0",
      }),
      [],
      "search",
    ),
  );
  expect(push).toHaveBeenCalledWith("/hub/registry/search");
});
it("keeps unsaved content on conflicts", async () => {
  vi.mocked(saveCapability).mockRejectedValue(
    new APIError(409, "capability_conflict", "草稿已更新"),
  );
  const user = userEvent.setup();
  mount(detail);
  await user.clear(screen.getByLabelText("名称"));
  await user.type(screen.getByLabelText("名称"), "Unsaved change");
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("草稿已更新");
  expect(screen.getByLabelText("名称")).toHaveValue("Unsaved change");
  expect(
    screen.getByRole("button", { name: "重新加载最新草稿" }),
  ).toBeInTheDocument();
  expect(push).not.toHaveBeenCalled();
});
it("rejects invalid tool schemas before submission", async () => {
  const user = userEvent.setup();
  mount(detail);
  await user.clear(screen.getByLabelText(/工具清单 JSON/));
  await user.paste('[{"name":"search","description":"bad","inputSchema":[]} ]');
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "工具清单需要 JSON 数组",
  );
  expect(saveCapability).not.toHaveBeenCalled();
});
it("requires a complete Skill bundle for new versions", async () => {
  const skill: CapabilityDetail = {
    ...detail,
    type: "skill",
    versions: [
      { ...detail.versions[0], has_bundle: true, mcp_endpoint: undefined },
    ],
  };
  const user = userEvent.setup();
  mount(skill);
  await user.clear(screen.getByLabelText(/版本号/));
  await user.type(screen.getByLabelText(/版本号/), "2.0.0");
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "新版本需要上传完整文件包",
  );
  expect(saveCapability).not.toHaveBeenCalled();
  const file = new File(["# Instructions"], "SKILL.md", {
    type: "text/markdown",
  });
  await user.upload(screen.getByLabelText(/SKILL.md 与附件/), file);
  vi.mocked(saveCapability).mockResolvedValue(skill);
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  await waitFor(() =>
    expect(saveCapability).toHaveBeenCalledWith(
      expect.objectContaining({
        type: "skill",
        tools: [],
        mcp_endpoint: "",
        mcp_transport: "",
        mcp_auth_scheme: "",
      }),
      [file],
      "search",
    ),
  );
});

it("keeps field labels stable when editing prefilled multiline content", () => {
  mount({
    ...detail,
    versions: [{ ...detail.versions[0], changelog: "Existing release notes" }],
  });
  expect(screen.getByRole("textbox", { name: "版本说明" })).toHaveValue(
    "Existing release notes",
  );
  expect(screen.getByRole("textbox", { name: "能力描述" })).toHaveValue(
    detail.description,
  );
});
