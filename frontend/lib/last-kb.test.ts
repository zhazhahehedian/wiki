import { beforeEach, describe, expect, it } from "vitest";

import { clearLastKbId, getLastKbId, setLastKbId } from "./last-kb";

describe("last-kb storage", () => {
  beforeEach(() => localStorage.clear());

  it("returns null when nothing stored", () => {
    expect(getLastKbId()).toBeNull();
  });

  it("round-trips the last visited kb id", () => {
    setLastKbId("kb-123");
    expect(getLastKbId()).toBe("kb-123");
  });

  it("clears the stored id when it matches", () => {
    setLastKbId("kb-123");
    clearLastKbId("kb-123");
    expect(getLastKbId()).toBeNull();
  });

  it("keeps the stored id when clearing a different id", () => {
    setLastKbId("kb-123");
    clearLastKbId("kb-456");
    expect(getLastKbId()).toBe("kb-123");
  });
});
