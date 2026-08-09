const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

const CSRF_COOKIE = "it_wiki_csrf";
const CSRF_HEADER = "X-CSRF-Token";
const unauthorizedListeners = new Set<(path?: string) => void>();

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
  signal?: AbortSignal;
}

export function apiURL(path: string): string {
  return BASE + path;
}

export function subscribeToUnauthorized(listener: (path?: string) => void): () => void {
  unauthorizedListeners.add(listener);
  return () => unauthorizedListeners.delete(listener);
}

export function apiRequestInit(opts: FetchOptions = {}): RequestInit {
  const method = opts.method ?? "GET";
  const csrf = isSafeMethod(method) ? "" : readCookie(CSRF_COOKIE);
  return {
    method,
    credentials: "include",
    headers: {
      ...(opts.body && !(opts.body instanceof FormData) ? { "Content-Type": "application/json" } : {}),
      ...(csrf ? { [CSRF_HEADER]: csrf } : {}),
      ...(opts.headers ?? {}),
    },
    body: opts.body,
    signal: opts.signal,
  };
}

export async function apiErrorFromResponse(res: Response): Promise<APIError> {
  let parsed: { error?: { code: string; message: string; details?: Record<string, unknown>; request_id?: string } } = {};
  try {
    parsed = await res.json();
  } catch {
    // ignore parse failure
  }
  const error = parsed.error ?? { code: "internal_error", message: res.statusText };
  return new APIError(res.status, error.code, error.message, error.details, error.request_id);
}

export async function apiFetch<T>(path: string, opts: FetchOptions = {}): Promise<T> {
  const res = await fetch(apiURL(path), apiRequestInit(opts));

  if (res.status === 204) {
    return undefined as T;
  }

  if (!res.ok) {
    throw await apiErrorFromRejectedResponse(res, path);
  }

  return (await res.json()) as T;
}

export async function apiFetchList<T>(
  path: string,
  opts: FetchOptions = {},
): Promise<{ items: T[]; total: number }> {
  const res = await fetch(apiURL(path), apiRequestInit(opts));
  if (!res.ok) {
    throw await apiErrorFromRejectedResponse(res, path);
  }
  const total = parseInt(res.headers.get("X-Total-Count") ?? "0", 10);
  const items = (await res.json()) as T[];
  return { items: items ?? [], total };
}

export async function apiErrorFromRejectedResponse(res: Response, path?: string): Promise<APIError> {
  const error = await apiErrorFromResponse(res);
  if (error.status === 401) {
    unauthorizedListeners.forEach((listener) => listener(path));
  }
  return error;
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
