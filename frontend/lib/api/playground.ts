import {
  apiFetch,
  apiURL,
  apiRequestInit,
  apiErrorFromRejectedResponse,
} from "./client";

export type ModelProtocol = "openai" | "anthropic";
export interface ModelConnection {
  protocol: ModelProtocol;
  baseUrl: string;
  models: string[];
  defaultModel: string;
  hasKey: boolean;
  version: number;
}
export interface ConnectionInput {
  protocol: ModelProtocol;
  baseUrl: string;
  models: string[];
  defaultModel: string;
  apiKey: string;
  version: number;
}
export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
}

export interface ChatParameters {
  temperature?: number;
  top_p?: number;
  frequency_penalty?: number;
  presence_penalty?: number;
  max_tokens?: number;
}

const path = "/api/v1/playground/connection";
export const playgroundApi = {
  models: (
    input: { baseUrl: string; apiKey: string; protocol: ModelProtocol },
    signal?: AbortSignal,
  ) =>
    apiFetch<{ models: string[] }>("/api/v1/playground/models", {
      method: "POST",
      body: JSON.stringify(input),
      signal,
    }),
  get: (signal?: AbortSignal) =>
    apiFetch<ModelConnection | null>(path, { signal }),
  save: (input: ConnectionInput, signal?: AbortSignal) =>
    apiFetch<ModelConnection>(path, {
      method: "PUT",
      body: JSON.stringify(input),
      signal,
    }),
  delete: (signal?: AbortSignal) =>
    apiFetch<void>(path, { method: "DELETE", signal }),
};

export async function streamModelChat(
  input: { model: string; messages: ChatMessage[] } & ChatParameters,
  signal: AbortSignal,
  onDelta: (text: string) => void,
): Promise<void> {
  const path = "/api/v1/playground/chat/stream";
  const response = await fetch(
    apiURL(path),
    apiRequestInit({ method: "POST", body: JSON.stringify(input), signal }),
  );
  if (!response.ok) throw await apiErrorFromRejectedResponse(response, path);
  if (
    !response.body ||
    !response.headers.get("Content-Type")?.startsWith("text/event-stream")
  )
    throw new Error("模型返回了无效响应");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let done = false;
  function consume() {
    let end: number;
    while ((end = buffer.indexOf("\n\n")) >= 0) {
      const block = buffer.slice(0, end);
      buffer = buffer.slice(end + 2);
      const lines = block.split("\n");
      const event = lines
        .find((line) => line.startsWith("event:"))
        ?.slice(6)
        .trim();
      const data = lines
        .filter((line) => line.startsWith("data:"))
        .map((line) => line.slice(5).trimStart())
        .join("\n");
      if (event === "done") {
        done = true;
        return;
      }
      if (event === "error") throw new Error("模型响应中断，请重试");
      if (event === "delta") {
        const value: unknown = JSON.parse(data);
        if (
          !value ||
          typeof value !== "object" ||
          !("text" in value) ||
          typeof value.text !== "string"
        )
          throw new Error("模型返回了无效响应");
        onDelta(value.text);
      }
    }
  }
  try {
    while (!done) {
      const chunk = await reader.read();
      if (chunk.done) {
        buffer += decoder.decode();
        consume();
        break;
      }
      buffer += decoder.decode(chunk.value, { stream: true });
      // Our API emits LF; support CRLF without corrupting CR/LF split across chunks.
      buffer = buffer.replace(/\r\n/g, "\n");
      if (buffer.length > 1024 * 1024) throw new Error("模型响应过大");
      consume();
    }
    if (!done) throw new Error("模型响应中断，请重试");
  } finally {
    await reader.cancel().catch(() => undefined);
    reader.releaseLock();
  }
}
