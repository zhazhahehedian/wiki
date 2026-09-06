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
}
export interface BuilderReply {
  message: string;
  draft: SkillDraft | null;
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
