import { apiFetchList } from "./client";
import type { Chunk } from "@/lib/schemas";

export const chunkApi = {
  async listByDoc(docId: string, limit = 20, offset = 0): Promise<{ items: Chunk[]; total: number }> {
    return apiFetchList<Chunk>(`/api/v1/docs/${docId}/chunks?limit=${limit}&offset=${offset}`);
  },
};
