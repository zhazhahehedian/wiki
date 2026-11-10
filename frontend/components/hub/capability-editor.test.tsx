import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { CapabilityEditor } from "./capability-editor";
import { saveCapability, type CapabilityDetail } from "@/lib/api/registry";
import { APIError } from "@/lib/api/client";
import { forgetBundleUpload, uploadBundle, type BundleUpload } from "@/lib/api/bundle-upload";
const { push } = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("@/lib/api/bundle-upload", async (original) => ({
  ...(await original<typeof import("@/lib/api/bundle-upload")>()),
  uploadBundle: vi.fn(),
  forgetBundleUpload: vi.fn(),
  discardBundleUpload: vi.fn(),
}));
const completed: BundleUpload = {
  id: "upload-1",
  file_name: "server.zip",
  size: 11,
  part_size: 8 << 20,
  part_count: 1,
  received_parts: [],
  completed: true,
  bundle_sha256: "ab".repeat(32),
  expires_at: "2026-11-07T00:00:00Z",
};
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
  vi.mocked(uploadBundle).mockReset();
  vi.mocked(forgetBundleUpload).mockReset();
  push.mockReset();
});
it("saves the loaded revision and clears hidden allowlist when visibility changes", async () => {
  vi.mocked(saveCapability).mockResolvedValue({ ...detail, revision: 4 });
  const user = userEvent.setup();
  mount(detail);
  expect(screen.getByLabelText(/唯一标识/)).toBeDisabled();
  await user.click(screen.getByRole("combobox", { name: "上线后可见范围" }));
  await user.click(await screen.findByRole("option", { name: "全公司" }));
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

it("uploads an MCP package in parts and saves it by upload ID without an endpoint", async () => {
  const user = userEvent.setup();
  vi.mocked(saveCapability).mockResolvedValue(detail);
  let finish: (u: BundleUpload) => void = () => {};
  vi.mocked(uploadBundle).mockImplementation((_file, onProgress) => {
    onProgress({ uploadedBytes: 5, totalBytes: 11, phase: "uploading" });
    return new Promise((resolve) => (finish = resolve));
  });
  mount(detail);
  await user.click(screen.getByRole("combobox", { name: "传输协议" }));
  await user.click(await screen.findByRole("option", { name: "stdio · 本地进程" }));
  expect(screen.queryByLabelText(/Endpoint/)).not.toBeInTheDocument();
  const zip = new File(["zip fixture"], "server.zip", { type: "application/zip" });
  await user.upload(screen.getByLabelText("MCP 程序包（ZIP）"), zip);
  expect(uploadBundle).toHaveBeenCalledWith(zip, expect.any(Function), expect.any(AbortSignal));
  expect(screen.getByRole("progressbar", { name: "程序包上传进度" })).toHaveAttribute("aria-valuenow", "45");
  // Saving waits for the upload to finish.
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("尚未上传完成");
  expect(saveCapability).not.toHaveBeenCalled();
  finish(completed);
  expect(await screen.findByText(/已上传并校验/)).toBeInTheDocument();
  expect(screen.queryByText(/尚未上传完成/)).not.toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  await waitFor(() => expect(saveCapability).toHaveBeenCalledWith(expect.objectContaining({mcp_endpoint:"", mcp_transport:"stdio", mcp_auth_scheme:"none", bundle_upload_id:"upload-1"}), [], "search"));
  expect(forgetBundleUpload).toHaveBeenCalledWith(zip);
});

it("pauses and resumes an MCP package upload", async () => {
  const user = userEvent.setup();
  vi.mocked(uploadBundle).mockImplementation((_file, onProgress, signal) => {
    onProgress({ uploadedBytes: 3, totalBytes: 11, phase: "uploading" });
    return new Promise((_, reject) => signal.addEventListener("abort", () => reject(signal.reason)));
  });
  mount(detail);
  const zip = new File(["zip fixture"], "server.zip", { type: "application/zip" });
  await user.upload(screen.getByLabelText("MCP 程序包（ZIP）"), zip);
  await user.click(screen.getByRole("button", { name: "暂停" }));
  expect(await screen.findByText(/已暂停/)).toBeInTheDocument();
  vi.mocked(uploadBundle).mockResolvedValue(completed);
  await user.click(screen.getByRole("button", { name: "继续上传" }));
  expect(await screen.findByText(/已上传并校验/)).toBeInTheDocument();
  expect(uploadBundle).toHaveBeenCalledTimes(2);
});

it("retains an existing MCP draft package but requires a package for the next version", async () => {
  const user = userEvent.setup();
  vi.mocked(saveCapability).mockResolvedValue(detail);
  mount({...detail, versions:[{...detail.versions[0], has_bundle:true, mcp_endpoint:"", mcp_transport:"stdio", mcp_auth_scheme:"none"}]});
  expect(screen.getByText(/当前草稿已有程序包/)).toBeInTheDocument();
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  await waitFor(() => expect(saveCapability).toHaveBeenCalledTimes(1));
  await user.clear(screen.getByLabelText(/版本号/));
  await user.type(screen.getByLabelText(/版本号/), "2.0.0");
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("新版本需要重新上传程序包");
  expect(saveCapability).toHaveBeenCalledTimes(1);
});

it("rejects MCP ZIPs above the limit before saving", async () => {
  const user = userEvent.setup();
  mount(detail);
  const zip = new File(["small"], "large.zip", {type:"application/zip"});
  Object.defineProperty(zip, "size", {value:500*1024*1024+1});
  await user.upload(screen.getByLabelText("MCP 程序包（ZIP）"), zip);
  expect(screen.getByRole("alert")).toHaveTextContent("500 MiB");
  await user.click(screen.getByRole("button", { name: "保存草稿" }));
  await waitFor(() => expect(screen.getAllByRole("alert")).toHaveLength(2));
  expect(uploadBundle).not.toHaveBeenCalled();
  expect(saveCapability).not.toHaveBeenCalled();
});
