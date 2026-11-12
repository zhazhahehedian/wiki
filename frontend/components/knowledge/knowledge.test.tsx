import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ConfirmDialogHost } from "@/components/ui/confirm-dialog";
import { KnowledgeApplications } from "./knowledge-applications";
import { KnowledgeBases } from "./knowledge-bases";
import { KnowledgeDocuments } from "./knowledge-documents";
import { KnowledgeSearch } from "./knowledge-search";
import { VersionPreview } from "./version-preview";
import { knowledge } from "@/lib/api/knowledge";
vi.mock("@/lib/api/knowledge", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/knowledge")>()),
  knowledge: {
    bases: vi.fn(),
    saveBase: vi.fn(),
    documents: vi.fn(),
    preview: vi.fn(),
    search: vi.fn(),
    upload: vi.fn(),
    retry: vi.fn(),
    deleteDocument: vi.fn(),
    applications: vi.fn(),
    saveApplication: vi.fn(),
    audit: vi.fn(),
  },
}));
vi.mock("@/lib/api/registry", () => ({
  getGovernanceIdentity: vi.fn().mockResolvedValue({ is_admin: false }),
}));
function mount(node: React.ReactNode) {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      {node}
    </QueryClientProvider>,
  );
}
const kb = {
  id: "kb",
  name: "财务制度",
  description: "资料",
  visibility: "private",
  department: "",
  members: [],
  manage: true,
  model: "fixture",
  dim: 4,
};
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(knowledge.bases).mockResolvedValue([kb]);
});
describe("knowledge pages", () => {
  it("creates a private knowledge base through the server", async () => {
    vi.mocked(knowledge.saveBase).mockResolvedValue(kb);
    mount(<KnowledgeBases />);
    fireEvent.click(screen.getByRole("button", { name: "创建知识库" }));
    fireEvent.change(screen.getByLabelText("名称"), {
      target: { value: "财务制度" },
    });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(knowledge.saveBase).toHaveBeenCalledWith("", {
        name: "财务制度",
        description: "",
        visibility: "private",
      }),
    );
  });
  it("lets a base manager grant their own base and only remove other grants", async () => {
    vi.mocked(knowledge.bases).mockResolvedValue([
      kb,
      { ...kb, id: "other", name: "人事制度", manage: false },
    ]);
    vi.mocked(knowledge.applications).mockResolvedValue([
      {
        id: "app",
        name: "报销助手",
        enabled: true,
        knowledge_base_ids: ["other", "gone"],
        credential_prefix: "",
      },
    ]);
    vi.mocked(knowledge.audit).mockResolvedValue([]);
    vi.mocked(knowledge.saveApplication).mockImplementation(async (id, body) => ({ ...body, id, credential_prefix: "" }));
    mount(<KnowledgeApplications />);
    fireEvent.click(
      await screen.findByRole("button", { name: "更多操作：报销助手" }),
    );
    fireEvent.click(
      await screen.findByRole("menuitem", { name: "编辑授权 / 停用" }),
    );
    const own = await screen.findByRole("checkbox", { name: "财务制度" });
    expect(own).not.toBeChecked();
    fireEvent.click(own);
    const foreign = screen.getByRole("checkbox", { name: /人事制度（非你管理或已删除/ });
    expect(foreign).toBeChecked();
    fireEvent.click(foreign);
    fireEvent.click(screen.getByRole("checkbox", { name: /gone（非你管理或已删除/ }));
    expect(screen.queryByText(/非你管理或已删除/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    await waitFor(() =>
      expect(knowledge.saveApplication).toHaveBeenCalledWith("app", {
        name: "报销助手",
        enabled: true,
        knowledge_base_ids: ["kb"],
      }),
    );
  });
  it("keeps the old retrievable version visible after an update fails", async () => {
    vi.mocked(knowledge.documents).mockResolvedValue([
      {
        id: "d",
        title: "制度.pdf",
        status: "failed",
        error: "parse_quality_blocked",
        active_version_id: "old",
        pending_version_id: "bad",
      },
    ]);
    mount(<KnowledgeDocuments id="kb" />);
    expect(await screen.findByText(/更新失败，旧版仍可用/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "预览" })).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "重试索引" }),
    ).toBeInTheDocument();
  });
  it("deletes a document only after the confirm dialog is accepted", async () => {
    vi.mocked(knowledge.documents).mockResolvedValue([
      {
        id: "d",
        title: "制度.pdf",
        status: "ready",
        error: "",
        active_version_id: "v1",
        pending_version_id: "",
      },
    ]);
    vi.mocked(knowledge.deleteDocument).mockResolvedValue(undefined);
    mount(
      <>
        <KnowledgeDocuments id="kb" />
        <ConfirmDialogHost />
      </>,
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "更多操作：制度.pdf" }),
    );
    fireEvent.click(await screen.findByRole("menuitem", { name: "删除文档" }));
    const dialog = await screen.findByRole("alertdialog");
    expect(dialog).toHaveTextContent("删除“制度.pdf”？");
    fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));
    await waitFor(() =>
      expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument(),
    );
    expect(knowledge.deleteDocument).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "更多操作：制度.pdf" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "删除文档" }));
    fireEvent.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", {
        name: "删除",
      }),
    );
    await waitFor(() =>
      expect(knowledge.deleteDocument).toHaveBeenCalledWith("d"),
    );
  });
  it("shows blocking page diagnostics with source positions", async () => {
    vi.mocked(knowledge.preview).mockResolvedValue({
      id: "v",
      title: "制度.pdf",
      status: "failed",
      error: "parse_quality_blocked",
      model: "fixture",
      dim: 4,
      checksum: "sum",
      preview: {
        diagnostics: [
          {
            code: "page_failed",
            message: "页面文字提取失败",
            page: 2,
            blocking: true,
          },
        ],
        blocks: [{ content: "可提取正文", locator: { page: 1 } }],
      },
    });
    mount(<VersionPreview id="v" />);
    expect(await screen.findByText(/阻止索引：第 2 页/)).toBeInTheDocument();
    expect(screen.getByText("第 1 页")).toBeInTheDocument();
  });
  it("performs user search and reports empty matches without inventing an answer", async () => {
    vi.mocked(knowledge.search).mockResolvedValue({
      request_id: "r",
      matches: [],
      retrieval: {strategy: "hybrid_rrf", rewrite: "fallback", rerank: "fallback", warnings: ["rewrite_unavailable_or_invalid"]},
    });
    mount(<KnowledgeSearch />);
    fireEvent.click(await screen.findByLabelText("财务制度"));
    fireEvent.change(screen.getByLabelText("业务问题"), {
      target: { value: "报销材料" },
    });
    fireEvent.click(screen.getByRole("button", { name: "检索" }));
    expect(await screen.findByText(/没有匹配资料/)).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("部分检索优化暂不可用");
    expect(knowledge.search).toHaveBeenCalledWith(
      ["kb"],
      "报销材料",
      expect.any(AbortSignal),
    );
  });
});
