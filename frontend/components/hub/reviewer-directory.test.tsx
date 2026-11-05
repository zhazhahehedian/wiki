import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { ReviewerDirectory } from "./reviewer-directory";
import { listReviewers } from "@/lib/api/registry";

vi.mock("@/lib/api/registry", () => ({ listReviewers: vi.fn() }));
beforeEach(() => vi.resetAllMocks());
function mount() {
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><ReviewerDirectory /></QueryClientProvider>);
}
it("tells submitters who can review and that self review is allowed", async () => {
  vi.mocked(listReviewers).mockResolvedValue({ items: [{ open_id: "ou_leader", display_name: "李主管", is_admin: false }], has_more: false });
  mount();
  expect(await screen.findByText("李主管")).toBeInTheDocument();
  expect(screen.getByText(/允许审核自己的发布/)).toBeInTheDocument();
});
it("makes the absence of eligible reviewers explicit", async () => {
  vi.mocked(listReviewers).mockResolvedValue({ items: [], has_more: false });
  mount();
  expect(await screen.findByText(/当前没有可审核人员/)).toBeInTheDocument();
});
