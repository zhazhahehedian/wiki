"use client";

import { useEffect, useState } from "react";
import { FileText, LoaderCircle } from "lucide-react";

export function GenerationProgress({ updating }: { updating: boolean }) {
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    const started = Date.now();
    const timer = window.setInterval(
      () => setSeconds(Math.floor((Date.now() - started) / 1000)),
      1000,
    );
    return () => window.clearInterval(timer);
  }, []);

  return (
    <div
      role="status"
      aria-label="Skill 生成进度"
      className="mt-5 rounded-2xl border border-blue-100 bg-blue-50/40 p-5 sm:p-7"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <span className="inline-flex items-center gap-2 text-sm font-medium text-slate-700">
          <LoaderCircle
            aria-hidden="true"
            className="size-4 motion-safe:animate-spin"
          />
          {updating ? "正在更新 Skill 草案" : "正在准备 Skill 草案"}
        </span>
        <span
          aria-hidden="true"
          className="text-xs tabular-nums text-slate-500"
        >
          已等待 {seconds} 秒
        </span>
      </div>
      {!updating && (
        <div
          aria-hidden="true"
          className="mx-auto my-8 max-w-sm rounded-xl border border-zinc-200/80 bg-white p-6 shadow-sm motion-safe:animate-pulse"
        >
          <FileText className="mb-6 size-8 text-slate-400" />
          <div className="mb-5 h-3 w-2/3 rounded-full bg-slate-200" />
          <div className="space-y-3">
            <div className="h-2 rounded-full bg-zinc-100" />
            <div className="h-2 w-5/6 rounded-full bg-zinc-100" />
            <div className="h-2 w-4/5 rounded-full bg-zinc-100" />
          </div>
        </div>
      )}
      <p className="mt-4 text-sm leading-6 text-slate-600">
        {seconds >= 45
          ? "这次等待较久，仍在等待模型完成。复杂草案可能需要几分钟，你可以随时停止生成。"
          : "模型正在整理你的需求，完成后会返回补充问题或可编辑的草案。"}
      </p>
      <p className="mt-2 text-xs leading-5 text-slate-500">
        {updating
          ? "新结果通过检查后才会替换下方草案，你之前的修改会保留。"
          : "完整结果通过检查后会显示在这里；等待期间请保留本页。"}
      </p>
    </div>
  );
}
