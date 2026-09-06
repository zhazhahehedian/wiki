import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { GovernanceControls } from "./governance-controls";
import { ReviewsPage } from "./governance-pages";
import {
  actOnCapability,
  getGovernanceIdentity,
  listReviews,
  type CapabilityDetail,
} from "@/lib/api/registry";
import { APIError } from "@/lib/api/client";
const mocks = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));
vi.mock("@/lib/api/registry", () => ({
  actOnCapability: vi.fn(),
  getGovernanceIdentity: vi.fn(),
  listReviews: vi.fn(),
  listAudit: vi.fn(),
  listProfiles: vi.fn(),
  setTrustedDepartment: vi.fn(),
}));
const item: CapabilityDetail = {
  id: "cap",
  slug: "search",
  type: "mcp",
  name: "Search",
  description: "Description",
  owner_open_id: "ou_owner",
  department: "",
  status: "in_review",
  visibility: "org",
  current_version_id: "old",
  draft_version_id: "pending",
  revision: 7,
  created_at: "2026-09-06",
  updated_at: "2026-09-06",
  versions: [],
  allowlist: [],
  is_admin: true,
  is_live: true,
};
function mount(children: React.ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(<QueryClientProvider client={client}>{children}</QueryClientProvider>);
  return client;
}
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(actOnCapability).mockResolvedValue({
    ...item,
    status: "published",
    revision: 8,
  });
});
it("binds approval to the pending version even while an old version is live", async () => {
  mount(<GovernanceControls item={item} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "通过并上线" }));
  await user.click(screen.getByRole("button", { name: "确认通过并上线" }));
  await waitFor(() =>
    expect(actOnCapability).toHaveBeenCalledWith("search", "approve", {
      revision: 7,
      version_id: "pending",
      reason: "",
    }),
  );
});
it("requires a rejection reason and keeps it on a conflict", async () => {
  vi.mocked(actOnCapability).mockRejectedValue(
    new APIError(409, "capability_conflict", "版本已变化"),
  );
  mount(<GovernanceControls item={item} />);
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "驳回" }));
  expect(screen.getByRole("button", { name: "确认驳回" })).toBeDisabled();
  await user.type(
    screen.getByRole("textbox", { name: "操作说明（必填）" }),
    "请补充工具说明",
  );
  await user.click(screen.getByRole("button", { name: "确认驳回" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("版本已变化");
  expect(screen.getByRole("textbox")).toHaveValue("请补充工具说明");
});
it("offers no governance action to a consumer and no approval to an Owner", () => {
  const client = new QueryClient();
  const view = render(
    <QueryClientProvider client={client}>
      <GovernanceControls
        item={{ ...item, is_admin: false, is_owner: false }}
      />
    </QueryClientProvider>,
  );
  expect(screen.queryByRole("button")).not.toBeInTheDocument();
  view.rerender(
    <QueryClientProvider client={client}>
      <GovernanceControls
        item={{ ...item, is_admin: false, is_owner: true, status: "draft" }}
      />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("button", { name: "提交审核" })).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "通过并上线" }),
  ).not.toBeInTheDocument();
});
it("gates the review query on server role metadata", async () => {
  vi.mocked(getGovernanceIdentity).mockResolvedValue({
    open_id: "ou_user",
    is_admin: false,
    department: "",
    revision: 0,
  });
  mount(<ReviewsPage />);
  expect(await screen.findByText("需要管理员权限")).toBeInTheDocument();
  expect(listReviews).not.toHaveBeenCalled();
});
it("lists pending capabilities for admin review", async () => {
  vi.mocked(getGovernanceIdentity).mockResolvedValue({
    open_id: "ou_admin",
    is_admin: true,
    department: "",
    revision: 1,
  });
  vi.mocked(listReviews).mockResolvedValue({ items: [item], has_more: false });
  mount(<ReviewsPage />);
  expect(await screen.findByRole("link", { name: /Search/ })).toHaveAttribute(
    "href",
    "/hub/registry/search",
  );
  expect(screen.getByText("旧版本仍上线")).toBeInTheDocument();
});
