import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import LoginPage from "./page";

const replace = vi.fn();
const useAuth = vi.fn();
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
    search = new URLSearchParams();
  });

  it("shows a minimal Feishu login screen for guests", () => {
    useAuth.mockReturnValue({ isLoading: false, isError: true, data: undefined });
    render(<LoginPage />);

    expect(screen.getByRole("heading", { name: "登录 it-wiki" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "使用飞书登录" })).toBeInTheDocument();
  });

  it("shows a safe login error from the OAuth return URL", () => {
    search = new URLSearchParams("error=tenant_not_allowed");
    useAuth.mockReturnValue({ isLoading: false, isError: true, data: undefined });
    render(<LoginPage />);

    expect(screen.getByRole("alert")).toHaveTextContent("当前飞书账号不属于允许的组织");
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
