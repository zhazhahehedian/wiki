import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { Downloaders } from "./downloaders";
import { listDownloaders, type CapabilityDetail } from "@/lib/api/registry";

vi.mock("@/lib/api/registry", () => ({ listDownloaders: vi.fn() }));
beforeEach(() => vi.resetAllMocks());
const item = {
  slug: "search",
  download_count: 5,
  versions: [{ id: "v2", version: "2.0.0", has_bundle: true }],
} as unknown as CapabilityDetail;
it("lists who downloaded which version", async () => {
  vi.mocked(listDownloaders).mockResolvedValue({
    items: [{ open_id: "ou_a", display_name: "王同学", downloads: 3, last_downloaded_at: "2026-11-05T08:00:00Z", last_version_id: "v2" }],
    has_more: false,
  });
  render(<QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}><Downloaders item={item} /></QueryClientProvider>);
  expect(await screen.findByText("王同学")).toBeInTheDocument();
  expect(screen.getByText(/3 次 · 最近 .* · v2\.0\.0/)).toBeInTheDocument();
  expect(screen.getByText("共 5 次下载")).toBeInTheDocument();
});
