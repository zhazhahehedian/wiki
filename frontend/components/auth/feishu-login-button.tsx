"use client";

import { LogIn } from "lucide-react";

import { buttonVariants } from "@/components/ui/button";
import { authApi } from "@/lib/api/auth";
import { cn } from "@/lib/utils";

export function FeishuLoginButton() {
  return (
    <a href={authApi.feishuStartURL()} className={cn(buttonVariants(), "w-full")}>
      <LogIn data-icon="inline-start" />
      使用飞书登录
    </a>
  );
}
