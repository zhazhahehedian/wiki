import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AuthSessionBoundary } from "@/components/auth/auth-session-boundary";
import { apiFetch, APIError } from "@/lib/api/client";
import { AppShell } from "./app-shell";

const replace = vi.fn();
const refetch = vi.fn();
const useAuth = vi.fn();

function renderShell(children: ReactNode) {
  const queryClient = new QueryClient();
  const removeQueries = vi.spyOn(queryClient, "removeQueries");
  const result = render(
    <QueryClientProvider client={queryClient}>
      <AuthSessionBoundary>
        <AppShell>{children}</AppShell>
      </AuthSessionBoundary>
    </QueryClientProvider>,
  );
  return { ...result, removeQueries };
}

vi.mock("next/navigation", () => ({
  useParams: () => ({ kbId: "kb-1" }),
  usePathname: () => "/kbs/kb-1/docs",
  useRouter: () => ({ replace }),
}));
vi.mock("@/lib/hooks/use-auth", () => ({ useAuth: () => useAuth() }));
vi.mock("@/components/layout/rail", () => ({ Rail: () => <nav>rail</nav> }));
vi.mock("@/components/layout/chat-panel", () => ({ ChatPanel: () => <aside>chat panel</aside> }));
vi.mock("@/components/layout/kb-panel", () => ({ KbPanel: () => <aside>kb panel</aside> }));
vi.mock("@/components/ui/sidebar", () => ({
  SidebarProvider: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  Sidebar: ({ children }: { children: React.ReactNode }) => <aside>{children}</aside>,
  SidebarContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  SidebarInset: ({ children }: { children: React.ReactNode }) => <main>{children}</main>,
}));

describe("AppShell auth guard", () => {
  beforeEach(() => {
    replace.mockReset();
    refetch.mockReset();
    useAuth.mockReset();
    vi.unstubAllGlobals();
  });

  it("does not flash protected UI while authentication is loading", () => {
    useAuth.mockReturnValue({ isLoading: true, isError: false, data: undefined, refetch });
    renderShell(<p>private docs</p>);

    expect(screen.getByText("正在验证登录状态…")).toBeInTheDocument();
    expect(screen.queryByText("private docs")).not.toBeInTheDocument();
  });

  it("keeps protected UI visible while confirmed auth data is revalidating", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: true,
      isError: false,
      data: { id: "stale-user", display_name: "Stale" },
      refetch,
    });

    renderShell(<p>private docs</p>);

    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByText("private docs")).toBeInTheDocument();
    expect(screen.getByText("rail")).toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });

  it("redirects an unauthenticated user without rendering protected UI", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isError: true,
      error: new APIError(401, "unauthenticated", "login required"),
      data: { id: "stale-user", display_name: "Stale" },
      refetch,
    });
    renderShell(<p>private docs</p>);

    expect(replace).toHaveBeenCalledWith("/login");
    expect(screen.queryByText("private docs")).not.toBeInTheDocument();
  });

  it("shows a retryable auth error instead of protected UI", async () => {
    const user = userEvent.setup();
    useAuth.mockReturnValue({
      isLoading: false,
      isError: true,
      error: new APIError(503, "unavailable", "auth unavailable"),
      data: undefined,
      refetch,
    });
    renderShell(<p>private docs</p>);

    await user.click(screen.getByRole("button", { name: "重试" }));
    expect(refetch).toHaveBeenCalledOnce();
    expect(screen.queryByText("private docs")).not.toBeInTheDocument();
  });

  it("keeps the auth error visible and disables retry while refetching", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: true,
      isError: true,
      error: new APIError(503, "unavailable", "internal provider detail"),
      data: undefined,
      refetch,
    });
    renderShell(<p>private docs</p>);

    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("button")).toBeDisabled();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByText("internal provider detail")).not.toBeInTheDocument();
    expect(screen.queryByText("private docs")).not.toBeInTheDocument();
  });

  it("renders the application only for an authenticated user", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isError: false,
      data: { id: "user-1", display_name: "Ada" },
      refetch,
    });
    renderShell(<p>private docs</p>);

    expect(screen.getByText("private docs")).toBeInTheDocument();
  });

  it("hides protected UI and redirects when a business request reports an expired session", async () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isError: false,
      data: { id: "user-1", display_name: "Ada" },
      refetch,
    });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "unauthenticated", message: "login required" },
    }), { status: 401 })));
    const { removeQueries } = renderShell(<p>private docs</p>);
    expect(screen.getByText("private docs")).toBeInTheDocument();

    await expect(apiFetch("/api/v1/docs/doc-1")).rejects.toMatchObject({ status: 401 });

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/login"));
    expect(screen.queryByText("private docs")).not.toBeInTheDocument();
    expect(removeQueries).toHaveBeenCalledOnce();
  });
});
