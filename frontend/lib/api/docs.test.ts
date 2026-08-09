import { beforeEach, describe, expect, it, vi } from "vitest";

import { docApi } from "./docs";

const acceptedDoc = {
  id: "doc-1",
  kb_id: "kb-1",
  source_type: "feishu-docx",
  source_ref: "feishu://feishu.cn/docx/token",
  title: "Runbook",
  mime_type: "text/markdown",
  bytes: 42,
  checksum: "checksum",
  status: "pending",
  metadata: {},
  created_at: "2026-08-09T00:00:00Z",
  updated_at: "2026-08-09T00:00:00Z",
};

describe("docApi Feishu operations", () => {
  beforeEach(() => {
    document.cookie = "it_wiki_csrf=sync-csrf; path=/";
    vi.unstubAllGlobals();
  });

  it("imports one URL through the authenticated KB endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(acceptedDoc), { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);

    await docApi.importFeishu("kb-1", "https://acme.feishu.cn/docx/token");

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/api/v1/kbs/kb-1/feishu-imports",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        body: JSON.stringify({ url: "https://acme.feishu.cn/docx/token" }),
        headers: expect.objectContaining({ "X-CSRF-Token": "sync-csrf" }),
      }),
    );
  });

  it("requests a manual sync through the authenticated document endpoint", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(acceptedDoc), { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);

    await docApi.syncFeishu("doc-1");

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/api/v1/docs/doc-1/sync",
      expect.objectContaining({
        method: "POST",
        credentials: "include",
        headers: expect.objectContaining({ "X-CSRF-Token": "sync-csrf" }),
      }),
    );
  });
});
