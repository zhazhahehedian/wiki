import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import Home from "./page";
import RetiredKnowledgeBasePage from "./kbs/[[...path]]/page";
const redirect = vi.fn();
const useAuth = vi.fn();
vi.mock("next/navigation", () => ({ redirect: (path: string) => redirect(path) }));
vi.mock("@/lib/hooks/use-auth", () => ({ useAuth: () => useAuth() }));
beforeEach(() => {
  redirect.mockReset();
  useAuth.mockReturnValue({ isLoading: false, isFetching: false, isError: true, data: undefined });
});

it("shows public guidance without redirecting guests into protected pages", () => {
  render(<Home />);
  expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("让团队的能力");
  expect(screen.getAllByRole("link", { name: "飞书登录" }).every(link => link.getAttribute("href") === "/login")).toBe(true);
  expect(screen.getByText(/使用公司飞书账号/)).toBeInTheDocument();
  expect(redirect).not.toHaveBeenCalled();
});

it("switches the preview and lets visitors pause animation", async () => {
  const user = userEvent.setup();
  render(<Home />);
  const knowledge = screen.getByRole("button", { name: /连接知识/ });
  await user.click(knowledge);
  expect(knowledge).toHaveAttribute("aria-pressed", "true");
  expect(screen.getByRole("heading", { name: "让沉淀的经验，成为可检索的知识。" })).toBeInTheDocument();
  expect(screen.getByRole("link", { name: "登录后进入知识服务" })).toHaveAttribute("href", "/login");
  await user.click(screen.getByRole("button", { name: "暂停装饰动画" }));
  expect(screen.getByRole("button", { name: "播放装饰动画" })).toBeInTheDocument();
});

it("offers confirmed users a workspace and the selected feature", async () => {
  useAuth.mockReturnValue({ isLoading: false, isFetching: false, isError: false, data: { id: "user-1" } });
  render(<Home />);
  expect(screen.getAllByRole("link", { name: "进入工作台" }).every(link => link.getAttribute("href") === "/hub")).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: /连接知识/ }));
  expect(screen.getByRole("link", { name: "进入知识服务" })).toHaveAttribute("href", "/hub/knowledge");
});

it("does not treat stale session data as an authenticated user after an error", () => {
  useAuth.mockReturnValue({ isLoading: false, isFetching: false, isError: true, data: { id: "stale" } });
  render(<Home />);
  expect(screen.queryByRole("link", { name: "进入工作台" })).not.toBeInTheDocument();
});

it("keeps legacy knowledge bookmarks redirected", () => {
  RetiredKnowledgeBasePage();
  expect(redirect).toHaveBeenCalledWith("/hub/knowledge");
});
