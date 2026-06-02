const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export class APIError extends Error {
  code: string;
  status: number;
  details?: Record<string, unknown>;
  requestId?: string;
  constructor(status: number, code: string, message: string, details?: Record<string, unknown>, requestId?: string) {
    super(message);
    this.code = code;
    this.status = status;
    this.details = details;
    this.requestId = requestId;
  }
}

export interface FetchOptions {
  method?: string;
  body?: BodyInit | null;
  headers?: Record<string, string>;
}

export async function apiFetch<T>(path: string, opts: FetchOptions = {}): Promise<T> {
  const res = await fetch(BASE + path, {
    method: opts.method ?? "GET",
    headers: {
      ...(opts.body && !(opts.body instanceof FormData) ? { "Content-Type": "application/json" } : {}),
      ...(opts.headers ?? {}),
    },
    body: opts.body,
  });

  if (res.status === 204) {
    return undefined as T;
  }

  if (!res.ok) {
    let parsed: { error?: { code: string; message: string; details?: Record<string, unknown>; request_id?: string } } = {};
    try {
      parsed = await res.json();
    } catch {
      // ignore parse failure
    }
    const e = parsed.error ?? { code: "internal_error", message: res.statusText };
    throw new APIError(res.status, e.code, e.message, e.details, e.request_id);
  }

  return (await res.json()) as T;
}

export async function apiFetchList<T>(
  path: string,
  opts: FetchOptions = {},
): Promise<{ items: T[]; total: number }> {
  const res = await fetch(BASE + path, {
    method: opts.method ?? "GET",
    headers: opts.headers,
  });
  if (!res.ok) {
    let body: { error?: { code: string; message: string; details?: Record<string, unknown>; request_id?: string } } = {};
    try {
      body = await res.json();
    } catch {
      // ignore
    }
    const e = body.error ?? { code: "internal_error", message: res.statusText };
    throw new APIError(res.status, e.code, e.message, e.details, e.request_id);
  }
  const total = parseInt(res.headers.get("X-Total-Count") ?? "0", 10);
  const items = (await res.json()) as T[];
  return { items: items ?? [], total };
}
