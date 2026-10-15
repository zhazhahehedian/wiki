import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, it, vi } from "vitest";
import { MarketDetailPage } from "./market-detail";
import {
  getCatalogCapability,
  getSkillFiles,
  type CapabilityDetail,
} from "@/lib/api/registry";
import { APIError } from "@/lib/api/client";
vi.mock("@/lib/api/registry", async (original) => ({
  ...(await original<typeof import("@/lib/api/registry")>()),
  getCatalogCapability: vi.fn(),
  getSkillFiles: vi.fn(),
}));
const detail = {
  slug: "weekly-report",
  name: "团队周报",
  description: "用于团队汇报",
  type: "skill",
  owner_open_id: "ou_author",
  department: "研发",
  status: "published",
  current_version_id: "live",
  versions: [{ id: "live", version: "1.2.0", has_bundle: true }],
} as CapabilityDetail;
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  // A cached management response must never be selected by the market.
  client.setQueryData(["registry", "detail", detail.slug], {
    ...detail,
    name: "秘密草稿",
    draft_version_id: "draft",
  });
  return render(
    <QueryClientProvider client={client}>
      <MarketDetailPage slug={detail.slug} />
    </QueryClientProvider>,
  );
}
beforeEach(() => {
  vi.mocked(getCatalogCapability).mockReset().mockResolvedValue(detail);
  vi.mocked(getSkillFiles)
    .mockReset()
    .mockResolvedValue({
      files: [
        {
          name: "SKILL.md",
          size: 80,
          content:
            "---\nname: weekly-report\ndescription: report\n---\n# 输入与输出\n[模板](references/example.md)\n![远程图片](https://example.test/tracking.png)",
        },
        {
          name: "references/example.md",
          size: 20,
          content: "## 输出示例\n完成情况与风险",
        },
      ],
    });
});
it("uses published details, pins the template and download, and previews attachments without remote images", async () => {
  mount();
  expect(
    await screen.findByRole("heading", { name: "团队周报" }),
  ).toBeInTheDocument();
  expect(screen.queryByText("秘密草稿")).not.toBeInTheDocument();
  expect(screen.getByRole("link", { name: "基于此创建" })).toHaveAttribute(
    "href",
    "/hub/registry/create-skill?source=weekly-report&version=live",
  );
  expect(
    screen.getByRole("link", { name: "下载 Skill 包" }).getAttribute("href"),
  ).toContain("/weekly-report/versions/live/bundle");
  expect(
    await screen.findByRole("heading", { name: "输入与输出" }),
  ).toBeInTheDocument();
  expect(document.querySelector("img")).toBeNull();
  await userEvent.click(
    screen.getByRole("button", { name: /references\/example.md/ }),
  );
  expect(
    await screen.findByRole("heading", { name: "输出示例" }),
  ).toBeInTheDocument();
  expect(getSkillFiles).toHaveBeenCalledWith(
    "weekly-report",
    "live",
    expect.any(AbortSignal),
  );
});
it("does not expose template or download actions after access is denied", async () => {
  vi.mocked(getCatalogCapability).mockRejectedValue(
    new APIError(404, "capability_not_found", "能力或版本不存在"),
  );
  mount();
  expect(await screen.findByRole("alert")).toHaveTextContent(
    "能力或版本不存在",
  );
  expect(
    screen.queryByRole("link", { name: "基于此创建" }),
  ).not.toBeInTheDocument();
  expect(getSkillFiles).not.toHaveBeenCalled();
});
