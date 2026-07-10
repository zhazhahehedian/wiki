"use client";

import { useEffect } from "react";
import { useParams, usePathname } from "next/navigation";
import type { ReactNode } from "react";

import { ChatPanel } from "@/components/layout/chat-panel";
import { KbPanel } from "@/components/layout/kb-panel";
import { Rail } from "@/components/layout/rail";
import { Sidebar, SidebarContent, SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { setLastKbId } from "@/lib/last-kb";

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const params = useParams<{ kbId?: string }>();
  const isChats = pathname.includes("/chats");

  useEffect(() => {
    if (params.kbId) setLastKbId(params.kbId);
  }, [params.kbId]);

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
