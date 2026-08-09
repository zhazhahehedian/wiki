"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { APIError } from "@/lib/api/client";
import { useAuth } from "@/lib/hooks/use-auth";
import { useKbs } from "@/lib/hooks/use-kbs";
import { getLastKbId } from "@/lib/last-kb";

export default function Home() {
  const router = useRouter();
  const auth = useAuth();
  const authenticated = Boolean(auth.data) && !auth.isLoading && !auth.isError;
  const { data, isError, error } = useKbs(20, 0, authenticated);
  const unauthenticated = auth.isError
    && auth.error instanceof APIError
    && auth.error.status === 401;
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

  return (
    <main className="flex min-h-screen items-center justify-center">
      <p className="text-sm text-muted-foreground">正在进入…</p>
    </main>
  );
}
