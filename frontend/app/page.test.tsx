import { expect, it, vi } from "vitest";
import Home from "./page";
import RetiredKnowledgeBasePage from "./kbs/[[...path]]/page";
const redirect = vi.fn();
vi.mock("next/navigation", () => ({ redirect: (path: string) => redirect(path) }));
it("routes the root and legacy bookmarks to Capability Hub", () => {
  Home();
  RetiredKnowledgeBasePage();
  expect(redirect.mock.calls).toEqual([["/hub"], ["/hub"]]);
});
