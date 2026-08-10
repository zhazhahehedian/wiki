import { act, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

import { authApi } from "@/lib/api/auth";
import { authQueryKey, useLogout } from "./use-auth";

vi.mock("@/lib/api/auth", () => ({ authApi: { logout: vi.fn(), me: vi.fn() } }));

describe("useLogout", () => {
  it("clears cached authentication after server logout", async () => {
    vi.mocked(authApi.logout).mockResolvedValue();
    const client = new QueryClient();
    client.setQueryData(authQueryKey, { id: "user-1" });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={client}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useLogout(), { wrapper });

    await act(() => result.current.mutateAsync());

    expect(authApi.logout).toHaveBeenCalledOnce();
    expect(client.getQueryData(authQueryKey)).toBeUndefined();
  });
});
