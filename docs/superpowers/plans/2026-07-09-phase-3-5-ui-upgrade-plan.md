# 阶段 3.5 · UI 视觉升级实施计划

> **适用范围（2026-09-05 规则整理）：** 本文属于转向前的 it-wiki 历史计划，保留阶段实施步骤与验收记录，不是能力中心实施计划或所有任务的常驻规则。仅执行本阶段相关工作时读取对应任务和依赖；旧分支、提交限制、环境结论、逐步 commit、全仓清扫与截图要求仅属于原阶段上下文，当前执行遵循 [AGENTS.md](../../../AGENTS.md) 和本次授权。未完成验收不自动视为通过。

> **执行方式：** 可使用适用且可用的 Skill 或等效流程；Superpowers 执行 Skills 不是前置依赖。不因 Skill 缺失停止工作，也不因阅读此计划自动启动子代理、提交或合并。

**Goal:** 按 [阶段 3.5 spec](../specs/2026-07-09-phase-3-5-ui-upgrade-design.md) 完成全站 UI 视觉升级：紫罗兰 OKLCH 主题 + 明暗切换、rail+二级面板全局布局、文档流式聊天界面、agent-timeline 工具轨迹、组件与状态设计补齐、文案中文化。**后端零改动。**

**Architecture:** 纯前端改造。设计 token 全量重写后接入 next-themes；用 shadcn Sidebar 的嵌套模式（外层 Sidebar 包 rail + panel 两个 `collapsible="none"` 内层 Sidebar）实现全局 AppShell，挂在 `app/kbs/layout.tsx`；聊天消息区改为文档流式，Markdown 管线升级为 prose + GFM + macOS 风格代码块；工具轨迹重写为竖向时间线组件。底座保持 Base UI（shadcn base-nova），不引入 motion。

**Tech Stack:** Next.js 15 App Router · React 19 · Tailwind v4（CSS-first）· shadcn base-nova（@base-ui/react）· next-themes · react-markdown + remark-gfm + rehype-highlight + @tailwindcss/typography · vitest + @testing-library/react（jsdom）

---

## 关键背景（执行前必读）

1. **工作分支**：`phase-3.5-ui-upgrade`（已存在，基于未合并的 `phase-3-react-agent` 切出）。所有 commit 打在这个分支。
2. **工作目录**：所有前端命令在 `frontend/` 下执行（`cd frontend`）。包管理器 pnpm。
3. **base-nova 惯例**：本项目 shadcn 组件基于 **Base UI**（非 Radix）。触发器组合用 **`render` prop**（如 `<DialogTrigger render={<Button />}>`），**不是** Radix 的 `asChild`。CLI 新装组件后，先看生成代码的实际导出与 props，示例代码若与生成 API 不符，以生成组件为准调整。
4. **网络**：`next/font/google` 在构建时要访问 Google Fonts，离线/无代理会导致 `pnpm build` 失败（阶段 3 遗留已知问题）。开发与测试不受影响；build 验收需要代理。
5. **验证命令**（每个 task 结束跑相关项，最后一个 task 跑全量）：
   ```bash
   pnpm lint && pnpm typecheck && pnpm test
   ```
6. **中文文案**：所有新写/改动的用户可见文案一律中文（spec §0 目标 5）。

## 文件结构总览

**新建：**

| 文件 | 职责 |
|---|---|
| `frontend/vitest.setup.ts` | 测试 setup（jest-dom matchers） |
| `frontend/lib/last-kb.ts` | localStorage 读写最近访问 KB |
| `frontend/components/layout/app-shell.tsx` | 全局三列布局组装（SidebarProvider + rail + panel + SidebarInset） |
| `frontend/components/layout/rail.tsx` | 图标 rail（功能区导航 + 明暗切换 + 设置占位） |
| `frontend/components/layout/theme-toggle.tsx` | 明暗切换按钮 |
| `frontend/components/layout/chat-panel.tsx` | 聊天区二级面板（KB 下拉 + 会话列表） |
| `frontend/components/layout/kb-panel.tsx` | 知识库区二级面板（KB 列表 + 新建 + 删除） |
| `frontend/components/common/empty-state.tsx` | 通用空态（图标 + 文案 + 可选按钮） |
| `frontend/components/chat/agent-timeline.tsx` | 工具轨迹竖向时间线（替代 tool-call-trace） |
| `frontend/components/chat/streaming-cursor.tsx` | 流式打字光标 |
| `frontend/components/chat/markdown-content.tsx` | assistant Markdown 渲染（prose + GFM + CodeBlock 集成） |
| `frontend/components/chat/citation-chip.tsx` | 引用 chip（替代 citation-card） |
| `frontend/components/ui/code-block.tsx` | macOS 窗口风格代码块（从 credit-master 移植改造） |
| `frontend/app/kbs/layout.tsx` | 挂 AppShell 的 route layout |
| `frontend/app/kbs/page.tsx` | 知识库区无选中页（空态/引导） |
| shadcn CLI 生成 | `components/ui/{sidebar,sheet,textarea,card,tabs,skeleton,tooltip,alert,alert-dialog,dropdown-menu,separator,scroll-area}.tsx` |
| 各新组件同目录 `*.test.tsx` | 组件测试 |

**修改：**

| 文件 | 变化 |
|---|---|
| `frontend/app/globals.css` | token 全量重写（紫罗兰 + 阴影/字距阶梯 + 字体映射 + typography 插件 + 光标动画 + reduced-motion） |
| `frontend/app/layout.tsx` | 字体（Geist + Noto Sans SC + Geist Mono）+ `suppressHydrationWarning` |
| `frontend/components/providers.tsx` | 包 next-themes ThemeProvider |
| `frontend/app/page.tsx` | 改为重定向逻辑 |
| `frontend/app/kbs/[kbId]/chats/page.tsx` `[conversationId]/page.tsx` | 去掉页内 sidebar/外层壳，适配 AppShell |
| `frontend/app/kbs/[kbId]/docs/page.tsx` `[docId]/page.tsx` | 去掉返回链接，适配 AppShell |
| `frontend/components/chat/chat-header.tsx` | Tabs 模式切换 + 中文化 + 移动端 SidebarTrigger，删 Docs/KBs 链接 |
| `frontend/components/chat/chat-input.tsx` | shadcn Textarea + 自动长高 + 中文 placeholder |
| `frontend/components/chat/message-bubble.tsx` | 文档流式改造 |
| `frontend/components/chat/message-list.tsx` | 空态用 EmptyState + 中文 |
| `frontend/components/chat/citation-drawer.tsx` | Dialog 硬改 → Sheet + 中文化 |
| `frontend/components/docs/ingest-status-badge.tsx` | 硬编码色 → 语义 token + spinner |
| `frontend/components/docs/doc-table.tsx` | AlertDialog 删除确认 + Tooltip |
| `frontend/components/docs/doc-uploader.tsx` | 视觉精修（微调，结构已可用） |
| `frontend/components/chunks/chunk-list.tsx` | Skeleton + 卡片规范统一 |
| `frontend/vitest.config.ts` | jsdom 环境 + react 插件 + setup |
| `frontend/package.json` | 新依赖 |
| `CLAUDE.md` / 主 spec | §10 文档同步（最后一个 task） |

**删除：**

- `frontend/components/chat/tool-call-trace.tsx`（被 agent-timeline 替代）
- `frontend/components/chat/chat-sidebar.tsx`（被 chat-panel 替代）
- `frontend/components/chat/citation-card.tsx`（被 citation-chip 替代）
- `frontend/components/kb/kb-card.tsx`（首页网格取消，KB 列表进 kb-panel）

---

### Task 1: 测试基建与新依赖

**Files:**
- Modify: `frontend/package.json`（通过 pnpm 命令）
- Modify: `frontend/vitest.config.ts`
- Create: `frontend/vitest.setup.ts`

- [ ] **Step 1: 安装运行时依赖与测试依赖**

```bash
cd frontend
pnpm add @tailwindcss/typography remark-gfm
pnpm add -D jsdom @vitejs/plugin-react @testing-library/react @testing-library/user-event @testing-library/jest-dom
```

- [ ] **Step 2: 改 vitest.config.ts 启用 jsdom + react 插件**

覆盖 `frontend/vitest.config.ts` 为：

```ts
import { fileURLToPath } from "node:url";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL(".", import.meta.url)),
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
  },
});
```

- [ ] **Step 3: 创建 vitest.setup.ts**

```ts
import "@testing-library/jest-dom/vitest";
```

- [ ] **Step 4: 跑既有测试确认不回归**

Run: `pnpm test`
Expected: 既有 `lib/api/sse.test.ts` 与 `lib/hooks/use-chat-stream.test.ts` 全部 PASS（jsdom 下纯逻辑测试不受影响）

- [ ] **Step 5: Commit**

```bash
git add package.json pnpm-lock.yaml vitest.config.ts vitest.setup.ts
git commit -m "chore(frontend): add typography/gfm deps and jsdom test infra

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 设计 token 重写 + 字体 + 明暗模式接入 + 状态 badge 语义化

**Files:**
- Modify: `frontend/app/globals.css`（全量重写）
- Modify: `frontend/app/layout.tsx`
- Modify: `frontend/components/providers.tsx`
- Create: `frontend/components/layout/theme-toggle.tsx`
- Modify: `frontend/components/docs/ingest-status-badge.tsx`
- Test: `frontend/components/docs/ingest-status-badge.test.tsx`

- [ ] **Step 1: 全量重写 globals.css**

覆盖 `frontend/app/globals.css` 为（保留 `shadcn/tailwind.css` import；紫罗兰主题 spec §3.1；阴影/字距阶梯；字体映射；打字光标动画；reduced-motion）：

```css
@import "tailwindcss";
@import "tw-animate-css";
@import "shadcn/tailwind.css";
@plugin "@tailwindcss/typography";

@custom-variant dark (&:is(.dark *));

@theme inline {
    --font-sans: var(--font-geist-sans), var(--font-noto-sans-sc), system-ui, -apple-system, BlinkMacSystemFont, sans-serif;
    --font-heading: var(--font-sans);
    --font-mono: var(--font-geist-mono), "SF Mono", Monaco, Inconsolata, "Roboto Mono", monospace;
    --color-sidebar-ring: var(--sidebar-ring);
    --color-sidebar-border: var(--sidebar-border);
    --color-sidebar-accent-foreground: var(--sidebar-accent-foreground);
    --color-sidebar-accent: var(--sidebar-accent);
    --color-sidebar-primary-foreground: var(--sidebar-primary-foreground);
    --color-sidebar-primary: var(--sidebar-primary);
    --color-sidebar-foreground: var(--sidebar-foreground);
    --color-sidebar: var(--sidebar);
    --color-chart-5: var(--chart-5);
    --color-chart-4: var(--chart-4);
    --color-chart-3: var(--chart-3);
    --color-chart-2: var(--chart-2);
    --color-chart-1: var(--chart-1);
    --color-ring: var(--ring);
    --color-input: var(--input);
    --color-border: var(--border);
    --color-destructive: var(--destructive);
    --color-accent-foreground: var(--accent-foreground);
    --color-accent: var(--accent);
    --color-muted-foreground: var(--muted-foreground);
    --color-muted: var(--muted);
    --color-secondary-foreground: var(--secondary-foreground);
    --color-secondary: var(--secondary);
    --color-primary-foreground: var(--primary-foreground);
    --color-primary: var(--primary);
    --color-popover-foreground: var(--popover-foreground);
    --color-popover: var(--popover);
    --color-card-foreground: var(--card-foreground);
    --color-card: var(--card);
    --color-foreground: var(--foreground);
    --color-background: var(--background);
    --radius-sm: calc(var(--radius) * 0.6);
    --radius-md: calc(var(--radius) * 0.8);
    --radius-lg: var(--radius);
    --radius-xl: calc(var(--radius) * 1.4);
    --radius-2xl: calc(var(--radius) * 1.8);
    --radius-3xl: calc(var(--radius) * 2.2);
    --radius-4xl: calc(var(--radius) * 2.6);
    --shadow-2xs: var(--shadow-2xs);
    --shadow-xs: var(--shadow-xs);
    --shadow-sm: var(--shadow-sm);
    --shadow: var(--shadow);
    --shadow-md: var(--shadow-md);
    --shadow-lg: var(--shadow-lg);
    --shadow-xl: var(--shadow-xl);
    --shadow-2xl: var(--shadow-2xl);
    --tracking-tighter: calc(var(--tracking-normal) - 0.05em);
    --tracking-tight: calc(var(--tracking-normal) - 0.025em);
    --tracking-normal: var(--tracking-normal);
    --tracking-wide: calc(var(--tracking-normal) + 0.025em);
    --tracking-wider: calc(var(--tracking-normal) + 0.05em);
    --tracking-widest: calc(var(--tracking-normal) + 0.1em);
    --animate-caret-blink: caret-blink 1.2s ease-out infinite;

    @keyframes caret-blink {
        0%, 70%, 100% { opacity: 1; }
        20%, 50% { opacity: 0; }
    }
}

:root {
    --radius: 0.625rem;
    --background: oklch(1 0 0);
    --foreground: oklch(0.141 0.005 285.823);
    --card: oklch(1 0 0);
    --card-foreground: oklch(0.141 0.005 285.823);
    --popover: oklch(1 0 0);
    --popover-foreground: oklch(0.141 0.005 285.823);
    --primary: oklch(0.585 0.233 277.117);
    --primary-foreground: oklch(0.985 0 0);
    --secondary: oklch(0.967 0.001 286.375);
    --secondary-foreground: oklch(0.21 0.006 285.885);
    --muted: oklch(0.967 0.001 286.375);
    --muted-foreground: oklch(0.552 0.016 285.938);
    --accent: oklch(0.967 0.001 286.375);
    --accent-foreground: oklch(0.21 0.006 285.885);
    --destructive: oklch(0.577 0.245 27.325);
    --border: oklch(0.92 0.004 286.32);
    --input: oklch(0.92 0.004 286.32);
    --ring: oklch(0.585 0.233 277.117);
    --chart-1: oklch(0.646 0.222 41.116);
    --chart-2: oklch(0.6 0.118 184.704);
    --chart-3: oklch(0.398 0.07 227.392);
    --chart-4: oklch(0.828 0.189 84.429);
    --chart-5: oklch(0.769 0.188 70.08);
    --sidebar: oklch(0.985 0 0);
    --sidebar-foreground: oklch(0.141 0.005 285.823);
    --sidebar-primary: oklch(0.585 0.233 277.117);
    --sidebar-primary-foreground: oklch(0.985 0 0);
    --sidebar-accent: oklch(0.967 0.001 286.375);
    --sidebar-accent-foreground: oklch(0.21 0.006 285.885);
    --sidebar-border: oklch(0.92 0.004 286.32);
    --sidebar-ring: oklch(0.585 0.233 277.117);
    --shadow-color: oklch(0 0 0);
    --shadow-2xs: 0 1px 3px 0 oklch(0 0 0 / 0.05);
    --shadow-xs: 0 1px 3px 0 oklch(0 0 0 / 0.05);
    --shadow-sm: 0 1px 3px 0 oklch(0 0 0 / 0.1), 0 1px 2px -1px oklch(0 0 0 / 0.1);
    --shadow: 0 1px 3px 0 oklch(0 0 0 / 0.1), 0 1px 2px -1px oklch(0 0 0 / 0.1);
    --shadow-md: 0 4px 6px -1px oklch(0 0 0 / 0.1), 0 2px 4px -2px oklch(0 0 0 / 0.1);
    --shadow-lg: 0 10px 15px -3px oklch(0 0 0 / 0.1), 0 4px 6px -4px oklch(0 0 0 / 0.1);
    --shadow-xl: 0 20px 25px -5px oklch(0 0 0 / 0.1), 0 8px 10px -6px oklch(0 0 0 / 0.1);
    --shadow-2xl: 0 25px 50px -12px oklch(0 0 0 / 0.25);
    --tracking-normal: 0em;
}

.dark {
    --background: oklch(0.141 0.005 285.823);
    --foreground: oklch(0.985 0 0);
    --card: oklch(0.21 0.006 285.885);
    --card-foreground: oklch(0.985 0 0);
    --popover: oklch(0.21 0.006 285.885);
    --popover-foreground: oklch(0.985 0 0);
    --primary: oklch(0.673 0.182 276.935);
    --primary-foreground: oklch(0.141 0.005 285.823);
    --secondary: oklch(0.274 0.006 286.033);
    --secondary-foreground: oklch(0.985 0 0);
    --muted: oklch(0.274 0.006 286.033);
    --muted-foreground: oklch(0.705 0.015 286.067);
    --accent: oklch(0.274 0.006 286.033);
    --accent-foreground: oklch(0.985 0 0);
    --destructive: oklch(0.704 0.191 22.216);
    --border: oklch(1 0 0 / 10%);
    --input: oklch(1 0 0 / 15%);
    --ring: oklch(0.673 0.182 276.935);
    --chart-1: oklch(0.488 0.243 264.376);
    --chart-2: oklch(0.696 0.17 162.48);
    --chart-3: oklch(0.769 0.188 70.08);
    --chart-4: oklch(0.627 0.265 303.9);
    --chart-5: oklch(0.645 0.246 16.439);
    --sidebar: oklch(0.21 0.006 285.885);
    --sidebar-foreground: oklch(0.985 0 0);
    --sidebar-primary: oklch(0.673 0.182 276.935);
    --sidebar-primary-foreground: oklch(0.141 0.005 285.823);
    --sidebar-accent: oklch(0.274 0.006 286.033);
    --sidebar-accent-foreground: oklch(0.985 0 0);
    --sidebar-border: oklch(1 0 0 / 10%);
    --sidebar-ring: oklch(0.673 0.182 276.935);
}

@layer base {
  * {
    @apply border-border outline-ring/50;
    }
  body {
    @apply bg-background text-foreground;
    }
  html {
    @apply font-sans;
    }
}

.hide-scrollbar::-webkit-scrollbar {
  display: none;
}

.hide-scrollbar {
  -ms-overflow-style: none;
  scrollbar-width: none;
}

@media (prefers-reduced-motion: reduce) {
  *, ::before, ::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

- [ ] **Step 2: 改 layout.tsx 接入三套字体与 suppressHydrationWarning**

覆盖 `frontend/app/layout.tsx` 为：

```tsx
import type { Metadata } from "next";
import "./globals.css";
import { Geist, Geist_Mono, Noto_Sans_SC } from "next/font/google";
import { cn } from "@/lib/utils";
import { Providers } from "@/components/providers";

const geist = Geist({ subsets: ["latin"], variable: "--font-geist-sans" });
const geistMono = Geist_Mono({ subsets: ["latin"], variable: "--font-geist-mono" });
const notoSansSC = Noto_Sans_SC({
  subsets: ["latin"],
  weight: ["400", "500", "700"],
  variable: "--font-noto-sans-sc",
});

export const metadata: Metadata = {
  title: "it-wiki",
  description: "团队知识库 Agent",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="zh-CN"
      suppressHydrationWarning
      className={cn("font-sans", geist.variable, geistMono.variable, notoSansSC.variable)}
    >
      <body className="min-h-screen bg-background text-foreground antialiased">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
```

注意：`--font-sans` CSS 变量名换成了 `--font-geist-sans`（globals.css `@theme inline` 已同步），避免旧版 `--font-sans: var(--font-sans)` 的自引用。

- [ ] **Step 3: providers.tsx 包 ThemeProvider**

覆盖 `frontend/components/providers.tsx` 为：

```tsx
"use client";

import { useState, type ReactNode } from "react";
import { ThemeProvider } from "next-themes";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReactQueryDevtools } from "@tanstack/react-query-devtools";
import { Toaster } from "@/components/ui/sonner";

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 5_000,
            retry: 1,
            refetchOnWindowFocus: false,
          },
        },
      }),
  );

  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <QueryClientProvider client={client}>
        {children}
        <Toaster richColors position="top-right" />
        <ReactQueryDevtools initialIsOpen={false} />
      </QueryClientProvider>
    </ThemeProvider>
  );
}
```

- [ ] **Step 4: 新建 theme-toggle.tsx**

创建 `frontend/components/layout/theme-toggle.tsx`：

```tsx
"use client";

import { useEffect, useState } from "react";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";

import { Button } from "@/components/ui/button";

export function ThemeToggle() {
  const { resolvedTheme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label="切换明暗模式"
      onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
    >
      {mounted && resolvedTheme === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </Button>
  );
}
```

- [ ] **Step 5: 写 ingest-status-badge 语义色的失败测试**

创建 `frontend/components/docs/ingest-status-badge.test.tsx`：

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { IngestStatusBadge } from "./ingest-status-badge";

describe("IngestStatusBadge", () => {
  it("renders processing status with primary tone and spinner", () => {
    render(<IngestStatusBadge status="embedding" />);
    const badge = screen.getByText("向量化中").closest("[data-slot=badge]") ?? screen.getByText("向量化中");
    expect(badge.className).toContain("text-primary");
    expect(badge.className).not.toContain("bg-blue-100");
    expect(badge.querySelector(".animate-spin")).not.toBeNull();
  });

  it("renders failed status with destructive tone", () => {
    render(<IngestStatusBadge status="failed" />);
    const badge = screen.getByText("失败").closest("[data-slot=badge]") ?? screen.getByText("失败");
    expect(badge.className).toContain("text-destructive");
    expect(badge.className).not.toContain("bg-red-100");
  });
});
```

- [ ] **Step 6: 跑测试确认失败**

Run: `pnpm vitest run components/docs/ingest-status-badge.test.tsx`
Expected: FAIL（当前实现是 `bg-blue-100`/`bg-red-100`，无 spinner）

- [ ] **Step 7: 重写 ingest-status-badge**

覆盖 `frontend/components/docs/ingest-status-badge.tsx` 为：

```tsx
import { LoaderCircle } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import type { DocStatus } from "@/lib/schemas";

const variantMap: Record<DocStatus, { label: string; className: string; spinning?: boolean }> = {
  pending:   { label: "排队中",   className: "bg-muted text-muted-foreground" },
  parsing:   { label: "解析中",   className: "bg-primary/10 text-primary", spinning: true },
  chunking:  { label: "切片中",   className: "bg-primary/10 text-primary", spinning: true },
  embedding: { label: "向量化中", className: "bg-primary/10 text-primary", spinning: true },
  ready:     { label: "就绪",     className: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400" },
  failed:    { label: "失败",     className: "bg-destructive/10 text-destructive" },
};

export function IngestStatusBadge({ status }: { status: DocStatus }) {
  const v = variantMap[status];
  return (
    <Badge variant="secondary" className={v.className}>
      {v.spinning && <LoaderCircle className="size-3 animate-spin" />}
      {v.label}
    </Badge>
  );
}
```

说明：`ready` 用 emerald 加显式 `dark:` 变体（设计体系没有 success 语义 token，这是有意的最小妥协，两种模式都不穿帮）。若生成的 Badge 根元素没有 `data-slot="badge"` 属性，测试里的 `closest` 回退到文本节点本身也能通过。

- [ ] **Step 8: 跑测试确认通过 + 手动烟测**

Run: `pnpm vitest run components/docs/ingest-status-badge.test.tsx`
Expected: PASS

Run: `pnpm dev` 后浏览器打开 http://localhost:3000 ，确认页面整体变为紫罗兰主色（按钮/链接高亮），无样式编译报错。

- [ ] **Step 9: 全量验证 + Commit**

Run: `pnpm lint && pnpm typecheck && pnpm test`
Expected: 全绿

```bash
git add app/globals.css app/layout.tsx components/providers.tsx components/layout/theme-toggle.tsx components/docs/ingest-status-badge.tsx components/docs/ingest-status-badge.test.tsx
git commit -m "feat(frontend): violet OKLCH theme tokens, dark mode via next-themes, semantic status badge

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: shadcn 组件补装（12 个）

**Files:**
- Create（CLI 生成）: `frontend/components/ui/{sidebar,sheet,textarea,card,tabs,skeleton,tooltip,alert,alert-dialog,dropdown-menu,separator,scroll-area}.tsx` 及其可能携带的依赖文件（如 `hooks/use-mobile.ts`）

- [ ] **Step 1: 逐个添加组件**

```bash
cd frontend
pnpm dlx shadcn@latest add sidebar sheet textarea card tabs skeleton tooltip alert alert-dialog dropdown-menu separator scroll-area
```

如果 CLI 询问是否覆盖已有文件（button/badge 等），选择 **No**（保留现有定制）。

- [ ] **Step 2: 验证生成结果**

Run: `ls components/ui/`
Expected: 新增 12 个组件文件。逐个打开确认 import 来源是 `@base-ui/react`（base-nova 体系）。

**兜底（spec §7.2）**：若个别组件 base-nova registry 未提供（CLI 报 not found），以 https://ui.shadcn.com 官方对应组件为蓝本，手工改写为 Base UI primitive（参考现有 `components/ui/dialog.tsx` 的写法：`render` prop、`data-slot` 属性、Base UI 命名空间导入），放入 `components/ui/` 并在 commit message 注明。

- [ ] **Step 3: 阅读并记录生成组件的关键 API**

打开 `components/ui/sidebar.tsx`，确认以下导出存在（后续 Task 依赖）：`SidebarProvider`、`Sidebar`、`SidebarInset`、`SidebarTrigger`、`SidebarHeader`、`SidebarContent`、`SidebarFooter`、`SidebarGroup`、`SidebarGroupLabel`、`SidebarMenu`、`SidebarMenuItem`、`SidebarMenuButton`、`useSidebar`。打开 `sheet.tsx` 确认 `Sheet`、`SheetContent`、`SheetHeader`、`SheetTitle`、`SheetDescription` 存在。若命名不同，在后续 Task 中按实际导出调整。

- [ ] **Step 4: 验证编译 + Commit**

Run: `pnpm typecheck && pnpm lint`
Expected: 全绿（新组件未被引用也应通过编译）

```bash
git add components/ui/ hooks/ package.json pnpm-lock.yaml
git commit -m "feat(frontend): add 12 shadcn base-nova components for phase 3.5

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 通用组件（empty-state / streaming-cursor / code-block）

**Files:**
- Create: `frontend/components/common/empty-state.tsx`
- Create: `frontend/components/chat/streaming-cursor.tsx`
- Create: `frontend/components/ui/code-block.tsx`
- Test: `frontend/components/common/empty-state.test.tsx`
- Test: `frontend/components/ui/code-block.test.tsx`

- [ ] **Step 1: 写 empty-state 失败测试**

创建 `frontend/components/common/empty-state.test.tsx`：

```tsx
import { render, screen } from "@testing-library/react";
import { FileText } from "lucide-react";
import { describe, expect, it } from "vitest";

import { EmptyState } from "./empty-state";

describe("EmptyState", () => {
  it("renders icon, title, description and action", () => {
    render(
      <EmptyState
        icon={FileText}
        title="还没有文档"
        description="拖入文件开始构建知识库"
        action={<button>上传</button>}
      />,
    );
    expect(screen.getByText("还没有文档")).toBeInTheDocument();
    expect(screen.getByText("拖入文件开始构建知识库")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "上传" })).toBeInTheDocument();
  });

  it("renders without optional description and action", () => {
    render(<EmptyState icon={FileText} title="空空如也" />);
    expect(screen.getByText("空空如也")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm vitest run components/common/empty-state.test.tsx`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现 empty-state**

创建 `frontend/components/common/empty-state.tsx`：

```tsx
import type { ComponentType, ReactNode } from "react";

import { cn } from "@/lib/utils";

export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  className,
}: {
  icon: ComponentType<{ className?: string }>;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col items-center justify-center gap-2 py-16 text-center", className)}>
      <div className="flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
        <Icon className="size-6" />
      </div>
      <h2 className="mt-2 text-base font-semibold">{title}</h2>
      {description && <p className="max-w-sm text-sm leading-relaxed text-muted-foreground">{description}</p>}
      {action && <div className="mt-3">{action}</div>}
    </div>
  );
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm vitest run components/common/empty-state.test.tsx`
Expected: PASS

- [ ] **Step 5: 实现 streaming-cursor（纯展示，不单独测试）**

创建 `frontend/components/chat/streaming-cursor.tsx`：

```tsx
export function StreamingCursor() {
  return (
    <span
      aria-hidden
      className="ml-0.5 inline-block h-4 w-2 animate-caret-blink rounded-[2px] bg-primary align-text-bottom"
    />
  );
}
```

（`animate-caret-blink` 来自 Task 2 globals.css 里定义的 `--animate-caret-blink`。）

- [ ] **Step 6: 写 code-block 失败测试**

创建 `frontend/components/ui/code-block.test.tsx`：

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { CodeBlock } from "./code-block";

describe("CodeBlock", () => {
  beforeEach(() => {
    Object.assign(navigator, {
      clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("renders code text and language label", () => {
    render(<CodeBlock code="show interface status" language="bash" />);
    expect(screen.getByText("show interface status")).toBeInTheDocument();
    expect(screen.getByText("bash")).toBeInTheDocument();
  });

  it("copies code to clipboard on copy button click", async () => {
    const user = userEvent.setup({ writeToClipboard: false });
    render(<CodeBlock code="goose up" language="bash" />);
    await user.click(screen.getByRole("button", { name: "复制代码" }));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith("goose up");
  });
});
```

注：若 `userEvent.setup` 不支持 `writeToClipboard` 选项（版本差异），去掉该选项、保留 `Object.assign(navigator, ...)` mock 即可。

- [ ] **Step 7: 跑测试确认失败**

Run: `pnpm vitest run components/ui/code-block.test.tsx`
Expected: FAIL（模块不存在）

- [ ] **Step 8: 实现 code-block（credit-master 移植改造）**

创建 `frontend/components/ui/code-block.tsx`。相对 credit-master 原版的改造点：支持 `children`（rehype-highlight 的高亮 span 直接渲染，`code` prop 仅用于复制）、加语言标签、复制按钮加 aria-label：

```tsx
"use client";

import * as React from "react";
import { Check, Copy } from "lucide-react";

import { cn } from "@/lib/utils";

interface CodeBlockProps extends React.HTMLAttributes<HTMLDivElement> {
  code: string;
  language?: string;
}

export function CodeBlock({ code, language, className, children, ...props }: CodeBlockProps) {
  const [hasCopied, setHasCopied] = React.useState(false);
  const resetTimerRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);

  React.useEffect(() => {
    return () => {
      if (resetTimerRef.current) clearTimeout(resetTimerRef.current);
    };
  }, []);

  const onCopy = async () => {
    // 非 https / 非 localhost 环境下 navigator.clipboard 不存在，静默跳过
    if (!navigator.clipboard) return;
    try {
      await navigator.clipboard.writeText(code);
    } catch {
      return;
    }
    setHasCopied(true);
    if (resetTimerRef.current) clearTimeout(resetTimerRef.current);
    resetTimerRef.current = setTimeout(() => setHasCopied(false), 2000);
  };

  return (
    <div
      className={cn(
        "relative my-4 overflow-hidden rounded-xl border border-border/40 bg-[#1e1e1e] shadow-sm dark:bg-[#0d0d0d]",
        className,
      )}
      {...props}
    >
      <div className="flex items-center justify-between border-b border-white/5 px-4 py-2.5">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5">
            <div className="size-3 rounded-full border border-[#e0443e]/50 bg-[#ff5f56]" />
            <div className="size-3 rounded-full border border-[#dea123]/50 bg-[#ffbd2e]" />
            <div className="size-3 rounded-full border border-[#1aab29]/50 bg-[#27c93f]" />
          </div>
          {language && <span className="font-mono text-[10px] text-white/40">{language}</span>}
        </div>
        <button
          type="button"
          aria-label={hasCopied ? "已复制" : "复制代码"}
          onClick={onCopy}
          className="text-white/40 transition-colors hover:text-white"
        >
          {hasCopied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
        </button>
      </div>
      <div className="overflow-x-auto p-4">
        <pre className="!m-0 !border-0 !bg-transparent !p-0 whitespace-pre-wrap break-all font-mono text-xs leading-relaxed text-[#e0e0e0] shadow-none">
          <code className={cn("!m-0 !border-0 !bg-transparent !p-0 font-mono", language && `language-${language}`)}>
            {children ?? code}
          </code>
        </pre>
      </div>
    </div>
  );
}
```

- [ ] **Step 9: 跑测试确认通过**

Run: `pnpm vitest run components/ui/code-block.test.tsx`
Expected: PASS

- [ ] **Step 10: Commit**

```bash
git add components/common/ components/chat/streaming-cursor.tsx components/ui/code-block.tsx components/ui/code-block.test.tsx
git commit -m "feat(frontend): add empty-state, streaming-cursor and macOS-style code-block

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: markdown-content（prose + GFM + CodeBlock 集成）

**Files:**
- Create: `frontend/components/chat/markdown-content.tsx`
- Test: `frontend/components/chat/markdown-content.test.tsx`

- [ ] **Step 1: 写失败测试**

创建 `frontend/components/chat/markdown-content.test.tsx`：

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { MarkdownContent } from "./markdown-content";

describe("MarkdownContent", () => {
  it("renders fenced code block as macOS-style CodeBlock with copy button", () => {
    render(<MarkdownContent content={"```bash\nshow interface status\n```"} />);
    expect(screen.getByRole("button", { name: "复制代码" })).toBeInTheDocument();
    expect(screen.getByText("bash")).toBeInTheDocument();
  });

  it("renders GFM table", () => {
    const md = "| 端口 | 状态 |\n| --- | --- |\n| Gi0/1 | up |";
    render(<MarkdownContent content={md} />);
    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByText("Gi0/1")).toBeInTheDocument();
  });

  it("renders inline code without CodeBlock chrome", () => {
    render(<MarkdownContent content={"先执行 `goose up` 再继续"} />);
    expect(screen.getByText("goose up")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "复制代码" })).not.toBeInTheDocument();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm vitest run components/chat/markdown-content.test.tsx`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现 markdown-content**

创建 `frontend/components/chat/markdown-content.tsx`：

```tsx
"use client";

import { Children, isValidElement, type ReactElement, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import rehypeHighlight from "rehype-highlight";
import remarkGfm from "remark-gfm";

import { CodeBlock } from "@/components/ui/code-block";

function extractText(node: ReactNode): string {
  if (node == null || typeof node === "boolean") return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(extractText).join("");
  if (isValidElement(node)) {
    return extractText((node.props as { children?: ReactNode }).children);
  }
  return "";
}

function Pre({ children }: { children?: ReactNode }) {
  const child = Children.toArray(children).find(isValidElement) as
    | ReactElement<{ className?: string; children?: ReactNode }>
    | undefined;
  if (!child) return <pre>{children}</pre>;
  const language = /language-(\w+)/.exec(child.props.className ?? "")?.[1];
  const code = extractText(child.props.children).replace(/\n$/, "");
  return (
    <CodeBlock code={code} language={language}>
      {child.props.children}
    </CodeBlock>
  );
}

export function MarkdownContent({ content }: { content: string }) {
  return (
    <div className="prose prose-sm max-w-none dark:prose-invert prose-pre:m-0 prose-pre:bg-transparent prose-pre:p-0">
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={{ pre: Pre }}>
        {content}
      </ReactMarkdown>
    </div>
  );
}
```

- [ ] **Step 4: 跑测试确认通过 + Commit**

Run: `pnpm vitest run components/chat/markdown-content.test.tsx`
Expected: PASS

```bash
git add components/chat/markdown-content.tsx components/chat/markdown-content.test.tsx
git commit -m "feat(frontend): markdown-content with prose typography, GFM and CodeBlock integration

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: agent-timeline（替代 tool-call-trace）

**Files:**
- Create: `frontend/components/chat/agent-timeline.tsx`
- Test: `frontend/components/chat/agent-timeline.test.tsx`
- Delete: `frontend/components/chat/tool-call-trace.tsx`（在 Task 7 改完 message-bubble 引用后删除，本 Task 先并存）

数据模型（已存在，不改）：`LocalToolStep`（`lib/hooks/use-chat-stream.ts`）= `ToolCallStep`（step/id/name/thought?/arguments?/result?/duration_ms?/error?）+ `running?`。

- [ ] **Step 1: 写失败测试**

创建 `frontend/components/chat/agent-timeline.test.tsx`：

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { LocalToolStep } from "@/lib/hooks/use-chat-stream";
import { AgentTimeline } from "./agent-timeline";

const doneStep: LocalToolStep = {
  step: 1,
  id: "call-1",
  name: "kb_retrieval",
  thought: "需要先检索知识库",
  arguments: { query: "交换机 端口" },
  result: '{"chunks":4}',
  duration_ms: 300,
};

const runningStep: LocalToolStep = {
  step: 2,
  id: "call-2",
  name: "list_documents",
  arguments: {},
  running: true,
};

const failedStep: LocalToolStep = {
  step: 2,
  id: "call-3",
  name: "kb_retrieval",
  arguments: {},
  error: "timeout",
};

describe("AgentTimeline", () => {
  it("renders nothing for empty steps", () => {
    const { container } = render(<AgentTimeline steps={[]} answerStarted={false} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("renders thought as quote and tool step with duration when expanded", () => {
    render(<AgentTimeline steps={[doneStep]} answerStarted={false} />);
    expect(screen.getByText("需要先检索知识库")).toBeInTheDocument();
    expect(screen.getByText("kb_retrieval")).toBeInTheDocument();
    expect(screen.getByText(/300 ms/)).toBeInTheDocument();
  });

  it("shows running state", () => {
    render(<AgentTimeline steps={[doneStep, runningStep]} answerStarted={false} />);
    expect(screen.getByText("运行中...")).toBeInTheDocument();
  });

  it("shows error state", () => {
    render(<AgentTimeline steps={[failedStep]} answerStarted={false} />);
    expect(screen.getByText("失败")).toBeInTheDocument();
    expect(screen.getByText("timeout")).toBeInTheDocument();
  });

  it("collapses to summary when answer started, expands on click", async () => {
    const user = userEvent.setup();
    render(<AgentTimeline steps={[doneStep]} answerStarted />);
    expect(screen.queryByText("需要先检索知识库")).not.toBeInTheDocument();
    const summary = screen.getByRole("button", { name: /调用了 1 个工具/ });
    await user.click(summary);
    expect(screen.getByText("需要先检索知识库")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm vitest run components/chat/agent-timeline.test.tsx`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现 agent-timeline**

创建 `frontend/components/chat/agent-timeline.tsx`：

```tsx
"use client";

import { useState } from "react";
import { Check, ChevronRight, CircleAlert, LoaderCircle, Wrench } from "lucide-react";

import { cn } from "@/lib/utils";
import type { LocalToolStep } from "@/lib/hooks/use-chat-stream";

function argsSummary(args: unknown): string {
  const text = typeof args === "string" ? args : JSON.stringify(args ?? {});
  return text.length > 60 ? text.slice(0, 57) + "..." : text;
}

function StepNode({ step }: { step: LocalToolStep }) {
  return (
    <li className="relative pl-5">
      <span
        className={cn(
          "absolute left-0 top-1.5 size-2 -translate-x-[calc(50%+1px)] rounded-full",
          step.running ? "bg-primary" : step.error ? "bg-destructive" : "bg-primary/40",
        )}
      />
      {step.thought && (
        <p className="mb-1.5 text-xs italic leading-relaxed text-muted-foreground">{step.thought}</p>
      )}
      <details className="group rounded-md border border-border/60 bg-muted/40 text-xs">
        <summary className="flex cursor-pointer list-none items-center gap-2 px-2.5 py-1.5 [&::-webkit-details-marker]:hidden">
          {step.running ? (
            <LoaderCircle className="size-3.5 shrink-0 animate-spin text-primary" />
          ) : step.error ? (
            <CircleAlert className="size-3.5 shrink-0 text-destructive" />
          ) : (
            <Check className="size-3.5 shrink-0 text-emerald-600 dark:text-emerald-400" />
          )}
          <Wrench className="size-3 shrink-0 text-muted-foreground" />
          <span className="font-medium">{step.name}</span>
          <span className="truncate font-mono text-muted-foreground">{argsSummary(step.arguments)}</span>
          <span className={cn("ml-auto shrink-0 text-muted-foreground", step.error && "text-destructive")}>
            {step.running ? "运行中..." : step.error ? "失败" : `${step.duration_ms ?? 0} ms`}
          </span>
        </summary>
        <div className="space-y-2 border-t border-border/60 px-2.5 py-2">
          <div>
            <p className="mb-1 font-medium text-muted-foreground">参数</p>
            <pre className="overflow-x-auto rounded bg-muted p-2 font-mono">{JSON.stringify(step.arguments, null, 2)}</pre>
          </div>
          {step.error ? (
            <div>
              <p className="mb-1 font-medium text-destructive">错误</p>
              <pre className="overflow-x-auto rounded bg-muted p-2 font-mono text-destructive">{step.error}</pre>
            </div>
          ) : (
            step.result !== undefined && (
              <div>
                <p className="mb-1 font-medium text-muted-foreground">结果</p>
                <pre className="max-h-48 overflow-auto whitespace-pre-wrap rounded bg-muted p-2 font-mono">{step.result}</pre>
              </div>
            )
          )}
        </div>
      </details>
    </li>
  );
}

export function AgentTimeline({
  steps,
  answerStarted,
}: {
  steps: LocalToolStep[];
  answerStarted: boolean;
}) {
  const [userOpen, setUserOpen] = useState<boolean | null>(null);
  if (steps.length === 0) return null;
  const open = userOpen ?? !answerStarted;

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setUserOpen(true)}
        className="mb-2 inline-flex items-center gap-1 rounded-full border border-primary/25 bg-primary/5 px-2.5 py-1 text-xs text-primary transition-colors hover:bg-primary/10"
      >
        <Wrench className="size-3" />
        调用了 {steps.length} 个工具
        <ChevronRight className="size-3" />
      </button>
    );
  }

  return (
    <div className="mb-3">
      {answerStarted && (
        <button
          type="button"
          onClick={() => setUserOpen(false)}
          className="mb-2 text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          收起过程 ▴
        </button>
      )}
      <ol className="space-y-3 border-l-2 border-primary/25 pl-3">
        {steps.map((step) => (
          <StepNode key={step.id || step.step} step={step} />
        ))}
      </ol>
    </div>
  );
}
```

- [ ] **Step 4: 跑测试确认通过 + Commit**

Run: `pnpm vitest run components/chat/agent-timeline.test.tsx`
Expected: PASS

```bash
git add components/chat/agent-timeline.tsx components/chat/agent-timeline.test.tsx
git commit -m "feat(frontend): vertical agent-timeline with thought quotes and auto-collapse

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: message-bubble 文档流式改造 + message-list 空态

**Files:**
- Modify: `frontend/components/chat/message-bubble.tsx`（全量重写）
- Modify: `frontend/components/chat/message-list.tsx`
- Delete: `frontend/components/chat/tool-call-trace.tsx`
- Test: `frontend/components/chat/message-bubble.test.tsx`

- [ ] **Step 1: 写失败测试**

创建 `frontend/components/chat/message-bubble.test.tsx`：

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { LocalChatMessage } from "@/lib/hooks/use-chat-stream";
import { MessageBubble } from "./message-bubble";

function makeMessage(patch: Partial<LocalChatMessage>): LocalChatMessage {
  return {
    id: "m1",
    conversation_id: "c1",
    role: "assistant",
    content: "",
    citations: [],
    tool_calls: [],
    token_usage: {},
    created_at: "2026-07-09T00:00:00.000Z",
    ...patch,
  };
}

describe("MessageBubble", () => {
  it("renders user message as right-aligned bubble", () => {
    render(<MessageBubble message={makeMessage({ role: "user", content: "端口不通" })} onCitationClick={vi.fn()} />);
    const article = screen.getByText("端口不通").closest("article");
    expect(article?.className).toContain("justify-end");
  });

  it("renders assistant markdown full-width without bubble border", () => {
    render(<MessageBubble message={makeMessage({ content: "**排查步骤**" })} onCitationClick={vi.fn()} />);
    expect(screen.getByText("排查步骤")).toBeInTheDocument();
    const article = screen.getByText("排查步骤").closest("article");
    expect(article?.className).toContain("justify-start");
  });

  it("shows streaming cursor while pending without content", () => {
    const { container } = render(
      <MessageBubble message={makeMessage({ pending: true })} onCitationClick={vi.fn()} />,
    );
    expect(container.querySelector(".animate-caret-blink")).not.toBeNull();
  });

  it("renders citations footer with source label", () => {
    const citation = {
      id: "c1",
      chunk_id: "chunk-1",
      document_id: "d1",
      document_title: "排障手册",
      seq: 3,
      score: 0.92,
      snippet: "...",
    };
    render(<MessageBubble message={makeMessage({ content: "答案", citations: [citation] })} onCitationClick={vi.fn()} />);
    expect(screen.getByText("来源")).toBeInTheDocument();
    expect(screen.getByText("排障手册")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm vitest run components/chat/message-bubble.test.tsx`
Expected: FAIL（现实现无 `来源` 标签、pending 是文字 "Thinking..."）

- [ ] **Step 3: 重写 message-bubble**

覆盖 `frontend/components/chat/message-bubble.tsx` 为：

```tsx
"use client";

import { Sparkles } from "lucide-react";

import { AgentTimeline } from "@/components/chat/agent-timeline";
import { CitationChip } from "@/components/chat/citation-chip";
import { MarkdownContent } from "@/components/chat/markdown-content";
import { StreamingCursor } from "@/components/chat/streaming-cursor";
import { cn } from "@/lib/utils";
import type { Citation } from "@/lib/schemas";
import type { LocalChatMessage } from "@/lib/hooks/use-chat-stream";

export function MessageBubble({
  message,
  onCitationClick,
}: {
  message: LocalChatMessage;
  onCitationClick: (citation: Citation) => void;
}) {
  const isUser = message.role === "user";

  if (isUser) {
    return (
      <article className="flex justify-end">
        <div className="max-w-[min(560px,85%)] rounded-2xl rounded-br-sm bg-primary px-4 py-2.5 text-sm text-primary-foreground">
          <p className="whitespace-pre-wrap leading-relaxed">{message.content}</p>
        </div>
      </article>
    );
  }

  const anyRunning = message.tool_calls.some((step) => step.running);
  const answerStarted = !message.pending || (message.content.trim().length > 0 && !anyRunning);
  const showCursorOnly = !!message.pending && message.content.trim().length === 0 && message.tool_calls.length === 0;

  return (
    <article className="flex justify-start">
      <div className="flex w-full gap-3">
        <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
          <Sparkles className="size-4" />
        </div>
        <div className="min-w-0 flex-1 pt-0.5 text-sm">
          {message.tool_calls.length > 0 && (
            <AgentTimeline steps={message.tool_calls} answerStarted={answerStarted} />
          )}
          {showCursorOnly ? (
            <StreamingCursor />
          ) : (
            message.content && (
              <div className="leading-relaxed">
                <MarkdownContent content={message.content} />
                {message.pending && <StreamingCursor />}
              </div>
            )
          )}
          {message.error && (
            <p className="mt-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {message.error}
            </p>
          )}
          {message.citations.length > 0 && (
            <div className="mt-3 border-t border-border/60 pt-2">
              <span className="mr-2 text-xs text-muted-foreground">来源</span>
              <span className="inline-flex flex-wrap gap-1.5 align-middle">
                {message.citations.map((citation) => (
                  <CitationChip key={citation.id} citation={citation} onClick={() => onCitationClick(citation)} />
                ))}
              </span>
            </div>
          )}
        </div>
      </div>
    </article>
  );
}
```

注意：本步引用了 `CitationChip`（Task 8 创建）。**Task 7 与 Task 8 需要连续执行**，本 Task 结束时 typecheck 会因缺 `citation-chip` 失败——所以本 Task 的提交合并到 Task 8 末尾一起做（见 Task 8 Step 6）。若执行者希望每 Task 独立编译通过，可先在本 Task 内创建 Task 8 Step 3 的 `citation-chip.tsx` 再一起提交。

- [ ] **Step 4: 改 message-list 空态**

覆盖 `frontend/components/chat/message-list.tsx` 为：

```tsx
"use client";

import { useEffect, useRef } from "react";
import { MessageSquarePlus } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { MessageBubble } from "@/components/chat/message-bubble";
import type { LocalChatMessage } from "@/lib/hooks/use-chat-stream";
import type { Citation } from "@/lib/schemas";

export function MessageList({
  messages,
  onCitationClick,
}: {
  messages: LocalChatMessage[];
  onCitationClick: (citation: Citation) => void;
}) {
  const endRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: "end" });
  }, [messages]);

  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
      {messages.length === 0 ? (
        <EmptyState
          icon={MessageSquarePlus}
          title="向这个知识库提问吧"
          description="从左侧选择或新建会话后输入问题，回答会流式输出并附带可点击的引用来源。"
          className="h-full py-0"
        />
      ) : (
        <div className="mx-auto max-w-3xl space-y-5">
          {messages.map((message) => (
            <MessageBubble key={message.id} message={message} onCitationClick={onCitationClick} />
          ))}
          <div ref={endRef} />
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 5: 删除 tool-call-trace.tsx**

```bash
git rm components/chat/tool-call-trace.tsx
```

（`message-bubble` 已不再引用；用 `grep -r "tool-call-trace" --include="*.tsx" --include="*.ts" .` 确认全仓无残留引用。）

---

### Task 8: 引用 chip + Sheet 抽屉

**Files:**
- Create: `frontend/components/chat/citation-chip.tsx`
- Delete: `frontend/components/chat/citation-card.tsx`
- Modify: `frontend/components/chat/citation-drawer.tsx`（全量重写）

- [ ] **Step 1: 实现 citation-chip**

创建 `frontend/components/chat/citation-chip.tsx`：

```tsx
"use client";

import type { Citation } from "@/lib/schemas";

export function CitationChip({ citation, onClick }: { citation: Citation; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={citation.snippet}
      className="inline-flex max-w-60 items-center gap-1.5 rounded-md border border-primary/30 bg-primary/5 px-2 py-1 text-xs text-primary transition-colors hover:bg-primary/10"
    >
      <span className="font-medium">[{citation.id.replace(/^c/, "")}]</span>
      <span className="truncate">{citation.document_title}</span>
      <span className="shrink-0 text-primary/70">{citation.score.toFixed(2)}</span>
    </button>
  );
}
```

- [ ] **Step 2: 删除 citation-card.tsx**

```bash
git rm components/chat/citation-card.tsx
```

- [ ] **Step 3: citation-drawer 换 Sheet + 中文化**

覆盖 `frontend/components/chat/citation-drawer.tsx` 为（`Sheet` 的具体 props 以 Task 3 生成的 `components/ui/sheet.tsx` 实际导出为准）：

```tsx
"use client";

import { useQuery } from "@tanstack/react-query";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { chatApi } from "@/lib/api/chat";
import { cn } from "@/lib/utils";
import type { Citation } from "@/lib/schemas";

export function CitationDrawer({
  kbId,
  citation,
  onOpenChange,
}: {
  kbId: string;
  citation: Citation | null;
  onOpenChange: (citation: Citation | null) => void;
}) {
  const query = useQuery({
    queryKey: ["chunk-neighbors", kbId, citation?.chunk_id],
    queryFn: () => chatApi.getNeighbors(kbId, citation!.chunk_id, 1),
    enabled: !!citation,
  });

  return (
    <Sheet open={!!citation} onOpenChange={(open) => !open && onOpenChange(null)}>
      <SheetContent side="right" className="flex w-full flex-col sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{citation?.document_title ?? "引用"}</SheetTitle>
          <SheetDescription>{citation ? `切片 #${citation.seq} 及相邻上下文` : ""}</SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
          {query.isLoading && (
            <div className="space-y-3">
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
              <Skeleton className="h-24 w-full" />
            </div>
          )}
          {query.isError && <p className="text-sm text-destructive">{(query.error as Error).message}</p>}
          {query.data && query.data.chunks.length === 0 && (
            <p className="text-sm text-muted-foreground">没有相邻切片。</p>
          )}
          {query.data && query.data.chunks.length > 0 && (
            <div className="space-y-3">
              {query.data.chunks.map((chunk) => (
                <article
                  key={chunk.id}
                  className={cn(
                    "rounded-lg border bg-card p-3 text-sm shadow-sm",
                    chunk.is_primary && "border-primary bg-primary/5",
                  )}
                >
                  <div className="mb-2 text-xs text-muted-foreground">#{chunk.seq}</div>
                  <pre className="whitespace-pre-wrap font-sans leading-relaxed">{chunk.content}</pre>
                </article>
              ))}
            </div>
          )}
        </div>
      </SheetContent>
    </Sheet>
  );
}
```

- [ ] **Step 4: 跑 Task 7 的测试确认通过**

Run: `pnpm vitest run components/chat/message-bubble.test.tsx`
Expected: PASS

- [ ] **Step 5: 全量验证**

Run: `pnpm lint && pnpm typecheck && pnpm test`
Expected: 全绿（Task 7 + 8 的改动此时全部编译通过）

- [ ] **Step 6: Commit（含 Task 7 改动）**

```bash
git add components/chat/ 
git commit -m "feat(frontend): document-flow assistant messages, citation chips and Sheet drawer

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: chat-input Textarea 化 + chat-header Tabs 化

**Files:**
- Modify: `frontend/components/chat/chat-input.tsx`（全量重写）
- Modify: `frontend/components/chat/chat-header.tsx`（全量重写）
- Test: `frontend/components/chat/chat-input.test.tsx`

- [ ] **Step 1: 写 chat-input 失败测试**

创建 `frontend/components/chat/chat-input.test.tsx`：

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ChatInput } from "./chat-input";

describe("ChatInput", () => {
  it("sends trimmed content on Enter and clears input", async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    render(<ChatInput onSend={onSend} onStop={vi.fn()} />);
    const box = screen.getByPlaceholderText(/输入问题/);
    await user.type(box, "  端口不通  {Enter}");
    expect(onSend).toHaveBeenCalledWith("端口不通");
    expect(box).toHaveValue("");
  });

  it("inserts newline on Shift+Enter without sending", async () => {
    const user = userEvent.setup();
    const onSend = vi.fn();
    render(<ChatInput onSend={onSend} onStop={vi.fn()} />);
    const box = screen.getByPlaceholderText(/输入问题/);
    await user.type(box, "第一行{Shift>}{Enter}{/Shift}第二行");
    expect(onSend).not.toHaveBeenCalled();
    expect(box).toHaveValue("第一行\n第二行");
  });

  it("shows stop button and disables send while streaming", () => {
    render(<ChatInput disabled onSend={vi.fn()} onStop={vi.fn()} />);
    expect(screen.getByRole("button", { name: "停止生成" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "发送" })).toBeDisabled();
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm vitest run components/chat/chat-input.test.tsx`
Expected: FAIL（placeholder 是英文，按钮无中文 aria-label）

- [ ] **Step 3: 重写 chat-input**

覆盖 `frontend/components/chat/chat-input.tsx` 为：

```tsx
"use client";

import { FormEvent, KeyboardEvent, useEffect, useRef, useState } from "react";
import { Send, Square } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

export function ChatInput({
  disabled,
  onSend,
  onStop,
}: {
  disabled?: boolean;
  onSend: (content: string) => void;
  onStop: () => void;
}) {
  const [value, setValue] = useState("");
  const boxRef = useRef<HTMLTextAreaElement | null>(null);
  const prevDisabledRef = useRef(disabled);

  useEffect(() => {
    // 流式生成结束（disabled true→false）时把焦点还给输入框
    if (prevDisabledRef.current && !disabled) {
      boxRef.current?.focus();
    }
    prevDisabledRef.current = disabled;
  }, [disabled]);

  function autoResize() {
    const box = boxRef.current;
    if (!box) return;
    box.style.height = "auto";
    box.style.height = `${Math.min(box.scrollHeight, 160)}px`; // 上限约 6 行
  }

  function submit() {
    const trimmed = value.trim();
    if (!trimmed || disabled) return;
    onSend(trimmed);
    setValue("");
    requestAnimationFrame(autoResize);
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    submit();
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
      event.preventDefault();
      submit();
    }
  }

  return (
    <form onSubmit={onSubmit} className="border-t bg-background p-3">
      <div className="mx-auto flex max-w-3xl items-end gap-2">
        <Textarea
          ref={boxRef}
          value={value}
          onChange={(event) => {
            setValue(event.target.value);
            autoResize();
          }}
          onKeyDown={onKeyDown}
          rows={2}
          disabled={disabled}
          placeholder="输入问题，Enter 发送，Shift+Enter 换行"
          className={cn("max-h-40 min-h-16 flex-1 resize-none")}
        />
        <div className="flex shrink-0 gap-2">
          <Button type="submit" size="icon" aria-label="发送" disabled={disabled || value.trim() === ""}>
            <Send className="size-4" />
          </Button>
          {disabled && (
            <Button type="button" size="icon" variant="outline" aria-label="停止生成" onClick={onStop}>
              <Square className="size-4" />
            </Button>
          )}
        </div>
      </div>
    </form>
  );
}
```

（若 Task 3 生成的 `Textarea` 不接受 `ref` prop——React 19 下函数组件透传 ref 通常没问题——改用其实际支持的方式，必要时包一层 `useEffect` 查询 DOM。）

> **勘误（code review 修正，已落地）**：
> 1. Enter 发送必须加 `!event.nativeEvent.isComposing` 守卫——否则中文输入法组词期间按 Enter（确认候选词）会把未上屏的拼音直接发送。配套回归测试：`fireEvent.keyDown(box, { key: "Enter", isComposing: true })` 断言 onSend 未被调用（jsdom 会把 `isComposing` 映射到 `nativeEvent.isComposing`）。
> 2. 流式生成期间 textarea 被 disabled 会丢焦点，需在 `disabled` true→false 转变时 `boxRef.current?.focus()` 还焦点（用 ref 记录前值，避免首挂载误触发）。

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm vitest run components/chat/chat-input.test.tsx`
Expected: PASS

- [ ] **Step 5: 重写 chat-header（Tabs + 中文 + SidebarTrigger，删 Docs/KBs 链接）**

覆盖 `frontend/components/chat/chat-header.tsx` 为（`Tabs` API 以生成组件为准；base-nova Tabs 通常是 `Tabs value onValueChange` + `TabsList` + `TabsTrigger value`）：

```tsx
"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import { SidebarTrigger } from "@/components/ui/sidebar";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { chatApi } from "@/lib/api/chat";
import type { Conversation } from "@/lib/schemas";

export function ChatHeader({
  kbId,
  conversation,
  disabled = false,
}: {
  kbId: string;
  conversation?: Conversation | null;
  disabled?: boolean;
}) {
  const queryClient = useQueryClient();
  const modeMutation = useMutation({
    mutationFn: (mode: Conversation["mode"]) =>
      chatApi.updateConversationMode(conversation!.id, mode),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["conversations", kbId] }),
    onError: (error) => toast.error(`切换模式失败：${(error as Error).message}`),
  });

  const subtitle = conversation
    ? conversation.mode === "react"
      ? "ReAct Agent"
      : "确定性 RAG"
    : "选择或新建会话";

  return (
    <header className="flex min-h-14 items-center justify-between gap-3 border-b px-4">
      <div className="flex min-w-0 items-center gap-2">
        <SidebarTrigger className="md:hidden" />
        <div className="min-w-0">
          <h1 className="truncate text-base font-semibold">{conversation?.title || "新对话"}</h1>
          <p className="truncate text-xs text-muted-foreground">{subtitle}</p>
        </div>
      </div>
      {conversation && (
        <Tabs
          value={conversation.mode}
          onValueChange={(value) => {
            if (value !== conversation.mode && !disabled && !modeMutation.isPending) {
              modeMutation.mutate(value as Conversation["mode"]);
            }
          }}
        >
          <TabsList>
            <TabsTrigger value="rag" disabled={disabled || modeMutation.isPending}>
              RAG
            </TabsTrigger>
            <TabsTrigger value="react" disabled={disabled || modeMutation.isPending}>
              Agent
            </TabsTrigger>
          </TabsList>
        </Tabs>
      )}
    </header>
  );
}
```

- [ ] **Step 6: 全量验证 + Commit**

Run: `pnpm lint && pnpm typecheck && pnpm test`
Expected: 全绿

```bash
git add components/chat/chat-input.tsx components/chat/chat-input.test.tsx components/chat/chat-header.tsx
git commit -m "feat(frontend): textarea chat input with autosize, tabs mode switch, chinese copy

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: AppShell（last-kb + rail + chat-panel + kb-panel + 组装）

**Files:**
- Create: `frontend/lib/last-kb.ts`
- Create: `frontend/components/layout/rail.tsx`
- Create: `frontend/components/layout/chat-panel.tsx`
- Create: `frontend/components/layout/kb-panel.tsx`
- Create: `frontend/components/layout/app-shell.tsx`
- Test: `frontend/lib/last-kb.test.ts`

- [ ] **Step 1: 写 last-kb 失败测试**

创建 `frontend/lib/last-kb.test.ts`：

```ts
import { beforeEach, describe, expect, it } from "vitest";

import { getLastKbId, setLastKbId } from "./last-kb";

describe("last-kb storage", () => {
  beforeEach(() => localStorage.clear());

  it("returns null when nothing stored", () => {
    expect(getLastKbId()).toBeNull();
  });

  it("round-trips the last visited kb id", () => {
    setLastKbId("kb-123");
    expect(getLastKbId()).toBe("kb-123");
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `pnpm vitest run lib/last-kb.test.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现 last-kb**

创建 `frontend/lib/last-kb.ts`：

```ts
const KEY = "it-wiki:last-kb-id";

export function getLastKbId(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(KEY);
}

export function setLastKbId(kbId: string): void {
  if (typeof window === "undefined") return;
  localStorage.setItem(KEY, kbId);
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `pnpm vitest run lib/last-kb.test.ts`
Expected: PASS

- [ ] **Step 5: 实现 rail**

创建 `frontend/components/layout/rail.tsx`。rail 是一条固定窄列（不用 Sidebar primitive，纯 flex 列，永远可见）：

```tsx
"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { BookOpen, Library, MessageSquare, Settings } from "lucide-react";

import { ThemeToggle } from "@/components/layout/theme-toggle";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getLastKbId } from "@/lib/last-kb";
import { cn } from "@/lib/utils";

export type RailSection = "chats" | "kbs";

function RailItem({
  section,
  active,
  href,
  icon: Icon,
  label,
}: {
  section: RailSection;
  active: boolean;
  href: string;
  icon: typeof MessageSquare;
  label: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Link
            href={href}
            data-section={section}
            aria-label={label}
            className={cn(
              "flex size-9 items-center justify-center rounded-lg transition-colors",
              active
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-accent hover:text-foreground",
            )}
          >
            <Icon className="size-4" />
          </Link>
        }
      />
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

export function Rail() {
  const pathname = usePathname();
  const params = useParams<{ kbId?: string }>();
  const activeSection: RailSection = pathname.includes("/chats") ? "chats" : "kbs";
  // localStorage 只能客户端读；用 state + effect 避免 SSR/CSR href 不一致的 hydration 告警
  const [lastKb, setLastKb] = useState<string | null>(null);
  useEffect(() => setLastKb(getLastKbId()), [pathname]);
  const kbId = params.kbId ?? lastKb;

  return (
    <div className="flex w-14 shrink-0 flex-col items-center gap-2 border-r border-sidebar-border bg-sidebar py-3">
      <div className="mb-2 flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
        <BookOpen className="size-5" />
      </div>
      <RailItem
        section="chats"
        active={activeSection === "chats"}
        href={kbId ? `/kbs/${kbId}/chats` : "/"}
        icon={MessageSquare}
        label="聊天"
      />
      <RailItem
        section="kbs"
        active={activeSection === "kbs"}
        href={kbId ? `/kbs/${kbId}/docs` : "/kbs"}
        icon={Library}
        label="知识库"
      />
      <div className="mt-auto flex flex-col items-center gap-1">
        <ThemeToggle />
        <Button type="button" variant="ghost" size="icon" aria-label="设置（即将上线）" disabled>
          <Settings className="size-4" />
        </Button>
      </div>
    </div>
  );
}
```

（文件顶部相应补 `import { useEffect, useState } from "react";` 与 `useParams` 导入：`import { useParams, usePathname } from "next/navigation";`）

（`TooltipTrigger` 的 `render` prop 用法以 Task 3 生成组件为准；若其 API 是 children 包裹形式则相应调整。）

- [ ] **Step 6: 实现 chat-panel（会话列表面板，吸收原 chat-sidebar 逻辑）**

创建 `frontend/components/layout/chat-panel.tsx`：

```tsx
"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, ChevronsUpDown, Plus } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import { chatApi } from "@/lib/api/chat";
import { kbApi } from "@/lib/api/kb";
import { cn } from "@/lib/utils";

export function ChatPanel() {
  const params = useParams<{ kbId?: string; conversationId?: string }>();
  const kbId = params.kbId ?? "";
  const selectedConversationId = params.conversationId;
  const router = useRouter();
  const queryClient = useQueryClient();

  const kbs = useQuery({ queryKey: ["kbs"], queryFn: () => kbApi.list() });
  const currentKb = kbs.data?.items.find((kb) => kb.id === kbId) ?? null;

  const conversations = useQuery({
    queryKey: ["conversations", kbId],
    queryFn: () => chatApi.listConversations(kbId, 50, 0),
    enabled: !!kbId,
  });
  const createConversation = useMutation({
    mutationFn: () => chatApi.createConversation(kbId),
    onSuccess: (conversation) => {
      queryClient.invalidateQueries({ queryKey: ["conversations", kbId] });
      router.push(`/kbs/${kbId}/chats/${conversation.id}`);
    },
    onError: (error) => toast.error(`新建会话失败：${(error as Error).message}`),
  });

  return (
    <div className="flex h-full flex-col">
      <div className="border-b p-3">
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button variant="outline" className="w-full justify-between font-medium">
                <span className="truncate">{currentKb?.name ?? "选择知识库"}</span>
                <ChevronsUpDown className="size-4 shrink-0 text-muted-foreground" />
              </Button>
            }
          />
          <DropdownMenuContent className="w-56">
            {kbs.data?.items.map((kb) => (
              <DropdownMenuItem key={kb.id} onClick={() => router.push(`/kbs/${kb.id}/chats`)}>
                <span className="truncate">{kb.name}</span>
                {kb.id === kbId && <Check className="ml-auto size-4" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          className="mt-2 w-full"
          onClick={() => createConversation.mutate()}
          disabled={!kbId || createConversation.isPending}
        >
          <Plus className="size-4" />
          新会话
        </Button>
      </div>

      <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
        {conversations.isLoading && (
          <div className="space-y-2 p-1">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        )}
        {conversations.isError && (
          <p className="p-2 text-xs text-destructive">{(conversations.error as Error).message}</p>
        )}
        {conversations.data?.items.length === 0 && (
          <p className="p-2 text-xs leading-relaxed text-muted-foreground">还没有会话，点击「新会话」开始提问。</p>
        )}
        {conversations.data?.items.map((conversation) => (
          <Link
            key={conversation.id}
            href={`/kbs/${kbId}/chats/${conversation.id}`}
            className={cn(
              "block rounded-md border-l-2 border-transparent px-3 py-2 text-sm transition-colors hover:bg-accent",
              conversation.id === selectedConversationId && "border-primary bg-accent",
            )}
          >
            <span className="block truncate font-medium">{conversation.title || "新对话"}</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              {new Date(conversation.updated_at).toLocaleString("zh-CN")}
            </span>
          </Link>
        ))}
      </nav>
    </div>
  );
}
```

说明：`kbApi.list(limit = 20, offset = 0)` 有默认参数，无参调用成立（已核实 `lib/api/kb.ts:11`）。会话列表项不做 hover 删除操作——后端没有删除会话的 API（`lib/api/chat.ts` 已核实），spec §5.4 已同步修正。

- [ ] **Step 7: 实现 kb-panel（KB 列表面板，吸收原 kb-card 的删除逻辑 + 新建入口）**

创建 `frontend/components/layout/kb-panel.tsx`：

```tsx
"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";

import { KBCreateDialog } from "@/components/kb/kb-create-dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useDeleteKb, useKbs } from "@/lib/hooks/use-kbs";
import { cn } from "@/lib/utils";
import type { KB } from "@/lib/schemas";

function KbRow({ kb, active }: { kb: KB; active: boolean }) {
  const del = useDeleteKb();

  return (
    <div
      className={cn(
        "group flex items-center gap-1 rounded-md border-l-2 border-transparent pr-1 transition-colors hover:bg-accent",
        active && "border-primary bg-accent",
      )}
    >
      <Link href={`/kbs/${kb.id}/docs`} className="min-w-0 flex-1 px-3 py-2">
        <span className="block truncate text-sm font-medium">{kb.name}</span>
        <span className="block truncate text-xs text-muted-foreground">{kb.description || "（无描述）"}</span>
      </Link>
      <AlertDialog>
        <AlertDialogTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={`删除 ${kb.name}`}
              className="opacity-0 transition-opacity group-hover:opacity-100"
            >
              <Trash2 className="size-3.5" />
            </Button>
          }
        />
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除知识库「{kb.name}」？</AlertDialogTitle>
            <AlertDialogDescription>其中所有文档和切片会一并删除，此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() =>
                del.mutate(kb.id, {
                  onSuccess: () => toast.success("已删除"),
                  onError: (e) => toast.error(`删除失败：${(e as Error).message}`),
                })
              }
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export function KbPanel() {
  const params = useParams<{ kbId?: string }>();
  const { data, isLoading, isError, error } = useKbs();

  return (
    <div className="flex h-full flex-col">
      <div className="border-b p-3">
        <KBCreateDialog />
      </div>
      <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
        {isLoading && (
          <div className="space-y-2 p-1">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        )}
        {isError && <p className="p-2 text-xs text-destructive">{(error as Error).message}</p>}
        {data?.items.length === 0 && (
          <p className="p-2 text-xs leading-relaxed text-muted-foreground">还没有知识库，点击上方按钮创建。</p>
        )}
        {data?.items.map((kb) => (
          <KbRow key={kb.id} kb={kb} active={kb.id === params.kbId} />
        ))}
      </nav>
    </div>
  );
}
```

注意：`KBCreateDialog` 的触发按钮需要撑满面板宽度——顺手把 `components/kb/kb-create-dialog.tsx` 里 `<DialogTrigger render={<Button />}>` 改为 `<DialogTrigger render={<Button className="w-full" />}>`（其余不动）。

- [ ] **Step 8: 实现 app-shell 组装**

创建 `frontend/components/layout/app-shell.tsx`。二级面板用 shadcn Sidebar（获得移动端 Sheet 抽屉与收起能力），rail 固定在最左：

```tsx
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
      <SidebarProvider className="min-w-0 flex-1">
        <Sidebar collapsible="offcanvas" className="border-r">
          <SidebarContent>{isChats ? <ChatPanel /> : <KbPanel />}</SidebarContent>
        </Sidebar>
        <SidebarInset className="flex min-w-0 flex-col overflow-hidden">{children}</SidebarInset>
      </SidebarProvider>
    </div>
  );
}
```

（`SidebarProvider`/`Sidebar` 的 className 透传与 `collapsible` 值以生成组件为准；目标行为：桌面端面板常驻可收起，移动端变 Sheet 抽屉由 `SidebarTrigger` 唤出——Task 9 已在 chat-header 放了 `SidebarTrigger className="md:hidden"`。若生成的 SidebarProvider 自带 `h-svh w-full` 之类的样式与外层 flex 冲突，用 className 覆盖。）

- [ ] **Step 9: 全量验证 + Commit**

Run: `pnpm lint && pnpm typecheck && pnpm test`
Expected: 全绿（AppShell 尚未挂到路由，无运行时影响）

```bash
git add lib/last-kb.ts lib/last-kb.test.ts components/layout/ components/kb/kb-create-dialog.tsx
git commit -m "feat(frontend): app shell with icon rail, chat/kb panels and last-kb memory

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: 路由收编（layout 挂载 + 页面适配 + `/` 重定向）

**Files:**
- Create: `frontend/app/kbs/layout.tsx`
- Create: `frontend/app/kbs/page.tsx`
- Modify: `frontend/app/page.tsx`（全量重写）
- Modify: `frontend/app/kbs/[kbId]/chats/page.tsx`（全量重写）
- Modify: `frontend/app/kbs/[kbId]/chats/[conversationId]/page.tsx`（全量重写）
- Modify: `frontend/app/kbs/[kbId]/docs/page.tsx`（全量重写）
- Modify: `frontend/app/kbs/[kbId]/docs/[docId]/page.tsx`（局部修改）
- Delete: `frontend/components/chat/chat-sidebar.tsx`、`frontend/components/kb/kb-card.tsx`

- [ ] **Step 1: 创建 app/kbs/layout.tsx**

```tsx
import type { ReactNode } from "react";

import { AppShell } from "@/components/layout/app-shell";

export default function KbsLayout({ children }: { children: ReactNode }) {
  return <AppShell>{children}</AppShell>;
}
```

- [ ] **Step 2: 创建 app/kbs/page.tsx（知识库区无选中页）**

```tsx
"use client";

import { Library } from "lucide-react";

import { EmptyState } from "@/components/common/empty-state";
import { useKbs } from "@/lib/hooks/use-kbs";

export default function KbsIndexPage() {
  const { data } = useKbs();
  const hasKbs = (data?.items.length ?? 0) > 0;

  return (
    <main className="flex flex-1 items-center justify-center">
      <EmptyState
        icon={Library}
        title={hasKbs ? "选择一个知识库" : "创建第一个知识库"}
        description={
          hasKbs
            ? "从左侧列表选择知识库，管理它的文档。"
            : "点击左侧「新建知识库」按钮，然后上传文档开始构建。"
        }
      />
    </main>
  );
}
```

- [ ] **Step 3: 重写 app/page.tsx 为重定向（spec D8）**

```tsx
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
```

- [ ] **Step 4: 重写聊天两页（去外壳，适配 AppShell）**

覆盖 `frontend/app/kbs/[kbId]/chats/page.tsx` 为：

```tsx
"use client";

import { use, useState } from "react";

import { ChatHeader } from "@/components/chat/chat-header";
import { ChatInput } from "@/components/chat/chat-input";
import { CitationDrawer } from "@/components/chat/citation-drawer";
import { MessageList } from "@/components/chat/message-list";
import type { Citation } from "@/lib/schemas";

export default function KBChatsPage({ params }: { params: Promise<{ kbId: string }> }) {
  const { kbId } = use(params);
  const [selectedCitation, setSelectedCitation] = useState<Citation | null>(null);

  return (
    <>
      <ChatHeader kbId={kbId} />
      <MessageList messages={[]} onCitationClick={setSelectedCitation} />
      <ChatInput disabled onSend={() => undefined} onStop={() => undefined} />
      <CitationDrawer kbId={kbId} citation={selectedCitation} onOpenChange={setSelectedCitation} />
    </>
  );
}
```

覆盖 `frontend/app/kbs/[kbId]/chats/[conversationId]/page.tsx` 为：

```tsx
"use client";

import { use, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { ChatHeader } from "@/components/chat/chat-header";
import { ChatInput } from "@/components/chat/chat-input";
import { CitationDrawer } from "@/components/chat/citation-drawer";
import { MessageList } from "@/components/chat/message-list";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { chatApi } from "@/lib/api/chat";
import { useChatStream } from "@/lib/hooks/use-chat-stream";
import type { ChatMessage, Citation } from "@/lib/schemas";

const EMPTY_MESSAGES: ChatMessage[] = [];

export default function ConversationPage({
  params,
}: {
  params: Promise<{ kbId: string; conversationId: string }>;
}) {
  const { kbId, conversationId } = use(params);
  const [selectedCitation, setSelectedCitation] = useState<Citation | null>(null);
  const conversations = useQuery({
    queryKey: ["conversations", kbId],
    queryFn: () => chatApi.listConversations(kbId, 50, 0),
    enabled: !!kbId,
  });
  const messagesQuery = useQuery({
    queryKey: ["messages", conversationId],
    queryFn: () => chatApi.listMessages(conversationId, 100, 0),
    enabled: !!conversationId,
  });
  const conversation = useMemo(
    () => conversations.data?.items.find((item) => item.id === conversationId) ?? null,
    [conversationId, conversations.data?.items],
  );
  const chat = useChatStream(messagesQuery.data?.items ?? EMPTY_MESSAGES);

  return (
    <>
      <ChatHeader kbId={kbId} conversation={conversation} disabled={chat.isStreaming} />
      {(messagesQuery.isError || chat.error) && (
        <Alert variant="destructive" className="mx-4 mt-2">
          <AlertDescription>
            {messagesQuery.isError ? (messagesQuery.error as Error).message : chat.error}
          </AlertDescription>
        </Alert>
      )}
      <MessageList messages={chat.messages} onCitationClick={setSelectedCitation} />
      <ChatInput
        disabled={chat.isStreaming || messagesQuery.isLoading}
        onSend={(content) => chat.send(conversationId, content)}
        onStop={chat.stop}
      />
      <CitationDrawer kbId={kbId} citation={selectedCitation} onOpenChange={setSelectedCitation} />
    </>
  );
}
```

（`Alert` 的 variant/子组件名以 Task 3 生成组件为准。）

- [ ] **Step 5: 重写 docs 列表页 + 微调详情页**

覆盖 `frontend/app/kbs/[kbId]/docs/page.tsx` 为：

```tsx
"use client";

import { use } from "react";
import { FileText } from "lucide-react";

import { DocTable } from "@/components/docs/doc-table";
import { DocUploader } from "@/components/docs/doc-uploader";
import { EmptyState } from "@/components/common/empty-state";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocsByKB } from "@/lib/hooks/use-docs";
import { useKbs } from "@/lib/hooks/use-kbs";

export default function KBDocsPage({ params }: { params: Promise<{ kbId: string }> }) {
  const { kbId } = use(params);
  const { data, isLoading, isError, error } = useDocsByKB(kbId);
  const kbs = useKbs();
  const kbName = kbs.data?.items.find((kb) => kb.id === kbId)?.name;

  return (
    <div className="flex h-full flex-col">
      <header className="flex min-h-14 items-center gap-2 border-b px-4">
        <SidebarTrigger className="md:hidden" />
        <div>
          <h1 className="text-base font-semibold">{kbName ?? "文档管理"}</h1>
          <p className="text-xs text-muted-foreground">上传与管理该知识库的文档</p>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
        <div className="mx-auto max-w-4xl space-y-6">
          <DocUploader kbId={kbId} />

          {isLoading && (
            <div className="space-y-2">
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
              <Skeleton className="h-10 w-full" />
            </div>
          )}
          {isError && <p className="text-sm text-destructive">加载失败：{(error as Error).message}</p>}
          {data && data.items.length === 0 && (
            <EmptyState icon={FileText} title="还没有文档" description="拖入或选择文件上传，处理完成后即可对话检索。" />
          )}
          {data && data.items.length > 0 && (
            <div className="rounded-lg border border-border/50">
              <DocTable kbId={kbId} docs={data.items} />
            </div>
          )}
          {data && data.items.length > 0 && (
            <p className="text-xs text-muted-foreground">共 {data.total} 个文档</p>
          )}
        </div>
      </div>
    </div>
  );
}
```

修改 `frontend/app/kbs/[kbId]/docs/[docId]/page.tsx`：

1. 外层 `<main className="container mx-auto max-w-5xl p-8">` 改为 `<main className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6"><div className="mx-auto max-w-4xl"> ... </div></main>`（补一个对应的闭合 `</div>`）
2. 顶部返回链接保留但指向文档列表且中文不变（`href={`/kbs/${kbId}/docs`}`，原样）
3. `加载中...` 段落替换为：

```tsx
{isLoading && (
  <div className="space-y-3">
    <Skeleton className="h-8 w-64" />
    <Skeleton className="h-4 w-40" />
    <Skeleton className="h-32 w-full" />
  </div>
)}
```

并在文件顶部加 `import { Skeleton } from "@/components/ui/skeleton";`

- [ ] **Step 6: 删除被替代组件并确认无引用**

```bash
git rm components/chat/chat-sidebar.tsx components/kb/kb-card.tsx
grep -rn "chat-sidebar\|ChatSidebar\|kb-card\|KBCard" --include="*.tsx" --include="*.ts" app components lib
```

Expected: grep 无输出（若有残留引用，回到对应文件清理）。

- [ ] **Step 7: 手动烟测**

Run: `pnpm dev`，浏览器验证：
- `/` 重定向：无 KB 时到 `/kbs` 引导页；有 KB 时进聊天区
- rail 两个图标可切换聊天/知识库功能区，选中态紫罗兰
- 聊天区：面板显示会话列表，可新建会话、切换 KB；知识库区：KB 列表 + 文档管理
- 明暗切换生效且刷新保持
- 窗口缩窄到移动端宽度：header 出现汉堡键，点击唤出面板抽屉

- [ ] **Step 8: 全量验证 + Commit**

Run: `pnpm lint && pnpm typecheck && pnpm test`
Expected: 全绿

```bash
git add app/ components/
git commit -m "feat(frontend): mount app shell on kbs routes, root redirect, remove page-level sidebars

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: 知识库区精修（doc-table 确认框/Tooltip + chunk-list 骨架屏）

**Files:**
- Modify: `frontend/components/docs/doc-table.tsx`（全量重写）
- Modify: `frontend/components/chunks/chunk-list.tsx`（全量重写）

- [ ] **Step 1: 重写 doc-table（AlertDialog 删除确认 + 图标按钮 Tooltip + 表头精修）**

覆盖 `frontend/components/docs/doc-table.tsx` 为：

```tsx
"use client";

import Link from "next/link";
import { RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { IngestStatusBadge } from "./ingest-status-badge";
import { useDeleteDoc, useReingestDoc } from "@/lib/hooks/use-docs";
import type { Doc } from "@/lib/schemas";

function fmt(bytes: number): string {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / 1024 / 1024).toFixed(1) + " MB";
}

export function DocTable({ kbId, docs }: { kbId: string; docs: Doc[] }) {
  const del = useDeleteDoc(kbId);
  const reingest = useReingestDoc(kbId);

  function onReingest(d: Doc) {
    reingest.mutate(d.id, {
      onSuccess: () => toast.success("已重新入队处理"),
      onError: (e) => toast.error(`重新处理失败：${(e as Error).message}`),
    });
  }

  function onDelete(d: Doc) {
    del.mutate(d.id, {
      onSuccess: () => toast.success("已删除"),
      onError: (e) => toast.error(`删除失败：${(e as Error).message}`),
    });
  }

  return (
    <Table>
      <TableHeader>
        <TableRow className="bg-muted/50">
          <TableHead>文件</TableHead>
          <TableHead>状态</TableHead>
          <TableHead>大小</TableHead>
          <TableHead>上传时间</TableHead>
          <TableHead className="w-24" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {docs.map((d) => (
          <TableRow key={d.id}>
            <TableCell>
              <Link href={`/kbs/${kbId}/docs/${d.id}`} className="font-medium hover:text-primary hover:underline">
                {d.title}
              </Link>
              {d.status === "failed" && d.error_message && (
                <p className="mt-1 line-clamp-1 text-xs text-destructive">{d.error_message}</p>
              )}
            </TableCell>
            <TableCell><IngestStatusBadge status={d.status} /></TableCell>
            <TableCell className="text-muted-foreground">{fmt(d.bytes)}</TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {new Date(d.created_at).toLocaleString("zh-CN")}
            </TableCell>
            <TableCell>
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label="重新处理"
                      onClick={() => onReingest(d)}
                      disabled={reingest.isPending || (d.status !== "ready" && d.status !== "failed")}
                    >
                      <RefreshCw className="size-4" />
                    </Button>
                  }
                />
                <TooltipContent>重新处理</TooltipContent>
              </Tooltip>
              <AlertDialog>
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <AlertDialogTrigger
                        render={
                          <Button size="icon-sm" variant="ghost" aria-label="删除" disabled={del.isPending}>
                            <Trash2 className="size-4" />
                          </Button>
                        }
                      />
                    }
                  />
                  <TooltipContent>删除</TooltipContent>
                </Tooltip>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>删除文档「{d.title}」？</AlertDialogTitle>
                    <AlertDialogDescription>切片会一并删除，此操作不可撤销。</AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>取消</AlertDialogCancel>
                    <AlertDialogAction onClick={() => onDelete(d)}>删除</AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
```

（Tooltip 与 AlertDialogTrigger 的嵌套 `render` 组合若在 base-nova 下打架，降级为只留 AlertDialogTrigger、舍弃删除按钮的 Tooltip——`aria-label` 已保证可达性。若 Button 没有 `icon-sm` 尺寸，用 `size="icon"`。）

- [ ] **Step 2: 重写 chunk-list（Skeleton + 卡片规范）**

覆盖 `frontend/components/chunks/chunk-list.tsx` 为：

```tsx
"use client";

import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useChunks } from "@/lib/hooks/use-chunks";

const PAGE_SIZE = 10;

export function ChunkList({ docId }: { docId: string }) {
  const [page, setPage] = useState(0);
  const { data, isLoading, isError, error } = useChunks(docId, PAGE_SIZE, page * PAGE_SIZE);

  if (isLoading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-24 w-full" />
      </div>
    );
  }
  if (isError) return <p className="text-sm text-destructive">加载失败：{(error as Error).message}</p>;
  if (!data || data.items.length === 0) return <p className="text-sm text-muted-foreground">无切片</p>;

  const totalPages = Math.ceil(data.total / PAGE_SIZE);

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        共 {data.total} 个切片 · 第 {page + 1} / {totalPages} 页
      </p>

      <div className="space-y-3">
        {data.items.map((c) => (
          <div key={c.id} className="rounded-lg border bg-card p-4 shadow-sm">
            <div className="mb-2 text-xs text-muted-foreground">
              #{c.seq} · {c.token_count} tokens
            </div>
            <pre className="whitespace-pre-wrap font-sans text-sm leading-relaxed">{c.content}</pre>
          </div>
        ))}
      </div>

      <div className="flex items-center justify-between pt-2">
        <Button size="sm" variant="outline" disabled={page === 0} onClick={() => setPage(p => p - 1)}>上一页</Button>
        <Button size="sm" variant="outline" disabled={page + 1 >= totalPages} onClick={() => setPage(p => p + 1)}>下一页</Button>
      </div>
    </div>
  );
}
```

（doc-uploader 现有虚线拖拽卡 + 拖入高亮已符合 spec §6，不动。）

- [ ] **Step 3: 全量验证 + Commit**

Run: `pnpm lint && pnpm typecheck && pnpm test`
Expected: 全绿

```bash
git add components/docs/doc-table.tsx components/chunks/chunk-list.tsx
git commit -m "feat(frontend): alert-dialog confirms, tooltips and skeletons in kb area

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: 收尾清扫 + 文档同步 + 全量验收

**Files:**
- 全仓扫描修正残留英文文案
- Modify: `CLAUDE.md`（§2 当前阶段、§5.4 组件名）
- Modify: `docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md`（§6.1 前端目录）

- [ ] **Step 1: 扫描并清理残留英文文案**

```bash
cd frontend
grep -rn -E "\"(Loading|Failed|Ask |Select |Create |New chat|Chat mode|Thinking)" --include="*.tsx" app components | grep -v ".test."
```

对每个命中：改为中文（如仍有 `Loading chats...` → `加载会话中...`）。特别检查 `lib/api/` 与 hooks 里的 toast 文案。测试文件不用改。

- [ ] **Step 2: 确认删除文件无残留引用 + 无硬编码调色板**

```bash
grep -rn "ToolCallTrace\|CitationCard\|ChatSidebar\|KBCard" --include="*.tsx" --include="*.ts" app components lib
grep -rn -E "bg-(blue|red|green|gray|yellow)-[0-9]" --include="*.tsx" app components
```

Expected: 两个 grep 均无输出（emerald 是 Task 2 有意保留的 success 色，不在扫描列表）。

- [ ] **Step 3: 全量验证**

```bash
pnpm lint && pnpm typecheck && pnpm test
pnpm build   # 需要代理（Google Fonts）；离线环境记录跳过原因，由用户后补
```

Expected: lint/typecheck/test 全绿；build 在有代理时成功。

- [ ] **Step 4: 截图验收清单（spec §9.2，人工执行）**

`pnpm dev` 起服务，配好 `.env` 的后端跑起来后，逐项截图（**亮色 + 暗色各一张**）：

| # | 页面/状态 | 验收点 |
|---|---|---|
| 1 | `/kbs` 空 KB 引导 | EmptyState 图标+文案+面板新建按钮 |
| 2 | 知识库区（有文档） | rail 选中态、面板 KB 列表、表格表头 `bg-muted/50`、状态 badge 语义色 |
| 3 | 文档详情（chunks） | 卡片 `bg-card shadow-sm`、处理中 Skeleton |
| 4 | 聊天空态 | EmptyState 中文引导 |
| 5 | 聊天流式中（Agent 模式） | 时间线实时推进、thought 引文体、spinner 节点、打字光标 |
| 6 | 回答完成 | 时间线自动折叠为「调用了 N 个工具」、点击可展开、来源 chip、代码块 macOS 窗口+复制 |
| 7 | 引用抽屉 | Sheet 右侧滑出、主 chunk 紫罗兰高亮 |
| 8 | 删除确认 | AlertDialog（KB 与文档） |
| 9 | 移动端宽度 | 面板收成抽屉、汉堡键唤出、聊天可收发 |
| 10 | 明暗切换 | 即时生效、刷新保持、暗色无穿帮 |

行为验收：Agent 模式发问 → 刷新页面 → 历史轨迹可回看（默认折叠）；流式中点停止 → 无半截消息残留。

- [ ] **Step 5: 文档同步（spec §10）**

1. `CLAUDE.md` §2 当前进度改为：阶段 3.5 已完成（日期、spec/plan 链接），下一阶段按主 spec §7 排（阶段 4）
2. `CLAUDE.md` §5.4 第 3 步 `前端 tool-call-trace 无需改动` 改为 `前端 agent-timeline 无需改动（按 name/arguments/result 通用渲染）`
3. 主 spec §6.1 前端目录结构补 `components/layout/`（AppShell/rail/面板）与 `components/common/`（通用组件）两行

- [ ] **Step 6: Commit**

```bash
git add -- CLAUDE.md docs/superpowers/specs/2026-05-19-it-wiki-agent-design.md
git commit -m "docs: sync phase 3.5 completion into CLAUDE.md and main spec"
```

提交示例仅在本次任务已授权提交时使用，暂存前确认当前工作目录为仓库根目录并审查目标差异；其他本次相关文件逐项添加。完成后按当前授权执行后续步骤，先核对实际分支依赖；无需强制调用分支收尾 Skill，已有合并方式不重复确认。

---

## Spec 覆盖对照（自检用）

| Spec 章节 | 对应 Task |
|---|---|
| §3.1 色彩 token / §3.2 字体 / §3.3 暗色接入 + badge 语义化 | Task 2 |
| §4.1 AppShell 结构 / §4.3 响应式 | Task 10 |
| §4.2 路由映射 + D8 重定向 | Task 11 |
| §5.1 文档流消息 + 代码块 + 流式光标 | Task 4、5、7 |
| §5.2 agent-timeline | Task 6 |
| §5.3 引用 chip + Sheet | Task 8 |
| §5.4 输入区 + Tabs + 空态 + Alert + 中文化 | Task 9、7、11、13 |
| §6 知识库区（上传卡/表格/AlertDialog/详情/空态） | Task 11、12 |
| §7.1 依赖 / §7.2 CLI 补装 / §7.3 code-block / §7.4 自研组件 | Task 1、3、4、6、10 |
| §8 动效（光标/spinner/transition/reduced-motion，无 motion） | Task 2、4、6 |
| §9 测试与验收 | 各 Task 测试步骤 + Task 13 |
| §10 文档同步 | Task 13 |
