import { afterEach, expect, it, vi } from "vitest";
import { streamModelChat } from "./playground";

afterEach(() => vi.restoreAllMocks());
const input = {
  model: "model",
  messages: [{ role: "user" as const, content: "hello" }],
};
function mockStream(text: string) {
  const bytes = new TextEncoder().encode(text);
  // Split every byte to exercise UTF-8 and event-boundary buffering.
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const byte of bytes) controller.enqueue(new Uint8Array([byte]));
      controller.close();
    },
  });
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(body, { headers: { "Content-Type": "text/event-stream" } }),
  );
}
it("decodes split UTF-8 SSE frames and forwards session, CSRF and cancellation", async () => {
  document.cookie = "it_wiki_csrf=test-csrf; path=/";
  mockStream(
    'event: delta\r\ndata: {"text":"你好"}\r\n\r\nevent: done\r\ndata: {}\r\n\r\n',
  );
  const controller = new AbortController();
  const emit = vi.fn();
  await streamModelChat(input, controller.signal, emit);
  expect(emit).toHaveBeenCalledWith("你好");
  expect(fetch).toHaveBeenCalledWith(
    expect.stringContaining("/playground/chat/stream"),
    expect.objectContaining({
      signal: controller.signal,
      credentials: "include",
      headers: expect.objectContaining({ "X-CSRF-Token": "test-csrf" }),
    }),
  );
  document.cookie = "it_wiki_csrf=; max-age=0; path=/";
});
it.each([
  'event: delta\ndata: {"text":"partial"}\n\n',
  'event: error\ndata: {"message":"private provider body"}\n\n',
])(
  "rejects incomplete and error streams without exposing provider details",
  async (body) => {
    mockStream(body);
    await expect(
      streamModelChat(input, new AbortController().signal, vi.fn()),
    ).rejects.toThrow("模型响应中断，请重试");
  },
);
