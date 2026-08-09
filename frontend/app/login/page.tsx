"use client";

import { Suspense, useEffect } from "react";
import { BookOpen } from "lucide-react";
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
  const unauthenticated = auth.isError
    && auth.error instanceof APIError
    && auth.error.status === 401;
  const authFailed = auth.isError && !unauthenticated;

  useEffect(() => {
    if (authenticated) router.replace("/");
  }, [authenticated, router]);

  if (authFailed) {
    return <AuthErrorState onRetry={() => void auth.refetch()} retrying={auth.isFetching} />;
  }

  if (auth.isLoading || auth.isFetching) {
    return <LoginState message="正在检查登录状态…" />;
  }

  if (authenticated) {
    return <LoginState message="正在进入知识库…" />;
  }

  const errorCode = searchParams.get("error");
  const errorMessage = errorCode ? (LOGIN_ERRORS[errorCode] ?? LOGIN_ERRORS.oauth_failed) : null;

  return (
    <main className="flex min-h-screen items-center justify-center px-5 py-10">
      <div className="w-full max-w-sm space-y-6">
        <div className="space-y-3 text-center">
          <div className="mx-auto flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <BookOpen className="size-5" aria-hidden="true" />
          </div>
          <div className="space-y-1">
            <h1 className="text-xl font-semibold">登录 it-wiki</h1>
            <p className="text-sm text-muted-foreground">使用公司飞书账号继续</p>
          </div>
        </div>

        {errorMessage && (
          <Alert variant="destructive">
            <AlertDescription>{errorMessage}</AlertDescription>
          </Alert>
        )}

        <FeishuLoginButton />
      </div>
    </main>
  );
}

function LoginState({ message }: { message: string }) {
  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <p className="text-sm text-muted-foreground" role="status">{message}</p>
    </main>
  );
}
