import { MarketDetailPage } from "@/components/hub/market-detail";
export default async function Page({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  return <MarketDetailPage slug={slug} />;
}
