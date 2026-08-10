import { useEffect, useRef } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, useQueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import { apiFetch } from "@/lib/api/client";
import { authQueryKey } from "@/lib/hooks/use-auth";
import LoginPage from "@/app/login/page";
import { Providers } from "./providers";

const replace = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace }),
  useSearchParams: () => new URLSearchParams(),
}));

vi.mock("next-themes", () => ({
  ThemeProvider: ({ children }: { children: React.ReactNode }) => children,
}));
vi.mock("@tanstack/react-query-devtools", () => ({ ReactQueryDevtools: () => null }));
vi.mock("@/components/ui/sonner", () => ({ Toaster: () => null }));
vi.mock("@/components/ui/tooltip", () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => children,
}));

function SeedAuthCache({ onReady }: { onReady: (client: QueryClient) => void }) {
  const queryClient = useQueryClient();
  useEffect(() => {
    queryClient.setQueryData(authQueryKey, { id: "stale-user", display_name: "Stale" });
    onReady(queryClient);
  }, [onReady, queryClient]);
  return null;
}

function StaleAuthLogin({ onReady }: { onReady: (client: QueryClient) => void }) {
  const queryClient = useQueryClient();
  const initialized = useRef(false);
  if (!initialized.current) {
    queryClient.setQueryData(
      authQueryKey,
      { id: "stale-user", display_name: "Stale" },
      { updatedAt: 1 },
    );
    onReady(queryClient);
    initialized.current = true;
  }
  return <LoginPage />;
}

describe("Providers authentication boundary", () => {
  afterEach(() => {
    replace.mockReset();
    vi.unstubAllGlobals();
  });

  it("clears stale auth data when a business request returns 401 outside AppShell", async () => {
    let queryClient: QueryClient | undefined;
    const onReady = vi.fn((client: QueryClient) => {
      queryClient = client;
    });
    render(
      <Providers>
        <SeedAuthCache onReady={onReady} />
      </Providers>,
    );
    await waitFor(() => expect(onReady).toHaveBeenCalledOnce());
    expect(queryClient?.getQueryData(authQueryKey)).toEqual({
      id: "stale-user",
      display_name: "Stale",
    });
    const removeQueries = vi.spyOn(queryClient!, "removeQueries");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "unauthenticated", message: "login required" },
    }), { status: 401 })));

    await expect(apiFetch("/api/v1/kbs")).rejects.toMatchObject({ status: 401 });

    expect(queryClient?.getQueryData(authQueryKey)).toBeUndefined();
    expect(removeQueries).toHaveBeenCalledOnce();
  });

  it("removes the unauthorized subscription when Providers unmounts", async () => {
    let queryClient: QueryClient | undefined;
    const onReady = vi.fn((client: QueryClient) => {
      queryClient = client;
    });
    const { unmount } = render(
      <Providers>
        <SeedAuthCache onReady={onReady} />
      </Providers>,
    );
    await waitFor(() => expect(onReady).toHaveBeenCalledOnce());
    unmount();
    queryClient!.setQueryData(authQueryKey, { id: "post-unmount" });
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "unauthenticated", message: "login required" },
    }), { status: 401 })));

    await expect(apiFetch("/api/v1/kbs")).rejects.toMatchObject({ status: 401 });

    expect(queryClient?.getQueryData(authQueryKey)).toEqual({ id: "post-unmount" });
  });

  it("lets the login auth query settle after /auth/me returns 401", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "unauthenticated", message: "login required" },
    }), { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);

    render(
      <Providers>
        <LoginPage />
      </Providers>,
    );

    expect(await screen.findByRole("link")).toHaveAttribute(
      "href",
      "http://localhost:8080/api/v1/auth/feishu/start",
    );
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(replace).not.toHaveBeenCalled();
  });

  it("rejects stale auth data after the login auth query returns 401", async () => {
    let queryClient: QueryClient | undefined;
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      error: { code: "unauthenticated", message: "login required" },
    }), { status: 401 }));
    vi.stubGlobal("fetch", fetchMock);

    render(
      <Providers>
        <StaleAuthLogin onReady={(client) => { queryClient = client; }} />
      </Providers>,
    );

    await waitFor(() => expect(queryClient?.getQueryState(authQueryKey)).toMatchObject({
      status: "error",
      fetchStatus: "idle",
    }));
    expect(screen.getByRole("link")).toHaveAttribute(
      "href",
      "http://localhost:8080/api/v1/auth/feishu/start",
    );
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(replace).not.toHaveBeenCalled();
  });
});
