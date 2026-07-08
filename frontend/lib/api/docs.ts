import { apiFetch, apiFetchList } from "./client";
import type { Doc, DocStatus } from "@/lib/schemas";

export const docApi = {
  async upload(kbId: string, file: File): Promise<Doc> {
    const form = new FormData();
    form.append("file", file);
    return apiFetch<Doc>(`/api/v1/kbs/${kbId}/docs`, {
      method: "POST",
      body: form,
    });
  },
  async listByKB(
    kbId: string,
    opts: { status?: DocStatus; limit?: number; offset?: number } = {},
  ): Promise<{ items: Doc[]; total: number }> {
    const qs = new URLSearchParams();
    qs.set("limit", String(opts.limit ?? 20));
    qs.set("offset", String(opts.offset ?? 0));
    if (opts.status) qs.set("status", opts.status);
    return apiFetchList<Doc>(`/api/v1/kbs/${kbId}/docs?${qs}`);
  },
  async get(id: string): Promise<Doc> {
    return apiFetch<Doc>(`/api/v1/docs/${id}`);
  },
  async delete(id: string): Promise<void> {
    await apiFetch<void>(`/api/v1/docs/${id}`, { method: "DELETE" });
  },
  async reingest(id: string): Promise<Doc> {
    return apiFetch<Doc>(`/api/v1/docs/${id}/reingest`, { method: "POST" });
  },
};
