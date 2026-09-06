import { CapabilityEditorPage } from "@/components/hub/capability-editor";
export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <CapabilityEditorPage slug={slug} />;
}
