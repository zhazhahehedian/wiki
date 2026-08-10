import { Suspense } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { Sidebar, SidebarContent, SidebarProvider } from "@/components/ui/sidebar";
import KBDocsPage from "./page";

vi.mock("@/lib/hooks/use-docs", () => ({
  useDocsByKB: () => ({ data: { items: [], total: 0 }, isLoading: false, isError: false }),
}));
vi.mock("@/lib/hooks/use-kbs", () => ({
  useKbs: () => ({ data: { items: [{ id: "kb-1", name: "运维知识库" }] } }),
}));
vi.mock("@/components/docs/doc-uploader", () => ({
  DocUploader: () => <button>上传文件</button>,
}));
vi.mock("@/components/docs/feishu-import-dialog", () => ({
  FeishuImportDialog: () => <button>从飞书导入</button>,
}));

function setViewport(width: number) {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: width });
  vi.stubGlobal("matchMedia", vi.fn().mockImplementation((query: string) => ({
    matches: width < 768,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })));
}

function renderPage(sidebarAction = vi.fn()) {
  const params = Promise.resolve({ kbId: "kb-1" }) as Promise<{ kbId: string }> & {
    status: "fulfilled";
    value: { kbId: string };
  };
  params.status = "fulfilled";
  params.value = { kbId: "kb-1" };
  return render(
    <SidebarProvider>
      <Sidebar>
        <SidebarContent>
          <button onClick={sidebarAction}>Mobile KB action</button>
        </SidebarContent>
      </Sidebar>
      <Suspense fallback={<p>loading</p>}>
        <KBDocsPage params={params} />
      </Suspense>
    </SidebarProvider>,
  );
}

describe("KBDocsPage import actions", () => {
  beforeEach(() => setViewport(1024));
  afterEach(() => vi.unstubAllGlobals());

  it("keeps file upload and Feishu import available together", async () => {
    renderPage();

    expect(await screen.findByRole("button", { name: "上传文件" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "从飞书导入" })).toBeInTheDocument();
  });

  it("opens and operates the real mobile navigation sheet from the docs trigger", async () => {
    setViewport(375);
    const user = userEvent.setup();
    const sidebarAction = vi.fn();
    const { container } = renderPage(sidebarAction);

    await waitFor(() => expect(container.querySelector("[data-slot=sidebar-gap]")).not.toBeInTheDocument());
    const trigger = screen.getByRole("button", { name: "切换侧栏" });
    trigger.focus();
    await user.keyboard("{Enter}");

    expect(await screen.findByRole("dialog")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Mobile KB action" }));
    expect(sidebarAction).toHaveBeenCalledOnce();

    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });
});
