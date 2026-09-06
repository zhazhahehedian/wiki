import {
  ReviewsPage,
  AuditPage,
  ProfilesPage,
} from "@/components/hub/governance-pages";
import { notFound } from "next/navigation";
import { PlaygroundPage } from "@/components/playground/playground-page";
import { RegistryOverview } from "@/components/hub/registry-overview";
import { HubSettings } from "@/components/hub/hub-settings";

export default async function SectionPage({
  params,
}: {
  params: Promise<{ section: string }>;
}) {
  const { section } = await params;
  if (section === "settings") return <HubSettings />;
  if (section === "playground") return <PlaygroundPage />;
  if (section === "registry") return <RegistryOverview />;
  if (section === "publications") return <RegistryOverview mine />;
  if (section === "reviews") return <ReviewsPage />;
  if (section === "audit") return <AuditPage />;
  if (section === "profiles") return <ProfilesPage />;
  notFound();
}
