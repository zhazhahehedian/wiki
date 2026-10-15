import { apiFetch } from "./client";
export interface KnowledgeBase {
  id: string;
  name: string;
  description: string;
  visibility: string;
  department: string;
  members: string[];
  manage: boolean;
  application_shared: boolean;
  model: string;
  dim: number;
}
export interface KnowledgeDocument {
  id: string;
  title: string;
  status: string;
  error: string;
  active_version_id: string;
  pending_version_id: string;
}
export interface Locator {
  page?: number;
  paragraph?: number;
  paragraph_end?: number;
  line?: number;
  line_end?: number;
  heading?: string;
  part?: number;
}
export interface Block {
  content: string;
  locator: Locator;
}
export interface Version {
  id: string;
  title: string;
  status: string;
  error: string;
  model: string;
  dim: number;
  checksum: string;
  preview: {
    blocks: Block[];
    diagnostics: {
      code: string;
      message: string;
      page?: number;
      blocking: boolean;
    }[];
  };
}
export interface Match extends Block {
  knowledge_base_id: string;
  document_id: string;
  document_version_id: string;
  chunk_id: string;
  title: string;
  score: number;
}
export interface RetrievalInfo {
  strategy: string;
  rewrite: string;
  rerank: string;
  warnings: string[];
}
export interface Application {
  id: string;
  name: string;
  enabled: boolean;
  knowledge_base_ids: string[];
  credential_prefix: string;
  expires_at?: string;
  last_used_at?: string;
}
const root = "/api/v1/knowledge";
export const knowledge = {
  audit: (signal?: AbortSignal) =>
    apiFetch<
      {
        request_id: string;
        knowledge_base_ids: string[];
        application_id: string;
        status: string;
        result_count: number;
        duration_ms: number;
        created_at: string;
      }[]
    >(`${root}/audit`, { signal }),
  bases: (signal?: AbortSignal) =>
    apiFetch<KnowledgeBase[]>(`${root}/bases`, { signal }),
  saveBase: (id: string, body: Partial<KnowledgeBase>) =>
    apiFetch<KnowledgeBase>(`${root}/bases${id ? `/${id}` : ""}`, {
      method: id ? "PUT" : "POST",
      body: JSON.stringify(body),
    }),
  deleteBase: (id: string) =>
    apiFetch(`${root}/bases/${id}`, { method: "DELETE" }),
  documents: (id: string, signal?: AbortSignal) =>
    apiFetch<KnowledgeDocument[]>(`${root}/bases/${id}/documents`, { signal }),
  upload: (id: string, file: File, documentID = "") => {
    const body = new FormData();
    body.append("file", file);
    return apiFetch<KnowledgeDocument>(
      `${root}/bases/${id}/documents${documentID ? `/${documentID}/versions` : ""}`,
      { method: "POST", body },
    );
  },
  deleteDocument: (id: string) =>
    apiFetch(`${root}/documents/${id}`, { method: "DELETE" }),
  retry: (id: string) =>
    apiFetch(`${root}/documents/${id}/retry`, { method: "POST" }),
  preview: (id: string, signal?: AbortSignal) =>
    apiFetch<Version>(`${root}/versions/${id}`, { signal }),
  search: (ids: string[], query: string, signal?: AbortSignal) =>
    apiFetch<{ request_id: string; matches: Match[]; retrieval?: RetrievalInfo }>(`${root}/test-search`, {
      method: "POST",
      body: JSON.stringify({ knowledge_base_ids: ids, query, top_k: 5 }),
      signal,
    }),
  applications: (signal?: AbortSignal) =>
    apiFetch<Application[]>(`${root}/applications`, { signal }),
  saveApplication: (
    id: string,
    body: Pick<Application, "name" | "enabled" | "knowledge_base_ids">,
  ) =>
    apiFetch<Application>(`${root}/applications${id ? `/${id}` : ""}`, {
      method: id ? "PUT" : "POST",
      body: JSON.stringify(body),
    }),
  rotate: (id: string) =>
    apiFetch<{ token: string; prefix: string; expires_at: string }>(
      `${root}/applications/${id}/credentials`,
      { method: "POST" },
    ),
  revoke: (id: string) =>
    apiFetch(`${root}/applications/${id}/credentials`, { method: "DELETE" }),
};
export function locationLabel(l: Locator) {
  return [
    l.heading,
    l.page && `第 ${l.page} 页`,
    l.paragraph &&
      `第 ${l.paragraph}${l.paragraph_end && l.paragraph_end !== l.paragraph ? `–${l.paragraph_end}` : ""} 段`,
    l.line &&
      `第 ${l.line}${l.line_end && l.line_end !== l.line ? `–${l.line_end}` : ""} 行`,
    l.part && `片段 ${l.part}`,
  ]
    .filter(Boolean)
    .join(" · ");
}
