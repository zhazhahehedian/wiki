import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { RegistryOverview } from "./registry-overview";
import { listCatalog, listCapabilities } from "@/lib/api/registry";
import { APIError } from "@/lib/api/client";
vi.mock("@/lib/api/registry", () => ({
  listCatalog: vi.fn(),
  listCapabilities: vi.fn(),
}));
const item = {
  id: "one",
  slug: "search",
  type: "mcp" as const,
  name: "团队搜索",
  description: "查找内部资料",
  owner_open_id: "ou_owner",
  department: "研发",
  status: "draft" as const,
  visibility: "org" as const,
  revision: 1,
  created_at: "2026-09-06T00:00:00Z",
  updated_at: "2026-09-06T00:00:00Z",
};
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <RegistryOverview />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.mocked(listCatalog).mockReset();
  vi.mocked(listCapabilities).mockReset();
  vi.mocked(listCatalog).mockResolvedValue({
    items: [item],
    has_more: false,
  });
});
it("shows saved capabilities in cards and table with navigation", async () => {
  const user = userEvent.setup();
  mount();
  expect(await screen.findByRole("link", { name: /团队搜索/ })).toHaveAttribute(
    "href",
    "/hub/registry/search",
  );
  expect(screen.getByRole("link", { name: "发布能力" })).toHaveAttribute(
    "href",
    "/hub/registry/new",
  );
  await user.click(screen.getByRole("button", { name: "表格视图" }));
  expect(screen.getByRole("table", { name: "能力列表" })).toBeInTheDocument();
  expect(screen.getAllByRole("columnheader")).toHaveLength(5);
  await user.click(screen.getByRole("button", { name: "卡片视图" }));
  expect(screen.queryByRole("table")).not.toBeInTheDocument();
});
it("sends filters and resets pagination", async () => {
  vi.mocked(listCatalog).mockResolvedValue({
    items: [item],
    has_more: true,
  });
  const user = userEvent.setup();
  mount();
  await screen.findByText("团队搜索");
  await user.click(screen.getByRole("button", { name: "下一页" }));
  await waitFor(() =>
    expect(listCatalog).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 24 }),
      expect.any(AbortSignal),
    ),
  );
  await user.click(screen.getByRole("button", { name: "MCP 服务" }));
  await user.type(screen.getByRole("textbox", { name: "搜索能力" }), "资料");
  await user.type(screen.getByRole("textbox", { name: "筛选部门" }), "研发");
  await user.click(screen.getByRole("button", { name: "搜索" }));
  await waitFor(() =>
    expect(listCatalog).toHaveBeenLastCalledWith(
      expect.objectContaining({
        type: "mcp",
        q: "资料",
        department: "研发",
        offset: 0,
      }),
      expect.any(AbortSignal),
    ),
  );
});
it("distinguishes errors from an empty directory and supports retry", async () => {
  vi.mocked(listCatalog).mockRejectedValueOnce(
    new APIError(503, "unavailable", "目录暂时不可用"),
  );
  const user = userEvent.setup();
  mount();
  expect(await screen.findByRole("alert")).toHaveTextContent("目录暂时不可用");
  expect(screen.queryByText("从第一个能力开始")).not.toBeInTheDocument();
  vi.mocked(listCatalog).mockResolvedValue({ items: [], has_more: false });
  await user.click(screen.getByRole("button", { name: "重新加载" }));
  expect(await screen.findByText("从第一个能力开始")).toBeInTheDocument();
});

it("uses separate queries for the published catalog and Owner drafts", async () => {
  vi.mocked(listCatalog).mockResolvedValue({
    items: [{ ...item, name: "上线内容", status: "published" }],
    has_more: false,
  });
  vi.mocked(listCapabilities).mockResolvedValue({
    items: [{ ...item, name: "待审草稿" }],
    has_more: false,
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={client}>
      <RegistryOverview />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("上线内容")).toBeInTheDocument();
  view.rerender(
    <QueryClientProvider client={client}>
      <RegistryOverview mine />
    </QueryClientProvider>,
  );
  expect(await screen.findByText("待审草稿")).toBeInTheDocument();
  expect(screen.queryByText("上线内容")).not.toBeInTheDocument();
  expect(listCapabilities).toHaveBeenCalled();
});
