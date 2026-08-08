const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

const CSRF_COOKIE = "it_wiki_csrf";
const CSRF_HEADER = "X-CSRF-Token";

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

export function apiURL(path: string): string {
  return BASE + path;
}

export async function apiFetch<T>(path: string, opts: FetchOptions = {}): Promise<T> {
  const method = opts.method ?? "GET";
  const csrf = isSafeMethod(method) ? "" : readCookie(CSRF_COOKIE);
  const res = await fetch(apiURL(path), {
    method,
    credentials: "include",
    headers: {
      ...(opts.body && !(opts.body instanceof FormData) ? { "Content-Type": "application/json" } : {}),
      ...(csrf ? { [CSRF_HEADER]: csrf } : {}),
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
  const res = await fetch(apiURL(path), {
    method: opts.method ?? "GET",
    credentials: "include",
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

function isSafeMethod(method: string): boolean {
  return ["GET", "HEAD", "OPTIONS"].includes(method.toUpperCase());
}

function readCookie(name: string): string {
  if (typeof document === "undefined") {
    return "";
  }
  const prefix = name + "=";
  for (const part of document.cookie.split(";")) {
    const cookie = part.trim();
    if (cookie.startsWith(prefix)) {
      return decodeURIComponent(cookie.slice(prefix.length));
    }
  }
  return "";
}
