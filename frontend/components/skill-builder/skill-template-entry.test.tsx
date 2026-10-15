import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { SkillTemplateEntry } from "./skill-template-entry";
import { getSkillTemplate } from "@/lib/api/skill-builder";
import { APIError } from "@/lib/api/client";
vi.mock("@/lib/api/skill-builder", () => ({ getSkillTemplate: vi.fn() }));
vi.mock("./skill-builder-page", () => ({
  SkillBuilderPage: ({
    template,
  }: {
    template: { draft: { name: string } };
  }) => <p>{template.draft.name}</p>,
}));
it("waits for the authorized source and can retry without creating a partial draft", async () => {
  vi.mocked(getSkillTemplate).mockRejectedValueOnce(
    new APIError(409, "capability_conflict", "来源已更新"),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <SkillTemplateEntry slug="report" version="v1" />
    </QueryClientProvider>,
  );
  expect(screen.getByRole("status")).toHaveTextContent("正在读取");
  expect(await screen.findByRole("alert")).toHaveTextContent("来源已更新");
  expect(getSkillTemplate).toHaveBeenCalledWith(
    "report",
    "v1",
    expect.any(AbortSignal),
  );
  vi.mocked(getSkillTemplate).mockResolvedValue({
    source: { slug: "report", name: "报告", version: "1.0", version_id: "v1" },
    draft: {
      slug: "report-copy",
      name: "已完整复制",
      description: "报告",
      instructions: "正文",
      files: [],
    },
  });
  await userEvent.click(screen.getByRole("button", { name: "重新加载" }));
  expect(await screen.findByText("已完整复制")).toBeInTheDocument();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});
