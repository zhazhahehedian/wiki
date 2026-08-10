import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { APIError } from "@/lib/api/client";
import LoginPage from "./page";

const replace = vi.fn();
const useAuth = vi.fn();
const refetch = vi.fn();
let search = new URLSearchParams();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace }),
  useSearchParams: () => search,
}));
vi.mock("@/lib/hooks/use-auth", () => ({ useAuth: () => useAuth() }));

describe("LoginPage", () => {
  beforeEach(() => {
    replace.mockReset();
    useAuth.mockReset();
    refetch.mockReset();
    search = new URLSearchParams();
  });

  it("shows a minimal Feishu login screen for guests", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: true,
      error: new APIError(401, "unauthenticated", "login required"),
      data: undefined,
      refetch,
    });
    render(<LoginPage />);

    expect(screen.getByRole("heading", { name: "登录 it-wiki" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "使用飞书登录" })).toBeInTheDocument();
  });

  it("shows a retryable fixed error instead of OAuth controls for auth service failures", async () => {
    const user = userEvent.setup();
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: true,
      error: new APIError(503, "internal", "provider token secret"),
      data: { id: "stale-user" },
      refetch,
    });

    render(<LoginPage />);

    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(screen.queryByText("provider token secret")).not.toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("disables auth retry while the request is in flight", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: true,
      isError: true,
      error: new APIError(503, "internal", "hidden"),
      data: undefined,
      refetch,
    });

    render(<LoginPage />);

    expect(screen.getByRole("button")).toBeDisabled();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("shows a safe login error from the OAuth return URL", () => {
    search = new URLSearchParams("error=tenant_not_allowed");
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: true,
      error: new APIError(401, "unauthenticated", "login required"),
      data: undefined,
      refetch,
    });
    render(<LoginPage />);

    expect(screen.getByRole("alert")).toHaveTextContent("当前飞书账号不属于允许的组织");
  });

  it.each([
    ["oauth_cancelled", "登录已取消"],
    ["oauth_state_invalid", "登录请求已失效"],
    ["feishu_reauth_required", "重新授权飞书只读权限"],
    ["auth_service_unavailable", "登录服务暂时不可用"],
  ])("shows an allowlisted message for %s", (code, message) => {
    search = new URLSearchParams(`error=${code}`);
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: true,
      error: new APIError(401, "unauthenticated", "login required"),
      data: undefined,
      refetch,
    });

    render(<LoginPage />);

    expect(screen.getByRole("alert")).toHaveTextContent(message);
  });

  it("does not show login controls while checking an existing session", () => {
    useAuth.mockReturnValue({ isLoading: true, isError: false, data: undefined });
    render(<LoginPage />);

    expect(screen.getByText("正在检查登录状态…")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "使用飞书登录" })).not.toBeInTheDocument();
  });

  it("does not use stale auth data while revalidating a session", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: true,
      isError: false,
      data: { id: "stale-user" },
    });
    render(<LoginPage />);

    expect(screen.getByRole("status")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    expect(replace).not.toHaveBeenCalled();
  });

  it("redirects an authenticated user into the application", () => {
    useAuth.mockReturnValue({ isLoading: false, isError: false, data: { id: "user-1" } });
    render(<LoginPage />);

    expect(replace).toHaveBeenCalledWith("/");
    expect(screen.queryByRole("link", { name: "使用飞书登录" })).not.toBeInTheDocument();
  });
});
