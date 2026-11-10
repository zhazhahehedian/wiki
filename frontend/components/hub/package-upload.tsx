"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { APIError } from "@/lib/api/client";
import {
  discardBundleUpload,
  MAX_MCP_PACKAGE_BYTES,
  sameFile,
  uploadBundle,
  type BundleUpload,
  type UploadProgress,
} from "@/lib/api/bundle-upload";
import { secondaryClass } from "./registry-shared";

export type PackageUploadState =
  | { status: "idle" }
  | { status: "uploading" | "paused"; progress: UploadProgress }
  | { status: "error"; progress: UploadProgress; message: string }
  | { status: "done"; upload: BundleUpload };

const mib = (n: number) => (n / 1024 / 1024).toFixed(1);

export function validPackageFile(file: File) {
  return file.name.toLowerCase().endsWith(".zip") && file.size > 0 && file.size <= MAX_MCP_PACKAGE_BYTES;
}

/** Uploads one MCP package in resumable parts as soon as it is selected. */
export function usePackageUpload() {
  const [file, setFile] = useState<File>();
  const [state, setState] = useState<PackageUploadState>({ status: "idle" });
  const controller = useRef<AbortController>(undefined);
  const latest = useRef<File>(undefined);

  const run = useCallback((target: File) => {
    controller.current?.abort();
    const abort = new AbortController();
    controller.current = abort;
    latest.current = target;
    let last: UploadProgress = { uploadedBytes: 0, totalBytes: target.size, phase: "preparing" };
    setState({ status: "uploading", progress: last });
    uploadBundle(
      target,
      (progress) => {
        last = progress;
        if (latest.current === target && !abort.signal.aborted)
          setState({ status: "uploading", progress });
      },
      abort.signal,
    ).then(
      (upload) => {
        if (latest.current === target) setState({ status: "done", upload });
      },
      (err: unknown) => {
        if (latest.current !== target) return;
        if (abort.signal.aborted) {
          setState({ status: "paused", progress: last });
          return;
        }
        const message =
          err instanceof APIError
            ? err.message
            : "网络中断，已上传的分片会保留；请检查网络后继续上传。";
        setState({ status: "error", progress: last, message });
      },
    );
  }, []);

  useEffect(() => () => controller.current?.abort(), []);

  return {
    file,
    state,
    select(next: File | undefined) {
      controller.current?.abort();
      latest.current = undefined;
      // A replaced selection's upload would otherwise count against the quota.
      if (file && (!next || !sameFile(file, next))) void discardBundleUpload(file);
      setFile(next);
      setState({ status: "idle" });
      if (next && validPackageFile(next)) run(next);
    },
    pause() {
      controller.current?.abort();
    },
    resume() {
      if (file) run(file);
    },
    discard() {
      controller.current?.abort();
      latest.current = undefined;
      if (file) void discardBundleUpload(file);
      setFile(undefined);
      setState({ status: "idle" });
    },
  };
}

export function PackageUploadStatus({
  file,
  state,
  onPause,
  onResume,
}: {
  file: File;
  state: PackageUploadState;
  onPause: () => void;
  onResume: () => void;
}) {
  if (!validPackageFile(file))
    return (
      <p role="alert" className="text-xs text-red-700">
        请选择一个不超过 500 MiB 的 ZIP 程序包。
      </p>
    );
  if (state.status === "idle") return null;
  if (state.status === "done") {
    const findings = state.upload.bundle_findings?.length ?? 0;
    return (
      <div className="space-y-1 text-xs leading-6 text-zinc-600">
        <p>
          已上传并校验：{file.name}（{mib(file.size)} MiB，SHA-256 {state.upload.bundle_sha256?.slice(0, 12)}…）。保存后替换该版本的完整程序包。
        </p>
        {findings > 0 && (
          <p className="text-amber-700">
            检测到 {findings} 处疑似凭证或密钥文件，审核员会看到提示；请确认未打包真实密钥。
          </p>
        )}
      </div>
    );
  }
  const { uploadedBytes, totalBytes, phase } = state.progress;
  const percent = totalBytes ? Math.floor((uploadedBytes / totalBytes) * 100) : 0;
  const verifying = state.status === "uploading" && phase === "verifying";
  const preparing = state.status === "uploading" && phase === "preparing";
  return (
    <div className="space-y-2 text-xs text-zinc-600">
      <div
        role="progressbar"
        aria-label="程序包上传进度"
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent}
        className="h-2 overflow-hidden rounded-full bg-zinc-100"
      >
        <div className="h-full bg-zinc-900 transition-all" style={{ width: `${percent}%` }} />
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span>
          {verifying
            ? "正在校验程序包，请稍候…"
            : preparing
              ? `${file.name}：正在检查已上传的分片…`
              : `${file.name}：已上传 ${mib(uploadedBytes)} / ${mib(totalBytes)} MiB（${percent}%）`}
        </span>
        {state.status === "uploading" && !verifying && !preparing && (
          <button type="button" className={secondaryClass} onClick={onPause}>
            暂停
          </button>
        )}
        {(state.status === "paused" || state.status === "error") && (
          <button type="button" className={secondaryClass} onClick={onResume}>
            继续上传
          </button>
        )}
      </div>
      {state.status === "paused" && (
        <p>已暂停。已上传的分片保留 12 小时；刷新页面后重新选择同一文件也会从断点继续。</p>
      )}
      {state.status === "error" && (
        <p role="alert" className="text-red-700">
          {state.message}
        </p>
      )}
    </div>
  );
}
