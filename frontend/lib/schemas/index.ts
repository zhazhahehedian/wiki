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

export const docSyncStatusEnum = z.enum(["idle", "syncing", "failed"]);
export type DocSyncStatus = z.infer<typeof docSyncStatusEnum>;

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
  source_url: z.string().nullable().optional(),
  remote_revision: z.string().nullable().optional(),
  sync_status: docSyncStatusEnum.optional(),
  last_sync_error: z.string().nullable().optional(),
  last_synced_at: z.string().nullable().optional(),
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

export const citationSchema = z.object({
  id: z.string(),
  chunk_id: z.string(),
  document_id: z.string(),
  document_title: z.string(),
  seq: z.number(),
  score: z.number(),
  snippet: z.string(),
});
export type Citation = z.infer<typeof citationSchema>;

export const conversationSchema = z.object({
  id: z.string(),
  kb_id: z.string(),
  title: z.string(),
  mode: z.enum(["rag", "react"]),
  user_id: z.string(),
  created_at: z.string(),
  updated_at: z.string(),
});
export type Conversation = z.infer<typeof conversationSchema>;

export const toolCallStepSchema = z.object({
  step: z.number(),
  id: z.string(),
  name: z.string(),
  thought: z.string().optional(),
  arguments: z.unknown().optional(),
  result: z.string().optional(),
  duration_ms: z.number().optional(),
  error: z.string().optional(),
});
export type ToolCallStep = z.infer<typeof toolCallStepSchema>;

export const chatMessageSchema = z.object({
  id: z.string(),
  conversation_id: z.string(),
  role: z.enum(["user", "assistant"]),
  content: z.string(),
  citations: z.array(citationSchema).default([]),
  tool_calls: z.array(toolCallStepSchema).default([]),
  token_usage: z.record(z.string(), z.unknown()).default({}),
  created_at: z.string(),
});
export type ChatMessage = z.infer<typeof chatMessageSchema>;

export const neighborChunkSchema = z.object({
  id: z.string(),
  document_id: z.string(),
  seq: z.number(),
  content: z.string(),
  is_primary: z.boolean(),
});
export type NeighborChunk = z.infer<typeof neighborChunkSchema>;

export const chunkNeighborsSchema = z.object({
  primary_chunk_id: z.string(),
  window: z.number(),
  chunks: z.array(neighborChunkSchema),
});
export type ChunkNeighbors = z.infer<typeof chunkNeighborsSchema>;
