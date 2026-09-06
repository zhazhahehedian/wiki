"use client";

import { Suspense, useEffect } from "react";
import { Boxes } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";

import { AuthErrorState } from "@/components/auth/auth-error-state";
import { useSessionExpired } from "@/components/auth/auth-session-boundary";
import { FeishuLoginButton } from "@/components/auth/feishu-login-button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { APIError } from "@/lib/api/client";
import { getAuthenticatedUser } from "@/lib/auth-state";
import { useAuth } from "@/lib/hooks/use-auth";

const LOGIN_ERRORS: Record<string, string> = {
  oauth_cancelled: "登录已取消，请重新发起登录。",
  oauth_state_invalid: "登录请求已失效，请重新发起登录。",
  feishu_reauth_required: "需要重新授权飞书只读权限，请重试。",
  auth_service_unavailable: "登录服务暂时不可用，请稍后重试。",
  invalid_state: "登录请求已失效，请重新发起登录。",
  tenant_not_allowed: "当前飞书账号不属于允许的组织。",
  insufficient_scope: "未授予所需的飞书只读权限，请重试。",
  oauth_failed: "飞书登录失败，请重试。",
};

export default function LoginPage() {
  return (
    <Suspense fallback={<LoginState message="正在检查登录状态…" />}>
      <LoginContent />
    </Suspense>
  );
}

function LoginContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const sessionExpired = useSessionExpired();
  const auth = useAuth();
  const authenticated = Boolean(getAuthenticatedUser(auth, sessionExpired));
  const unauthenticated =
    auth.isError && auth.error instanceof APIError && auth.error.status === 401;
  const authFailed = auth.isError && !unauthenticated;

  useEffect(() => {
    // Entering the app from login must wait for the current session check.
    if (authenticated && !auth.isFetching) router.replace("/");
  }, [authenticated, auth.isFetching, router]);

  if (authFailed) {
    return (
      <AuthErrorState
        onRetry={() => void auth.refetch()}
        retrying={auth.isFetching}
      />
    );
  }

  if (auth.isLoading || auth.isFetching) {
    return <LoginState message="正在检查登录状态…" />;
  }

  if (authenticated) {
    return <LoginState message="正在进入能力中心…" />;
  }

  const errorCode = searchParams.get("error");
  const errorMessage = errorCode
    ? (LOGIN_ERRORS[errorCode] ?? LOGIN_ERRORS.oauth_failed)
    : null;

  return (
    <main className="relative flex min-h-screen items-center justify-center overflow-hidden bg-slate-50 px-5 py-10">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -top-32 left-1/4 size-96 rounded-full bg-sky-100/60 blur-3xl"
      />
      <div
        aria-hidden="true"
        className="pointer-events-none absolute -bottom-32 right-1/4 size-96 rounded-full bg-indigo-100/40 blur-3xl"
      />
      <div className="relative w-full max-w-md space-y-7 rounded-3xl border border-white bg-white/90 px-7 py-10 shadow-[0_20px_80px_-40px_#94a3b8] sm:px-10">
        <div className="space-y-3 text-center">
          <div className="mx-auto mb-5 flex size-14 items-center justify-center rounded-2xl border border-sky-100 bg-sky-50 text-primary">
            <Boxes className="size-7" aria-hidden="true" />
          </div>
          <div className="space-y-1">
            <h1 className="text-xl font-semibold">登录能力中心</h1>
            <p className="text-sm text-muted-foreground">
              使用飞书账号，连接团队的能力与经验
            </p>
          </div>
        </div>

        {errorMessage && (
          <Alert variant="destructive">
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        )}

        <FeishuLoginButton />
        <p className="border-t border-slate-100 pt-5 text-center text-xs leading-6 text-slate-400">
          团队能力，共同创造
          <br />
          Capability Hub
        </p>
      </div>
    </main>
  );
}

function LoginState({ message }: { message: string }) {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <p className="text-sm text-muted-foreground" role="status">
        {message}
      </p>
    </main>
  );
}
