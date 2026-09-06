"use client";

import { useQuery } from "@tanstack/react-query";
import { getGovernanceIdentity } from "@/lib/api/registry";
import { useAuth } from "@/lib/hooks/use-auth";
import { getAuthenticatedUser } from "@/lib/auth-state";
import { useSessionExpired } from "@/components/auth/auth-session-boundary";
import { ConnectionSettings } from "@/components/playground/connection-settings";

export function HubSettings() {
  const auth = useAuth();
  const user = getAuthenticatedUser(auth, useSessionExpired());
  const governance = useQuery({
    queryKey: ["registry", "identity"],
    queryFn: ({ signal }) => getGovernanceIdentity(signal),
    enabled: !!user,
  });
  if (!user) return null;
  return (
    <div>
      <h1 className="text-2xl font-semibold tracking-tight">设置</h1>
      <p className="mt-2 text-sm text-zinc-500">你的账号与工作空间偏好。</p>
      <div className="mt-8 max-w-2xl">
        <ConnectionSettings />
      </div>
      <section className="mt-8 max-w-2xl rounded-xl border border-zinc-200 bg-white p-6">
        <h2 className="font-semibold">账号信息</h2>
        <p className="mt-2 text-xs text-zinc-500">账号信息由飞书提供。</p>
        <dl className="mt-6 divide-y divide-zinc-100 text-sm">
          <div className="flex flex-wrap justify-between gap-3 py-4">
            <dt className="text-zinc-500">姓名</dt>
            <dd className="break-all">{user.display_name || "未提供"}</dd>
          </div>
          <div className="flex flex-wrap justify-between gap-3 py-4">
            <dt className="text-zinc-500">邮箱</dt>
            <dd className="break-all">{user.email || "未提供"}</dd>
          </div>
          <div className="flex justify-between gap-3 py-4">
            <dt className="text-zinc-500">登录方式</dt>
            <dd>飞书</dd>
          </div>
          <div className="flex justify-between gap-3 py-4">
            <dt className="text-zinc-500">平台角色</dt>
            <dd>
              {governance.data
                ? governance.data.is_admin
                  ? "管理员"
                  : "普通用户 / 自有能力 Owner"
                : "尚未取得权限资料"}
            </dd>
          </div>
          <div className="flex justify-between gap-3 py-4">
            <dt className="text-zinc-500">可信部门</dt>
            <dd>{governance.data?.department || "未设置（联系管理员核实）"}</dd>
          </div>
        </dl>
      </section>
    </div>
  );
}
