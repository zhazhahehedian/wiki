import { apiFetch, apiFetchList } from "./client";
import type { KB } from "@/lib/schemas";

export const kbApi = {
  async create(input: { name: string; description?: string }): Promise<KB> {
    return apiFetch<KB>("/api/v1/kbs", {
      method: "POST",
      body: JSON.stringify({ name: input.name, description: input.description ?? "" }),
    });
  },
  async list(limit = 20, offset = 0): Promise<{ items: KB[]; total: number }> {
    return apiFetchList<KB>(`/api/v1/kbs?limit=${limit}&offset=${offset}`);
  },
  async get(id: string): Promise<KB> {
    return apiFetch<KB>(`/api/v1/kbs/${id}`);
  },
  async delete(id: string): Promise<void> {
    await apiFetch<void>(`/api/v1/kbs/${id}`, { method: "DELETE" });
  },
};
