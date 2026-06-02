import { z } from "zod";

export const kbSchema = z.object({
  id: z.string(),
  name: z.string(),
  description: z.string(),
  owner_id: z.string(),
  embed_model: z.string(),
  embed_dim: z.number(),
  settings: z.record(z.string(), z.unknown()).default({}),
  created_at: z.string(),
  updated_at: z.string(),
});
export type KB = z.infer<typeof kbSchema>;

export const docStatusEnum = z.enum([
  "pending", "parsing", "chunking", "embedding", "ready", "failed",
]);
export type DocStatus = z.infer<typeof docStatusEnum>;

export const docSchema = z.object({
  id: z.string(),
  kb_id: z.string(),
  source_type: z.string(),
  source_ref: z.string(),
  title: z.string(),
  mime_type: z.string(),
  bytes: z.number(),
  checksum: z.string(),
  status: docStatusEnum,
  error_message: z.string().nullable().optional(),
  metadata: z.record(z.string(), z.unknown()).default({}),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Doc = z.infer<typeof docSchema>;

export const chunkSchema = z.object({
  id: z.string(),
  kb_id: z.string(),
  document_id: z.string(),
  seq: z.number(),
  content: z.string(),
  token_count: z.number(),
  metadata: z.record(z.string(), z.unknown()).default({}),
  created_at: z.string(),
});
export type Chunk = z.infer<typeof chunkSchema>;

export const apiErrorSchema = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    request_id: z.string().optional(),
    details: z.record(z.string(), z.unknown()).optional(),
  }),
});
