import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { Rail } from "./rail";

const replace = vi.fn();
const logoutMutate = vi.fn();
const useAuth = vi.fn();

vi.mock("next/navigation", () => ({
  useParams: () => ({ kbId: "kb-1" }),
  usePathname: () => "/kbs/kb-1/docs",
  useRouter: () => ({ replace }),
}));
vi.mock("@/lib/hooks/use-auth", () => ({
  useAuth: () => useAuth(),
  useLogout: () => ({ mutate: logoutMutate, isPending: false }),
}));
vi.mock("@/components/layout/theme-toggle", () => ({ ThemeToggle: () => <button>主题</button> }));

describe("Rail user menu", () => {
  beforeEach(() => {
    replace.mockReset();
    logoutMutate.mockReset();
    useAuth.mockReset();
    useAuth.mockReturnValue({
      isError: false,
      data: { id: "user-1", display_name: "Ada", email: "ada@example.test", avatar_url: "" },
    });
  });

  it("opens from the keyboard and redirects after logout", async () => {
    const user = userEvent.setup();
    render(<TooltipProvider><Rail /></TooltipProvider>);

    const account = screen.getByRole("button", { name: "Ada 的账号菜单" });
    account.focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByText("ada@example.test")).toBeInTheDocument();

    await user.click(screen.getByRole("menuitem", { name: "退出登录" }));
    expect(logoutMutate).toHaveBeenCalledWith(undefined, expect.objectContaining({ onSuccess: expect.any(Function) }));

    const options = logoutMutate.mock.calls[0][1] as { onSuccess: () => void };
    options.onSuccess();
    expect(replace).toHaveBeenCalledWith("/login");
  });

  it("does not render a stale user menu after an auth error", () => {
    useAuth.mockReturnValue({
      isError: true,
      data: { id: "stale-user", display_name: "Ada", email: "ada@example.test", avatar_url: "" },
    });

    render(<TooltipProvider><Rail /></TooltipProvider>);

    expect(screen.queryByRole("button", { name: /Ada/ })).not.toBeInTheDocument();
  });

  it("keeps the confirmed user menu while auth revalidates", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: true,
      isError: false,
      data: { id: "stale-user", display_name: "Ada", email: "ada@example.test", avatar_url: "" },
    });

    render(<TooltipProvider><Rail /></TooltipProvider>);

    expect(screen.getByRole("button", { name: /Ada/ })).toBeInTheDocument();
  });
});
