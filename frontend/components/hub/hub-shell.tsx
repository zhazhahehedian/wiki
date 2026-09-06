"use client";

import { useEffect, useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import {
  Boxes,
  LayoutDashboard,
  Library,
  FlaskConical,
  Send,
  Settings,
  Menu,
  LogOut,
  ShieldCheck,
  ScrollText,
  ChevronRight,
} from "lucide-react";
import { getGovernanceIdentity } from "@/lib/api/registry";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { AuthErrorState } from "@/components/auth/auth-error-state";
import { useSessionExpired } from "@/components/auth/auth-session-boundary";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";
import { APIError } from "@/lib/api/client";
import { getAuthenticatedUser } from "@/lib/auth-state";
import { useAuth, useLogout } from "@/lib/hooks/use-auth";
import { cn } from "@/lib/utils";

const navigation = [
  { href: "/hub", label: "仪表盘", icon: LayoutDashboard },
  { href: "/hub/registry", label: "注册中心", icon: Library },
  { href: "/hub/playground", label: "游乐场", icon: FlaskConical },
  { href: "/hub/publications", label: "我的发布", icon: Send },
  { href: "/hub/settings", label: "设置", icon: Settings },
];

export function HubShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const queryClient = useQueryClient();
  const auth = useAuth();
  const logout = useLogout();
  const expired = useSessionExpired();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [signedOut, setSignedOut] = useState(false);
  const user = getAuthenticatedUser(auth, expired || signedOut);
  const governance = useQuery({
    queryKey: ["registry", "identity"],
    queryFn: ({ signal }) => getGovernanceIdentity(signal),
    enabled: !!user,
  });
  const unauthorized =
    expired ||
    signedOut ||
    (auth.isError &&
      auth.error instanceof APIError &&
      auth.error.status === 401);

  useEffect(() => {
    if (unauthorized) router.replace("/login");
  }, [unauthorized, router]);

  async function signOut() {
    try {
      await logout.mutateAsync();
      setSignedOut(true);
      queryClient.clear();
      router.replace("/login");
    } catch {
      toast.error("退出登录失败，请重试。");
    }
  }

  if (unauthorized) return <Status message="正在跳转登录…" />;
  if (auth.isError)
    return (
      <AuthErrorState
        onRetry={() => void auth.refetch()}
        retrying={auth.isFetching}
      />
    );
  if (!user) return <Status message="正在验证登录状态…" />;

  const isActive = (href: string) =>
    pathname === href || (href !== "/hub" && pathname.startsWith(`${href}/`));
  const title =
    navigation.find((item) => isActive(item.href))?.label ??
    {
      "/hub/reviews": "审核队列",
      "/hub/audit": "审计日志",
      "/hub/profiles": "部门授权",
    }[pathname] ??
    "能力中心";
  const sidebar = (
    <div className="flex h-full flex-col border-r border-zinc-200 bg-white text-zinc-600">
      <Link
        href="/hub"
        onClick={() => setMobileOpen(false)}
        className="flex items-center gap-3 px-5 py-6 text-zinc-900"
      >
        <span className="flex size-10 items-center justify-center rounded-lg bg-blue-600 text-white">
          <Boxes className="size-6" aria-hidden="true" />
        </span>
        <span>
          <span className="block text-base font-semibold tracking-tight">
            能力中心
          </span>
          <span className="block text-[10px] tracking-[0.18em] text-zinc-400">
            CAPABILITY HUB
          </span>
        </span>
      </Link>
      <p className="px-6 pb-3 pt-4 text-[10px] font-medium tracking-widest text-zinc-500">
        工作空间
      </p>
      <nav aria-label="主导航" className="space-y-1 px-3">
        {navigation.map(({ href, label, icon: Icon }) => (
          <Link
            key={href}
            href={href}
            aria-current={isActive(href) ? "page" : undefined}
            onClick={() => setMobileOpen(false)}
            className={cn(
              "flex items-center gap-3 rounded-lg px-3 py-2.5 text-[13px] transition-colors focus-visible:outline-2 focus-visible:outline-blue-400",
              isActive(href)
                ? "bg-blue-50 font-medium text-blue-700 hover:bg-blue-100"
                : "hover:bg-zinc-100 hover:text-zinc-900",
            )}
          >
            <Icon className="size-[18px]" aria-hidden="true" />
            {label}
            {isActive(href) && (
              <ChevronRight className="ml-auto size-3.5" aria-hidden="true" />
            )}
          </Link>
        ))}
      </nav>
      {governance.data?.is_admin && (
        <div className="mx-6 mt-8 border-t border-zinc-200 pt-5">
          <p className="text-[10px] tracking-widest text-zinc-500">治理</p>
          {[
            { href: "/hub/reviews", label: "审核队列", icon: ShieldCheck },
            { href: "/hub/audit", label: "审计日志", icon: ScrollText },
            { href: "/hub/profiles", label: "部门授权", icon: Settings },
          ].map(({ href, label, icon: Icon }) => (
            <Link
              key={href}
              href={href}
              onClick={() => setMobileOpen(false)}
              aria-current={isActive(href) ? "page" : undefined}
              className={cn(
                "mt-3 flex items-center gap-3 rounded-lg px-2 py-2 text-xs transition-colors focus-visible:outline-2 focus-visible:outline-blue-400",
                isActive(href)
                  ? "bg-blue-50 font-medium text-blue-700 hover:bg-blue-100"
                  : "text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900",
              )}
            >
              <Icon className="size-4" aria-hidden="true" />
              {label}
            </Link>
          ))}
        </div>
      )}
      <div className="mt-auto p-4">
        <div className="mt-4 flex min-w-0 items-center gap-3 px-2 py-2">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-zinc-100 text-xs font-semibold text-zinc-700">
            {(user.display_name || "用户").slice(0, 1)}
          </span>
          <span className="min-w-0 flex-1 truncate text-xs text-zinc-700">
            {user.display_name || "飞书用户"}
          </span>
          <button
            type="button"
            onClick={() => void signOut()}
            disabled={logout.isPending}
            aria-label="退出登录"
            className="rounded-md p-2 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-900 disabled:opacity-50"
          >
            <LogOut className="size-4" aria-hidden="true" />
          </button>
        </div>
      </div>
    </div>
  );

  return (
    <div className="flex min-h-screen bg-[#fafafa] text-zinc-900">
      <a
        href="#hub-content"
        className="sr-only z-[60] rounded bg-white p-3 focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
      >
        跳到主要内容
      </a>
      <aside className="fixed inset-y-0 left-0 hidden w-[220px] lg:block">
        {sidebar}
      </aside>
      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent
          side="left"
          className="gap-0 border-zinc-200 bg-white p-0 text-zinc-900"
          showCloseButton={true}
        >
          <SheetTitle className="sr-only">能力中心导航</SheetTitle>
          <SheetDescription className="sr-only">
            选择工作空间中的页面
          </SheetDescription>
          {sidebar}
        </SheetContent>
      </Sheet>
      <div className="min-w-0 flex-1 lg:pl-[220px]">
        <header className="flex h-16 items-center justify-between gap-4 border-b border-zinc-200 bg-white px-5 sm:px-7">
          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              size="icon"
              className="lg:hidden"
              aria-label="打开导航"
              onClick={() => setMobileOpen(true)}
            >
              <Menu className="size-5" />
            </Button>
            <span className="text-sm text-zinc-400">工作空间</span>
            <ChevronRight className="size-3 text-zinc-300" />
            <span className="text-sm font-medium">{title}</span>
          </div>
          <span className="rounded-full border border-zinc-200 px-3 py-1 text-[11px] text-zinc-500">
            内部平台
          </span>
        </header>
        <main
          id="hub-content"
          className="w-full px-5 py-6 sm:px-7 sm:py-7"
        >
          {children}
        </main>
      </div>
    </div>
  );
}

function Status({ message }: { message: string }) {
  return (
    <main className="flex min-h-screen items-center justify-center">
      <p role="status" className="text-sm text-muted-foreground">
        {message}
      </p>
    </main>
  );
}
