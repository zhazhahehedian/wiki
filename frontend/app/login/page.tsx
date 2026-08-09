"use client";

import { Suspense, useEffect } from "react";
import { BookOpen } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";

import { FeishuLoginButton } from "@/components/auth/feishu-login-button";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useAuth } from "@/lib/hooks/use-auth";

const LOGIN_ERRORS: Record<string, string> = {
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
  const auth = useAuth();

  useEffect(() => {
    if (auth.data) router.replace("/");
  }, [auth.data, router]);

  if (auth.isLoading) {
    return <LoginState message="正在检查登录状态…" />;
  }

  if (auth.data) {
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
