import { SkillBuilderPage } from "@/components/skill-builder/skill-builder-page";
import { SkillTemplateEntry } from "@/components/skill-builder/skill-template-entry";
export default async function Page({
  searchParams,
}: {
  searchParams: Promise<{ source?: string; version?: string }>;
}) {
  const { source, version } = await searchParams;
  return source ? (
    <SkillTemplateEntry slug={source} version={version ?? ""} />
  ) : (
    <SkillBuilderPage />
  );
}
