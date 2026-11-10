import { apiFetch, APIError } from "./client";
import type { BundleFinding } from "./registry";

// Browser part uploads for large MCP packages. Parts are 8 MiB raw bodies;
// the server reports which arrived, so an interrupted upload resumes by
// sending only the missing parts, even after a page reload when the user
// selects the same file again.

export const MAX_MCP_PACKAGE_BYTES = 500 * 1024 * 1024;
const PART_CONCURRENCY = 3;
const PART_ATTEMPTS = 5;
const STORAGE_PREFIX = "caphub.bundle-upload:";

export interface BundleUpload {
  id: string;
  file_name: string;
  size: number;
  part_size: number;
  part_count: number;
  received_parts: number[];
  completed: boolean;
  bundle_sha256?: string;
  bundle_findings?: BundleFinding[];
  expires_at: string;
}

export interface UploadProgress {
  uploadedBytes: number;
  totalBytes: number;
  phase: "preparing" | "uploading" | "verifying" | "done";
}

const root = "/api/v1/bundle-uploads";

function fileKey(file: File) {
  return `${STORAGE_PREFIX}${file.name}:${file.size}:${file.lastModified}`;
}

function remembered(file: File): string | null {
  try {
    return window.localStorage.getItem(fileKey(file));
  } catch {
    return null;
  }
}

function remember(file: File, id: string | null) {
  try {
    if (id) window.localStorage.setItem(fileKey(file), id);
    else window.localStorage.removeItem(fileKey(file));
  } catch {
    // Without storage, resume still works within the page.
  }
}

/** Forget a saved upload so the same file is not matched to a consumed upload. */
export function forgetBundleUpload(file: File) {
  remember(file, null);
}

/** Cancel the upload remembered for file, if any; failures are left to expiry. */
export async function discardBundleUpload(file: File) {
  const id = remembered(file);
  remember(file, null);
  if (id)
    await apiFetch<void>(`${root}/${encodeURIComponent(id)}`, { method: "DELETE" }).catch(
      () => undefined,
    );
}

export function sameFile(a: File, b: File) {
  return fileKey(a) === fileKey(b);
}

function sleep(ms: number, signal: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      "abort",
      () => {
        clearTimeout(timer);
        reject(signal.reason);
      },
      { once: true },
    );
  });
}

// Network failures, timeouts, throttling and server errors are retried;
// other client errors (expired upload, invalid part) are not.
function retryable(err: unknown) {
  if (!(err instanceof APIError)) return true;
  return err.status >= 500 || err.status === 408 || err.status === 429;
}

async function sendPart(upload: BundleUpload, file: File, part: number, signal: AbortSignal) {
  const start = (part - 1) * upload.part_size;
  const body = file.slice(start, Math.min(start + upload.part_size, upload.size));
  for (let attempt = 1; ; attempt++) {
    try {
      await apiFetch<void>(`${root}/${encodeURIComponent(upload.id)}/parts/${part}`, {
        method: "PUT",
        body,
        headers: { "Content-Type": "application/octet-stream" },
        signal,
      });
      return body.size;
    } catch (err) {
      if (signal.aborted || attempt >= PART_ATTEMPTS || !retryable(err)) throw err;
      await sleep(1000 * 2 ** (attempt - 1), signal);
    }
  }
}

async function resumeOrStart(file: File, signal: AbortSignal): Promise<BundleUpload> {
  const id = remembered(file);
  if (id) {
    try {
      return await apiFetch<BundleUpload>(`${root}/${encodeURIComponent(id)}`, { signal });
    } catch (err) {
      if (!(err instanceof APIError) || err.status !== 404) throw err;
      remember(file, null);
    }
  }
  const created = await apiFetch<BundleUpload>(root, {
    method: "POST",
    body: JSON.stringify({ file_name: file.name, size: file.size }),
    signal,
  });
  remember(file, created.id);
  return created;
}

/**
 * Upload file in parts and return the completed upload. Aborting the signal
 * pauses; calling again with the same file resumes from the received parts.
 */
export async function uploadBundle(
  file: File,
  onProgress: (p: UploadProgress) => void,
  signal: AbortSignal,
): Promise<BundleUpload> {
  const upload = await resumeOrStart(file, signal);
  if (upload.completed) {
    onProgress({ uploadedBytes: file.size, totalBytes: file.size, phase: "done" });
    return upload;
  }
  const received = new Set(upload.received_parts);
  let uploadedBytes = 0;
  const missing: number[] = [];
  for (let part = 1; part <= upload.part_count; part++) {
    if (received.has(part)) {
      uploadedBytes += Math.min(upload.part_size, upload.size - (part - 1) * upload.part_size);
    } else {
      missing.push(part);
    }
  }
  onProgress({ uploadedBytes, totalBytes: file.size, phase: "uploading" });
  // A failed worker aborts the others so the user sees one error promptly.
  const workers = new AbortController();
  const stop = () => workers.abort(signal.reason);
  signal.addEventListener("abort", stop, { once: true });
  try {
    await Promise.all(
      Array.from({ length: Math.min(PART_CONCURRENCY, missing.length) }, async () => {
        for (let part = missing.shift(); part !== undefined; part = missing.shift()) {
          try {
            uploadedBytes += await sendPart(upload, file, part, workers.signal);
          } catch (err) {
            workers.abort(err);
            throw err;
          }
          onProgress({ uploadedBytes, totalBytes: file.size, phase: "uploading" });
        }
      }),
    );
  } finally {
    signal.removeEventListener("abort", stop);
  }
  onProgress({ uploadedBytes: file.size, totalBytes: file.size, phase: "verifying" });
  try {
    const done = await apiFetch<BundleUpload>(`${root}/${encodeURIComponent(upload.id)}/complete`, {
      method: "POST",
      signal,
    });
    onProgress({ uploadedBytes: file.size, totalBytes: file.size, phase: "done" });
    return done;
  } catch (err) {
    // The server discards an invalid package; start over next time.
    if (err instanceof APIError && err.status === 400) remember(file, null);
    throw err;
  }
}
