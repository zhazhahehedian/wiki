import { beforeEach, describe, expect, it, vi } from "vitest";

import { authApi } from "./auth";

describe("authApi", () => {
  beforeEach(() => {
    document.cookie = "it_wiki_csrf=; Max-Age=0; path=/";
    vi.unstubAllGlobals();
  });

  it("loads the current user with credentials included", async () => {
    const user = {
      id: "user-1",
      display_name: "Ada",
      email: "ada@example.test",
      avatar_url: "",
      created_at: "2026-08-09T00:00:00Z",
      updated_at: "2026-08-09T00:00:00Z",
    };
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify(user), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(authApi.me()).resolves.toEqual(user);

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/api/v1/auth/me",
      expect.objectContaining({ credentials: "include", method: "GET" }),
    );
  });

  it("logs out with credentials and the csrf cookie value", async () => {
    document.cookie = "it_wiki_csrf=session-csrf; path=/";
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await authApi.logout();

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/api/v1/auth/logout",
      expect.objectContaining({
        credentials: "include",
        method: "POST",
        headers: expect.objectContaining({ "X-CSRF-Token": "session-csrf" }),
      }),
    );
  });

  it("provides a fixed OAuth start URL without browser tokens", () => {
    const url = authApi.feishuStartURL();

    expect(url).toBe("http://localhost:8080/api/v1/auth/feishu/start");
    expect(url).not.toMatch(/token|redirect=/);
  });
});
