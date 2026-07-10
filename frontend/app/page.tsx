"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { useKbs } from "@/lib/hooks/use-kbs";
import { getLastKbId } from "@/lib/last-kb";

export default function Home() {
  const router = useRouter();
  const { data, isError } = useKbs();

  useEffect(() => {
    if (!data) return;
    if (data.items.length === 0) {
      router.replace("/kbs");
      return;
    }
    const lastKbId = getLastKbId();
    const target = data.items.find((kb) => kb.id === lastKbId) ?? data.items[0];
    router.replace(`/kbs/${target.id}/chats`);
  }, [data, router]);

  useEffect(() => {
    if (isError) router.replace("/kbs");
  }, [isError, router]);

  return (
    <main className="flex min-h-screen items-center justify-center">
      <p className="text-sm text-muted-foreground">正在进入…</p>
    </main>
  );
}
