import { beforeEach, describe, expect, it, vi } from "vitest";

import { streamConversationMessage } from "./chat";

describe("streamConversationMessage", () => {
  beforeEach(() => {
    document.cookie = "it_wiki_csrf=; Max-Age=0; path=/";
    vi.unstubAllGlobals();
  });

  it("includes session credentials and the shared csrf token", async () => {
    document.cookie = "it_wiki_csrf=stream-csrf; path=/";
    const fetchMock = vi.fn().mockResolvedValue(
      new Response("", {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await streamConversationMessage("conversation-1", "hello", { onEvent: vi.fn() });

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/api/v1/conversations/conversation-1/messages/stream",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        headers: expect.objectContaining({
          Accept: "text/event-stream",
          "Content-Type": "application/json",
          "X-CSRF-Token": "stream-csrf",
        }),
      }),
    );
  });

  it.each([
    { status: 401, code: "unauthenticated" },
    { status: 403, code: "csrf_rejected" },
  ])("preserves standardized $status errors", async ({ status, code }) => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          error: {
            code,
            message: "request rejected",
            details: { reason: "test" },
            request_id: "request-1",
          },
        }),
        {
          status,
          headers: { "Content-Type": "application/json" },
        },
      ),
    );
    vi.stubGlobal("fetch", fetchMock);

    await expect(
      streamConversationMessage("conversation-1", "hello", { onEvent: vi.fn() }),
    ).rejects.toMatchObject({
      name: "Error",
      status,
      code,
      message: "request rejected",
      details: { reason: "test" },
      requestId: "request-1",
    });
  });
});
