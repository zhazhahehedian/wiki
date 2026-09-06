import { afterEach, expect, it, vi } from "vitest";
import {
  listCapabilities,
  saveCapability,
  type CapabilityInput,
} from "./registry";
afterEach(() => {
  vi.unstubAllGlobals();
  document.cookie = "it_wiki_csrf=; Max-Age=0; path=/";
});
it("uploads multipart metadata/files with session and CSRF", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response('{"slug":"report"}', { status: 201 }));
  vi.stubGlobal("fetch", fetch);
  document.cookie = "it_wiki_csrf=test-csrf; path=/";
  const input = {
    slug: "report",
    type: "skill",
    revision: 2,
  } as CapabilityInput;
  const file = new File(["# Instructions"], "SKILL.md");
  await saveCapability(input, [file], "report");
  const [url, options] = fetch.mock.calls[0];
  expect(url).toContain("/api/v1/capabilities/report");
  expect(options).toMatchObject({
    method: "PUT",
    credentials: "include",
    headers: { "X-CSRF-Token": "test-csrf" },
  });
  expect(options.headers["Content-Type"]).toBeUndefined();
  expect(options.body.get("metadata")).toBe(JSON.stringify(input));
  expect(options.body.get("files").name).toBe("SKILL.md");
});
it("encodes filter values and forwards cancellation", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response('{"items":[],"has_more":false}'));
  vi.stubGlobal("fetch", fetch);
  const controller = new AbortController();
  await listCapabilities(
    { q: "a&status=published", offset: 24 },
    controller.signal,
  );
  const [url, options] = fetch.mock.calls[0];
  expect(new URL(url).searchParams.get("q")).toBe("a&status=published");
  expect(new URL(url).searchParams.get("status")).toBeNull();
  expect(options.signal).toBe(controller.signal);
});

it("preserves relative attachment paths beneath the selected Skill directory", async () => {
  const fetch = vi
    .fn()
    .mockResolvedValue(new Response('{"slug":"report"}', { status: 201 }));
  vi.stubGlobal("fetch", fetch);
  const file = new File(["template"], "template.md");
  Object.defineProperty(file, "webkitRelativePath", {
    value: "my-skill/references/template.md",
  });
  await saveCapability({ slug: "report" } as CapabilityInput, [file]);
  expect(fetch.mock.calls[0][1].body.get("files").name).toBe(
    "references/template.md",
  );
});

it("separates catalog reads and binds governance writes to revision and version with CSRF", async () => {
  const { listCatalog, actOnCapability } = await import("./registry");
  const fetch = vi
    .fn()
    .mockImplementation(() =>
      Promise.resolve(new Response('{"items":[],"has_more":false}')),
    );
  vi.stubGlobal("fetch", fetch);
  document.cookie = "it_wiki_csrf=governance-csrf; path=/";
  const signal = new AbortController().signal;
  await listCatalog({ q: "pending&revision=1" }, signal);
  expect(new URL(fetch.mock.calls[0][0]).pathname).toBe("/api/v1/catalog");
  expect(fetch.mock.calls[0][1].signal).toBe(signal);
  await actOnCapability("safe-slug", "approve", {
    revision: 7,
    version_id: "reviewed-version",
    reason: "approved",
  });
  const [url, options] = fetch.mock.calls[1];
  expect(url).toContain("/api/v1/capabilities/safe-slug/approve");
  expect(options).toMatchObject({
    method: "POST",
    credentials: "include",
    headers: { "X-CSRF-Token": "governance-csrf" },
  });
  expect(JSON.parse(options.body)).toEqual({
    revision: 7,
    version_id: "reviewed-version",
    reason: "approved",
  });
});
