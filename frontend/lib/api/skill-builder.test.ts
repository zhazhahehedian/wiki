import { afterEach, expect, it, vi } from "vitest";
import {
  generateSkill,
  saveGeneratedSkill,
  type SkillDraft,
} from "./skill-builder";
afterEach(() => {
  vi.unstubAllGlobals();
  document.cookie = "it_wiki_csrf=; Max-Age=0; path=/";
});
it("uses session/CSRF and propagates cancellation for both generation and save", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response("{}"));
  vi.stubGlobal("fetch", fetch);
  document.cookie = "it_wiki_csrf=builder-csrf; path=/";
  const controller = new AbortController();
  const input = {
    mode: "task" as const,
    model: "model",
    messages: [{ role: "user" as const, content: "周报" }],
    material: "",
  };
  await generateSkill(input, controller.signal);
  let [url, options] = fetch.mock.calls[0];
  expect(url).toContain("/api/v1/skill-builder/turn");
  expect(options).toMatchObject({
    method: "POST",
    credentials: "include",
    headers: {
      "X-CSRF-Token": "builder-csrf",
      "Content-Type": "application/json",
    },
    signal: controller.signal,
  });
  expect(JSON.parse(options.body)).toEqual(input);
  fetch.mockResolvedValue(new Response("{}"));
  const draft = { slug: "weekly" } as SkillDraft;
  await saveGeneratedSkill(draft, controller.signal);
  [url, options] = fetch.mock.calls[1];
  expect(url).toContain("/api/v1/skill-builder/drafts");
  expect(JSON.parse(options.body)).toEqual({ draft });
  expect(options.signal).toBe(controller.signal);
});
