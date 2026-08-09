"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useParams, usePathname, useRouter } from "next/navigation";
import type { ReactNode } from "react";

import { ChatPanel } from "@/components/layout/chat-panel";
import { KbPanel } from "@/components/layout/kb-panel";
import { Rail } from "@/components/layout/rail";
import { Button } from "@/components/ui/button";
import { Sidebar, SidebarContent, SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { APIError, subscribeToUnauthorized } from "@/lib/api/client";
import { useAuth } from "@/lib/hooks/use-auth";
import { setLastKbId } from "@/lib/last-kb";

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const params = useParams<{ kbId?: string }>();
  const router = useRouter();
  const queryClient = useQueryClient();
  const auth = useAuth();
  const [sessionExpired, setSessionExpired] = useState(false);
  const isChats = pathname.includes("/chats");

  useEffect(() => {
    if (params.kbId) setLastKbId(params.kbId);
  }, [params.kbId]);

  useEffect(() => subscribeToUnauthorized(() => {
    queryClient.removeQueries({ queryKey: ["auth"] });
    setSessionExpired(true);
  }), [queryClient]);

  const unauthenticated = sessionExpired
    || (auth.isError && auth.error instanceof APIError && auth.error.status === 401);
  useEffect(() => {
    if (unauthenticated) router.replace("/login");
  }, [router, unauthenticated]);

  if (auth.isLoading) {
    return <AuthState message="正在验证登录状态…" />;
  }

  if (unauthenticated) {
    return <AuthState message="正在跳转登录…" />;
  }

  if (auth.isError) {
    return (
      <AuthState message="登录状态检查失败，请稍后重试。">
        <Button variant="outline" onClick={() => auth.refetch()}>重试</Button>
      </AuthState>
    );
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <Rail />
      {/* translate-x-0 让 SidebarProvider 成为内部 fixed 定位 Sidebar 的 containing block，避免面板锚定视口盖住 Rail；
          overflow-x-clip 裁掉收起态滑出 provider 左缘的面板残留，防止其压在 Rail 上拦截点击 */}
      <SidebarProvider className="min-h-0 min-w-0 flex-1 translate-x-0 overflow-x-clip">
        <Sidebar collapsible="offcanvas" className="border-r">
          <SidebarContent>{isChats ? <ChatPanel /> : <KbPanel />}</SidebarContent>
        </Sidebar>
        <SidebarInset className="flex min-w-0 flex-col overflow-hidden">{children}</SidebarInset>
      </SidebarProvider>
    </div>
  );
}

function AuthState({ children, message }: { children?: ReactNode; message: string }) {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-3 px-4 text-center">
      <p className="text-sm text-muted-foreground" role="status">{message}</p>
      {children}
    </main>
  );
}
