"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { AuthErrorState } from "@/components/auth/auth-error-state";
import { useSessionExpired } from "@/components/auth/auth-session-boundary";
import { APIError } from "@/lib/api/client";
import { getAuthenticatedUser } from "@/lib/auth-state";
import { useAuth } from "@/lib/hooks/use-auth";
import { useKbs } from "@/lib/hooks/use-kbs";
import { getLastKbId } from "@/lib/last-kb";

export default function Home() {
  const router = useRouter();
  const sessionExpired = useSessionExpired();
  const auth = useAuth();
  const authenticated = Boolean(getAuthenticatedUser(auth, sessionExpired));
  const { data, isError, error } = useKbs(20, 0, authenticated);
  const authUnauthenticated = auth.isError
    && auth.error instanceof APIError
    && auth.error.status === 401;
  const unauthenticated = sessionExpired || authUnauthenticated;
  const authFailed = auth.isError && !authUnauthenticated;
  const kbUnauthenticated = isError
    && error instanceof APIError
    && error.status === 401;

  useEffect(() => {
    if (unauthenticated || kbUnauthenticated) {
      router.replace("/login?next=%2F");
    }
  }, [kbUnauthenticated, router, unauthenticated]);

  useEffect(() => {
    if (!authenticated || isError || !data) return;
    if (data.items.length === 0) {
      router.replace("/kbs");
      return;
    }
    const lastKbId = getLastKbId();
    const target = data.items.find((kb) => kb.id === lastKbId) ?? data.items[0];
    router.replace(`/kbs/${target.id}/chats`);
  }, [authenticated, data, isError, router]);

  useEffect(() => {
    if (authenticated && isError && !kbUnauthenticated) router.replace("/kbs");
  }, [authenticated, isError, kbUnauthenticated, router]);

  if (authFailed) {
    return <AuthErrorState onRetry={() => void auth.refetch()} retrying={auth.isFetching} />;
  }

  return (
    <main className="flex min-h-screen items-center justify-center">
      <p className="text-sm text-muted-foreground">正在进入…</p>
    </main>
  );
}
