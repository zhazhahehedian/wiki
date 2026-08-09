"use client";

import { CircleAlert, LoaderCircle, RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";

export function AuthErrorState({
  onRetry,
  retrying,
}: {
  onRetry: () => void;
  retrying: boolean;
}) {
  return (
    <main
      className="flex min-h-screen flex-col items-center justify-center gap-4 px-5 text-center"
      role="alert"
    >
      <CircleAlert className="size-7 text-destructive" aria-hidden="true" />
      <div className="space-y-1">
        <h1 className="text-lg font-semibold">无法验证登录状态</h1>
        <p className="text-sm text-muted-foreground">登录服务暂时不可用，请稍后重试。</p>
      </div>
      <Button type="button" variant="outline" onClick={onRetry} disabled={retrying}>
        {retrying ? (
          <LoaderCircle className="animate-spin" aria-hidden="true" />
        ) : (
          <RefreshCw aria-hidden="true" />
        )}
        {retrying ? "正在重试" : "重试"}
      </Button>
    </main>
  );
}
