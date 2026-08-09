import { describe, expect, it } from "vitest";

import type { Doc } from "@/lib/schemas";
import { docNeedsRefresh } from "./use-docs";

function state(status: Doc["status"], syncStatus?: Doc["sync_status"]): Doc {
  return { status, sync_status: syncStatus } as Doc;
}

describe("docNeedsRefresh", () => {
  it("treats ingestion and remote sync as independent state machines", () => {
    expect(docNeedsRefresh(state("ready", "syncing"))).toBe(true);
    expect(docNeedsRefresh(state("ready", "failed"))).toBe(false);
    expect(docNeedsRefresh(state("pending", "idle"))).toBe(true);
    expect(docNeedsRefresh(state("ready", "idle"))).toBe(false);
    expect(docNeedsRefresh(state("ready", "idle"), true)).toBe(true);
  });
});
