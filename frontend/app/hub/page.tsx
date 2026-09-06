"use client";

import Link from "next/link";
import {
  ArrowRight,
  Library,
  Send,
  FlaskConical,
  FilePlus2,
} from "lucide-react";
import { useAuth } from "@/lib/hooks/use-auth";
import { primaryClass } from "@/components/hub/registry-shared";

export default function Dashboard() {
  const { data: user } = useAuth();
  return (
    <div className="space-y-8">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            欢迎回来，{user?.display_name || "飞书用户"}
          </h1>
          <p className="mt-2 text-sm text-zinc-500">
            查找可用能力，管理发布，或测试模型。
          </p>
        </div>
        <Link href="/hub/registry/new" className={primaryClass}>
          <FilePlus2 className="size-4" />
          创建能力
        </Link>
      </div>
      <section aria-label="常用操作" className="grid gap-4 md:grid-cols-3">
        {[
          {
            title: "可用能力",
            text: "浏览你有权访问的 MCP 服务和 Skill，查看版本与接入说明。",
            href: "/hub/registry",
            icon: Library,
            action: "浏览注册中心",
          },
          {
            title: "我的发布",
            text: "继续编辑草稿、提交审核，查看已上线的版本。",
            href: "/hub/publications",
            icon: Send,
            action: "管理我的能力",
          },
          {
            title: "模型游乐场",
            text: "配置个人模型接口，调整参数并发起临时对话。",
            href: "/hub/playground",
            icon: FlaskConical,
            action: "打开游乐场",
          },
        ].map(({ title, text, href, icon: Icon, action }) => (
          <Link
            key={href}
            href={href}
            className="group flex flex-col rounded-xl border border-zinc-200 bg-white p-6 transition-colors hover:border-zinc-400 focus-visible:outline-2 focus-visible:outline-primary"
          >
            <Icon className="size-5 text-zinc-600" aria-hidden="true" />
            <h2 className="mt-5 font-semibold">{title}</h2>
            <p className="mt-2 flex-1 text-sm leading-7 text-zinc-500">
              {text}
            </p>
            <span className="mt-6 flex items-center gap-2 text-xs font-medium text-zinc-700">
              {action}
              <ArrowRight className="size-3.5 transition-transform group-hover:translate-x-1" />
            </span>
          </Link>
        ))}
      </section>
      <section className="rounded-xl border border-zinc-200 bg-white">
        <div className="border-b border-zinc-100 px-6 py-4">
          <h2 className="text-sm font-semibold">发布一份能力</h2>
        </div>
        <ol className="grid gap-6 p-6 md:grid-cols-3">
          {[
            ["创建草稿", "填写 MCP 服务信息，或上传 Skill 文件。"],
            ["提交审核", "确认版本和可见范围，由管理员审核。"],
            [
              "上线与维护",
              "审核通过后进入目录；新版审核期间，已上线版本继续可用。",
            ],
          ].map(([title, description], index) => (
            <li key={title} className="flex gap-3">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-zinc-100 text-xs text-zinc-600">
                {index + 1}
              </span>
              <div>
                <h3 className="text-sm font-medium">{title}</h3>
                <p className="mt-2 text-xs leading-6 text-zinc-500">
                  {description}
                </p>
              </div>
            </li>
          ))}
        </ol>
      </section>
    </div>
  );
}
