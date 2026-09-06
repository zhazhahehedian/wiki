import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { APIError } from "@/lib/api/client";
import { getGovernanceIdentity } from "@/lib/api/registry";
vi.mock("@/lib/api/registry", () => ({ getGovernanceIdentity: vi.fn() }));
import { HubShell } from "./hub-shell";
const mocks = vi.hoisted(() => ({
  auth: vi.fn(),
  logout: vi.fn(),
  replace: vi.fn(),
  expired: false,
  pathname: "/hub/registry",
}));
vi.mock("next/navigation", () => ({
  usePathname: () => mocks.pathname,
  useRouter: () => ({ replace: mocks.replace }),
}));
vi.mock("@/lib/hooks/use-auth", () => ({
  useAuth: () => mocks.auth(),
  useLogout: () => ({ mutateAsync: mocks.logout, isPending: false }),
}));
vi.mock("@/components/auth/auth-session-boundary", () => ({
  useSessionExpired: () => mocks.expired,
}));
function mount() {
  const client = new QueryClient();
  client.setQueryData(["private"], "cached");
  render(
    <QueryClientProvider client={client}>
      <HubShell>
        <p>private content</p>
      </HubShell>
    </QueryClientProvider>,
  );
  return client;
}
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getGovernanceIdentity).mockResolvedValue({
    open_id: "ou_user",
    is_admin: false,
    department: "",
    revision: 0,
  });
  mocks.expired = false;
  mocks.pathname = "/hub/registry";
  mocks.auth.mockReturnValue({
    data: { id: "user", display_name: "Ada" },
    isLoading: false,
    isFetching: false,
    isError: false,
  });
});
it("shows the new navigation with an active page and no legacy product links", () => {
  mount();
  expect(screen.getByRole("link", { name: "注册中心" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  expect(screen.getByText("private content")).toBeInTheDocument();
  expect(
    screen.queryByRole("link", { name: /知识库|审核队列|审计日志/ }),
  ).not.toBeInTheDocument();
});
it.each(["loading", "revalidating", "expired", "unauthorized"])(
  "hides private content when %s",
  (state) => {
    mocks.expired = state === "expired";
    mocks.auth.mockReturnValue({
      data: { id: "stale" },
      isLoading: state === "loading",
      isFetching: state === "revalidating",
      isError: state === "unauthorized",
      error: new APIError(401, "unauthenticated", ""),
    });
    mount();
    expect(screen.queryByText("private content")).not.toBeInTheDocument();
    if (state === "expired" || state === "unauthorized")
      expect(mocks.replace).toHaveBeenCalledWith("/login");
  },
);
it("offers retry rather than login on an auth service error", async () => {
  const refetch = vi.fn();
  mocks.auth.mockReturnValue({
    data: undefined,
    isError: true,
    error: new APIError(503, "internal", "secret"),
    refetch,
  });
  mount();
  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(screen.queryByText("secret")).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole("button"));
  expect(refetch).toHaveBeenCalled();
  expect(mocks.replace).not.toHaveBeenCalled();
});
it("clears cached private data and hides content after successful logout", async () => {
  mocks.logout.mockResolvedValue(undefined);
  const client = mount();
  await userEvent.click(screen.getByRole("button", { name: "退出登录" }));
  await waitFor(() => expect(mocks.replace).toHaveBeenCalledWith("/login"));
  expect(client.getQueryData(["private"])).toBeUndefined();
  expect(screen.queryByText("private content")).not.toBeInTheDocument();
});
it("keeps the session visible if logout fails", async () => {
  mocks.logout.mockRejectedValue(new Error("unavailable"));
  const client = mount();
  await userEvent.click(screen.getByRole("button", { name: "退出登录" }));
  expect(mocks.replace).not.toHaveBeenCalled();
  expect(client.getQueryData(["private"])).toBe("cached");
});
it("opens mobile navigation and closes it on destination selection", async () => {
  mount();
  await userEvent.click(screen.getByRole("button", { name: "打开导航" }));
  expect(screen.getByRole("dialog")).toBeInTheDocument();
  const links = screen.getAllByRole("link", { name: "设置" });
  await userEvent.click(links[links.length - 1]);
  await waitFor(() =>
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
  );
});

it("keeps registry navigation selected on detail and edit routes", () => {
  mocks.pathname = "/hub/registry/search/edit";
  mount();
  expect(screen.getByRole("link", { name: "注册中心" })).toHaveAttribute(
    "aria-current",
    "page",
  );
  expect(screen.getByRole("link", { name: "仪表盘" })).not.toHaveAttribute(
    "aria-current",
  );
});

it("shows governance navigation only for a server-authorized admin", async () => {
  vi.mocked(getGovernanceIdentity).mockResolvedValue({
    open_id: "ou_user",
    is_admin: true,
    department: "",
    revision: 1,
  });
  mount();
  expect(await screen.findByRole("link", { name: "审核队列" })).toHaveAttribute(
    "href",
    "/hub/reviews",
  );
  expect(screen.getByRole("link", { name: "审计日志" })).toHaveAttribute(
    "href",
    "/hub/audit",
  );
  expect(screen.getByRole("link", { name: "部门授权" })).toBeInTheDocument();
});
