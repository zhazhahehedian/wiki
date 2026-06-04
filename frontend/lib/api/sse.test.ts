import { describe, expect, it } from "vitest";

import { parseSSEBuffer, parseSSEEvent } from "./sse";

describe("parseSSEEvent", () => {
  it("parses named events", () => {
    expect(parseSSEEvent('event: token\ndata: {"text":"hi"}')).toEqual({
      event: "token",
      data: '{"text":"hi"}',
    });
  });

  it("joins multi-line data", () => {
    expect(parseSSEEvent("event: token\ndata: one\ndata: two")).toEqual({
      event: "token",
      data: "one\ntwo",
    });
  });
});

describe("parseSSEBuffer", () => {
  it("keeps partial trailing events", () => {
    const parsed = parseSSEBuffer("event: token\ndata: one\n\nevent: token\ndata:");

    expect(parsed.events).toHaveLength(1);
    expect(parsed.rest).toBe("event: token\ndata:");
  });
});
