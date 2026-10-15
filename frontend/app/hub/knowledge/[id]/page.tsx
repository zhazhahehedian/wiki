import { KnowledgeDocuments } from "@/components/knowledge/knowledge-documents";
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <KnowledgeDocuments id={id} />;
}
