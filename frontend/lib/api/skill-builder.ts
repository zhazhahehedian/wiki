import { apiFetch } from "@/lib/api/client";
import type { CapabilityDetail } from "@/lib/api/registry";
export interface SkillFile {
  path: string;
  content: string;
}
export interface SkillDraft {
  slug: string;
  name: string;
  description: string;
  instructions: string;
  files: SkillFile[];
}
export interface BuilderMessage {
  role: "user" | "assistant";
  content: string;
}
export interface BuilderRequest {
  mode: "task" | "perspective";
  model: string;
  messages: BuilderMessage[];
  material: string;
  draft?: SkillDraft;
  max_tokens?: number;
}
export interface BuilderReply {
  message: string;
  draft: SkillDraft | null;
}
export interface SkillTemplate {
  source: { slug: string; name: string; version: string; version_id: string };
  draft: SkillDraft;
}
export function getSkillTemplate(
  slug: string,
  version: string,
  signal?: AbortSignal,
) {
  const query = new URLSearchParams({ version_id: version });
  return apiFetch<SkillTemplate>(
    `/api/v1/catalog/${encodeURIComponent(slug)}/template?${query}`,
    { signal },
  );
}
export function generateSkill(input: BuilderRequest, signal: AbortSignal) {
  return apiFetch<BuilderReply>("/api/v1/skill-builder/turn", {
    method: "POST",
    body: JSON.stringify(input),
    signal,
  });
}
export function saveGeneratedSkill(draft: SkillDraft, signal: AbortSignal) {
  return apiFetch<CapabilityDetail>("/api/v1/skill-builder/drafts", {
    method: "POST",
    body: JSON.stringify({ draft }),
    signal,
  });
}
export function skillMarkdown(draft: SkillDraft) {
  return `---\nname: ${JSON.stringify(draft.slug)}\ndescription: ${JSON.stringify(draft.description)}\n---\n\n${draft.instructions}\n`;
}
