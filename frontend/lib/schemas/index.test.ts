import { describe, expect, it } from "vitest";

import { docSchema } from "./index";

const baseDoc = {
  id: "doc-1",
  kb_id: "kb-1",
  source_type: "feishu-docx",
  source_ref: "feishu://feishu.cn/docx/token",
  title: "Runbook",
  mime_type: "text/markdown",
  bytes: 42,
  checksum: "checksum",
  status: "ready",
  metadata: {},
  created_at: "2026-08-09T00:00:00Z",
  updated_at: "2026-08-09T00:00:00Z",
};

describe("docSchema sync fields", () => {
  it("preserves staged Feishu sync fields using backend JSON names", () => {
    const parsed = docSchema.parse({
      ...baseDoc,
      source_url: "https://acme.feishu.cn/docx/token",
      remote_revision: "rev-1",
      sync_status: "failed",
      last_sync_error: "permission denied",
      last_synced_at: "2026-08-09T01:00:00Z",
    });

    expect(parsed).toEqual(expect.objectContaining({
      source_url: "https://acme.feishu.cn/docx/token",
      remote_revision: "rev-1",
      sync_status: "failed",
      last_sync_error: "permission denied",
      last_synced_at: "2026-08-09T01:00:00Z",
    }));
  });

  it("accepts legacy and local-upload responses without remote sync fields", () => {
    expect(docSchema.parse({ ...baseDoc, source_type: "local-upload" })).toEqual(
      expect.objectContaining({ source_type: "local-upload", status: "ready" }),
    );
  });
});
