import { beforeEach, describe, expect, it } from "vitest";

import { getLastKbId, setLastKbId } from "./last-kb";

describe("last-kb storage", () => {
  beforeEach(() => localStorage.clear());

  it("returns null when nothing stored", () => {
    expect(getLastKbId()).toBeNull();
  });

  it("round-trips the last visited kb id", () => {
    setLastKbId("kb-123");
    expect(getLastKbId()).toBe("kb-123");
  });
});
