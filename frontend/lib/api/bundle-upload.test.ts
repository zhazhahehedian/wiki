import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { forgetBundleUpload, uploadBundle, type BundleUpload } from "./bundle-upload";

const MiB = 1024 * 1024;
const partSize = 8 * MiB;

function plan(over: Partial<BundleUpload> = {}): BundleUpload {
  return {
    id: "u1",
    file_name: "pkg.zip",
    size: 20 * MiB,
    part_size: partSize,
    part_count: 3,
    received_parts: [],
    completed: false,
    expires_at: "2026-11-07T00:00:00Z",
    ...over,
  };
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status });
}

function packageFile() {
  return new File([new Uint8Array(20 * MiB)], "pkg.zip", { lastModified: 1 });
}

beforeEach(() => {
  localStorage.clear();
  document.cookie = "it_wiki_csrf=test-csrf; path=/";
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  document.cookie = "it_wiki_csrf=; Max-Age=0; path=/";
});

it("uploads every part with CSRF, then completes", async () => {
  const done = plan({ completed: true, bundle_sha256: "a".repeat(64) });
  const fetch = vi.fn(async (url: string, init: RequestInit) => {
    if (url.endsWith("/bundle-uploads")) return json(plan(), 201);
    if (url.includes("/parts/")) return new Response(null, { status: 204 });
    if (url.endsWith("/complete")) return json(done);
    throw new Error(`unexpected ${init.method} ${url}`);
  });
  vi.stubGlobal("fetch", fetch);
  const file = packageFile();
  const progress: number[] = [];
  const result = await uploadBundle(file, (p) => progress.push(p.uploadedBytes), new AbortController().signal);
  expect(result).toEqual(done);
  const parts = fetch.mock.calls.filter(([url]) => url.includes("/parts/"));
  expect(parts.map(([url]) => url.split("/").pop()).sort()).toEqual(["1", "2", "3"]);
  const [, init] = parts.find(([url]) => url.endsWith("/parts/3"))!;
  expect(init.method).toBe("PUT");
  expect((init.body as Blob).size).toBe(4 * MiB);
  expect(init.headers).toMatchObject({ "Content-Type": "application/octet-stream", "X-CSRF-Token": "test-csrf" });
  expect(progress.at(-1)).toBe(file.size);
  // The completed upload is remembered until the capability is saved.
  expect(localStorage.length).toBe(1);
  forgetBundleUpload(file);
  expect(localStorage.length).toBe(0);
});

it("resumes a remembered upload by sending only missing parts", async () => {
  const file = packageFile();
  const fetch = vi.fn(async (url: string) => {
    if (url.endsWith("/bundle-uploads")) return json(plan(), 201);
    if (url.endsWith("/bundle-uploads/u1")) return json(plan({ received_parts: [1, 3] }));
    if (url.includes("/parts/")) return new Response(null, { status: 204 });
    return json(plan({ completed: true }));
  });
  vi.stubGlobal("fetch", fetch);
  // A previous page session started this upload for the same file.
  localStorage.setItem(`caphub.bundle-upload:pkg.zip:${file.size}:1`, "u1");
  await uploadBundle(file, () => {}, new AbortController().signal);
  const urls = fetch.mock.calls.map(([url]) => url);
  expect(urls.some((url) => url.endsWith("/bundle-uploads"))).toBe(false);
  expect(urls.filter((url) => url.includes("/parts/"))).toEqual([expect.stringMatching(/\/parts\/2$/)]);
});

it("retries transient part failures but not rejected parts", async () => {
  vi.useFakeTimers();
  let failures = 2;
  const fetch = vi.fn(async (url: string) => {
    if (url.endsWith("/bundle-uploads")) return json(plan({ size: 100, part_count: 1 }), 201);
    if (url.includes("/parts/")) {
      if (failures-- > 0) throw new TypeError("network down");
      return new Response(null, { status: 204 });
    }
    return json(plan({ completed: true }));
  });
  vi.stubGlobal("fetch", fetch);
  const file = new File([new Uint8Array(100)], "small.zip");
  const run = uploadBundle(file, () => {}, new AbortController().signal);
  await vi.runAllTimersAsync();
  await expect(run).resolves.toMatchObject({ completed: true });
  expect(fetch.mock.calls.filter(([url]) => url.includes("/parts/"))).toHaveLength(3);

  localStorage.clear();
  fetch.mockImplementation(async (url: string) => {
    if (url.endsWith("/bundle-uploads")) return json(plan({ size: 100, part_count: 1 }), 201);
    return json({ error: { code: "invalid_capability", message: "分片无效" } }, 400);
  });
  fetch.mockClear();
  await expect(uploadBundle(file, () => {}, new AbortController().signal)).rejects.toMatchObject({ status: 400 });
  expect(fetch.mock.calls.filter(([url]) => url.includes("/parts/"))).toHaveLength(1);
});

it("starts over when the remembered upload expired", async () => {
  const file = packageFile();
  localStorage.setItem(`caphub.bundle-upload:pkg.zip:${file.size}:1`, "gone");
  const fetch = vi.fn(async (url: string) => {
    if (url.endsWith("/bundle-uploads/gone")) return json({ error: { code: "capability_not_found", message: "x" } }, 404);
    if (url.endsWith("/bundle-uploads")) return json(plan({ completed: true }), 201);
    throw new Error(url);
  });
  vi.stubGlobal("fetch", fetch);
  await expect(uploadBundle(file, () => {}, new AbortController().signal)).resolves.toMatchObject({ id: "u1" });
  expect(localStorage.getItem(`caphub.bundle-upload:pkg.zip:${file.size}:1`)).toBe("u1");
});
