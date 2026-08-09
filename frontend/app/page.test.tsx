import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { APIError } from "@/lib/api/client";
import Home from "./page";

const replace = vi.fn();
const useAuth = vi.fn();
const useKbs = vi.fn();
const useSessionExpired = vi.fn();
const refetch = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace }),
}));
vi.mock("@/lib/hooks/use-auth", () => ({ useAuth: () => useAuth() }));
vi.mock("@/components/auth/auth-session-boundary", () => ({
  useSessionExpired: () => useSessionExpired(),
}));
vi.mock("@/lib/hooks/use-kbs", () => ({
  useKbs: (limit?: number, offset?: number, enabled?: boolean) => useKbs(limit, offset, enabled),
}));

describe("root authentication gate", () => {
  beforeEach(() => {
    replace.mockReset();
    useAuth.mockReset();
    useKbs.mockReset();
    useSessionExpired.mockReset();
    refetch.mockReset();
    useSessionExpired.mockReturnValue(false);
    useKbs.mockReturnValue({ data: undefined, isError: false });
  });

  it("does not request knowledge bases while authentication is loading", () => {
    useAuth.mockReturnValue({ isLoading: true, isError: false, data: undefined });

    render(<Home />);

    expect(useKbs).toHaveBeenCalledWith(20, 0, false);
    expect(replace).not.toHaveBeenCalled();
    expect(screen.queryByText("private knowledge base")).not.toBeInTheDocument();
  });

  it("does not request knowledge bases while stale auth data is revalidating", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: true,
      isError: false,
      data: { id: "stale-user", display_name: "Stale" },
    });

    render(<Home />);

    expect(useKbs).toHaveBeenCalledWith(20, 0, false);
    expect(replace).not.toHaveBeenCalled();
  });

  it("shows a retryable error without requesting knowledge bases after an auth service failure", async () => {
    const user = userEvent.setup();
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: false,
      isError: true,
      error: new APIError(503, "unavailable", "auth unavailable"),
      data: { id: "stale-user", display_name: "Stale" },
      refetch,
    });

    render(<Home />);

    expect(useKbs).toHaveBeenCalledWith(20, 0, false);
    expect(replace).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.queryByText("private knowledge base")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button"));
    expect(refetch).toHaveBeenCalledOnce();
  });

  it("disables retry while the auth service request is in flight", () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isFetching: true,
      isError: true,
      error: new APIError(503, "unavailable", "internal provider detail"),
      data: { id: "stale-user", display_name: "Stale" },
      refetch,
    });

    render(<Home />);

    expect(screen.getByRole("button")).toBeDisabled();
    expect(screen.queryByText("internal provider detail")).not.toBeInTheDocument();
  });

  it("redirects an unauthenticated visitor directly to login without requesting KBs", async () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isError: true,
      error: new APIError(401, "unauthenticated", "login required"),
      data: { id: "stale-user", display_name: "Stale" },
    });

    render(<Home />);

    expect(useKbs).toHaveBeenCalledWith(20, 0, false);
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/login?next=%2F"));
    expect(replace).not.toHaveBeenCalledWith("/kbs");
  });

  it("stops using stale auth data after the shared session boundary expires", async () => {
    useSessionExpired.mockReturnValue(true);
    useAuth.mockReturnValue({
      isLoading: false,
      isError: false,
      data: { id: "stale-user", display_name: "Stale" },
    });

    render(<Home />);

    expect(useKbs).toHaveBeenCalledWith(20, 0, false);
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/login?next=%2F"));
    expect(replace).not.toHaveBeenCalledWith("/kbs");
  });

  it("loads KBs only after authentication and keeps the existing destination logic", async () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isError: false,
      data: { id: "user-1", display_name: "Ada" },
    });
    useKbs.mockReturnValue({
      data: { items: [{ id: "kb-1", name: "Operations" }] },
      isError: false,
    });

    render(<Home />);

    expect(useKbs).toHaveBeenCalledWith(20, 0, true);
    await waitFor(() => expect(replace).toHaveBeenCalledWith("/kbs/kb-1/chats"));
  });

  it("does not navigate with stale KB data when the KB request returns 401", async () => {
    useAuth.mockReturnValue({
      isLoading: false,
      isError: false,
      data: { id: "user-1", display_name: "Ada" },
    });
    useKbs.mockReturnValue({
      data: { items: [{ id: "stale-kb", name: "Stale" }] },
      isError: true,
      error: new APIError(401, "unauthenticated", "login required"),
    });

    render(<Home />);

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/login?next=%2F"));
    expect(replace).not.toHaveBeenCalledWith("/kbs/stale-kb/chats");
    expect(replace).not.toHaveBeenCalledWith("/kbs");
  });
});
