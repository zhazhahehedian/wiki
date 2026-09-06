import { CapabilityDetailPage } from "@/components/hub/capability-detail";
export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <CapabilityDetailPage slug={slug} />;
}
