# 阶段 3.5 · UI 视觉升级设计

- 起草日期：2026-07-09
- 作者：Zenith Wang · 与 Claude 共建
- 状态：设计已确认，待实施
- 上游文档：[主 spec §6 / §7](2026-05-19-it-wiki-agent-design.md) · [阶段 3 spec §1.2 / D6](2026-07-08-phase-3-react-agent-design.md)
- 视觉参考：credit-master（`E:\MyLearn\go-project\credit-master\frontend`，Next.js 16 + Tailwind v4 + shadcn/ui）

---

## 0. 背景与目标

阶段 0~3 交付了功能完整的知识库 Agent（文档摄入 → RAG 对话 → ReAct Agent 模式），但 UI 一直按"能用"标准实现：纯灰阶默认主题无品牌色、暗色模式 CSS 写全但未接入、仅 7 个 shadcn 组件大量手写替代、加载/空态全是纯文字、无全局导航、中英文文案混排。阶段 3 spec D6 决定将视觉升级独立为阶段 3.5。

**目标**：

1. 建立品牌视觉体系（紫罗兰主色 OKLCH token + 明暗双模式），全站视觉一致
2. 全局布局重构为「图标 rail + 二级面板」结构，为未来功能区扩展（MCP、skills）预留骨架
3. 聊天界面重设计：文档流式消息排版、工具轨迹时间线、引用体验升级
4. 补齐组件与状态设计：shadcn 组件替换手写实现、骨架屏、空态、AlertDialog、Tooltip
5. 文案全面中文化
6. **后端零改动**：纯前端阶段，SSE 协议、API、数据库都不碰

## 1. 范围

### 1.1 本阶段做

- `globals.css` 设计 token 全量重写（紫罗兰主题、阴影/字距阶梯、明暗双套）
- 接入 next-themes 实现明暗切换
- 新增 `AppShell` 全局布局（rail + 二级面板 + 主内容区），5 条现有路由收编进新布局
- 聊天界面重设计（文档流消息、agent-timeline、Sheet 引用抽屉、Textarea 输入区）
- 知识库区页面精修（拖拽上传卡、表格样式、AlertDialog、空态、骨架屏）
- shadcn CLI 补装约 12 个组件；从 credit-master 拷贝 code-block
- 现有 vitest 测试同步迁移 + 新增关键组件测试

### 1.2 本阶段不做

- **增强性动效**（路由淡入、时间线节点入场动画、AnimatePresence 展开收起）：作为 3.5 之后的可选迭代，不引入 `motion` 依赖（见 D7）。功能性反馈（打字光标、spinner、transition-colors、Sidebar 自带过渡）保留
- **运行时多主题切换**：只保留 token 结构与 tweakcn 主题包格式兼容，不做切换 UI（见 D2）
- **不从 credit-master 搬**：Aurora 极光背景、3D loading 动画、cmdk 命令面板、CountingNumber 数字滚动、头像装饰系统
- **设置页**：rail 底部设置图标仅占位
- 后端任何改动、移动端专项优化（仅保证基本可用：面板收成 Sheet 抽屉）

## 2. 决策记录

| # | 决策 | 理由 |
|---|---|---|
| D1 | **全面升级**（token + 布局 + 聊天 + 组件状态一次到位） | 分散刷视觉会反复返工；3.5 独立立项就是为了一次性对齐 |
| D2 | **单主题 + 明暗切换，不做运行时多主题** | 单用户 MVP 够用；token 结构对齐 tweakcn 格式，未来接多主题只加不改 |
| D3 | **品牌色：紫罗兰 `oklch(0.585 0.233 277)`**（credit-master 同款） | 与视觉参考完全同源，可最大程度复用其样式；现代 SaaS 气质 |
| D4 | **布局：图标 rail + 二级面板**（Slack/Linear 式），不用单一侧栏 | 为 MCP/skills 等未来功能区预留结构——每加功能区只加一个图标+一个面板，布局框架不动；布局是改起来最疼的层，值得预留 |
| D5 | **消息区：文档流式**（用户气泡 + assistant 通栏排版） | 知识库回答以长文+代码为主，"答案是正文"的排版匹配产品定位；prose 排版与 macOS 风格代码块直接可用 |
| D6 | **工具轨迹：竖向时间线**，回答生成后自动折叠为摘要行 | Agent 模式的差异化就是"看得到它怎么想"；时间线让阶段 3 持久化的 thought 有了舞台；自动折叠避免过程永久占屏 |
| D7 | **底座留在 Base UI（shadcn base-nova），不切 Radix；增强动效后移** | Base UI 是 shadcn 官方新方向，切回 Radix 是向后迁移且用户不可见；视觉彻底度由 token+样式决定，与底座无关。动效后移使 3.5 无需引入 motion，后加时只在组件外包 `motion.div`，不改结构 |
| D8 | **`/` 首页改为重定向** | 有 KB → 聊天区；无 KB → 知识库区引导创建。产品中心是对话，不是 KB 卡片列表 |

## 3. 设计 token（`app/globals.css` 重写）

### 3.1 色彩

整体替换为紫罗兰 OKLCH 主题，亮/暗两套完整变量。核心值：

```css
:root {
  --radius: 0.625rem;                            /* 维持不变 */
  --background: oklch(1 0 0);
  --foreground: oklch(0.141 0.005 285.823);
  --primary: oklch(0.585 0.233 277.117);         /* 紫罗兰主色 */
  --primary-foreground: oklch(0.985 0 0);
  --muted: oklch(0.967 0.001 286.375);
  --muted-foreground: oklch(0.552 0.016 285.938);
  --destructive: oklch(0.577 0.245 27.325);
  --border: oklch(0.92 0.004 286.32);
  /* --card/--popover/--secondary/--accent/--input/--ring 以及
     --sidebar-* 系列、--chart-1..5 一并对齐 credit-master 亮色值 */
}
.dark {
  --background: oklch(0.141 0.005 285.823);
  --card: oklch(0.21 0.006 285.885);
  --border: oklch(1 0 0 / 10%);                  /* 暗色边框用半透明白 */
  /* 其余同步对齐 credit-master 暗色值 */
}
```

补齐 credit-master 主题包里有而当前 globals 没有的阶梯（结构对齐 tweakcn 格式，为 D2 留接口）：

- `--shadow-2xs` … `--shadow-2xl` 阴影阶梯（含可配置 `--shadow-color/-opacity`）
- `--tracking-tighter` … `--tracking-widest` 字距 token

### 3.2 字体

- `--font-sans`: **Geist**（拉丁）+ **Noto Sans SC**（中文回退），均走 `next/font/google`
- `--font-mono`: **Geist Mono**（代码块、JSON 展示）
- 不引入 serif 标题字体；`--font-heading` 保持等于 `--font-sans`

### 3.3 暗色模式接入

- `providers.tsx` 加 next-themes `ThemeProvider`（`attribute="class"`、`defaultTheme="system"`、`enableSystem`）
- `layout.tsx` 的 `<html>` 加 `suppressHydrationWarning`
- rail 底部放明暗切换按钮（Sun/Moon 图标），localStorage 持久化（next-themes 自带）
- **清除全部硬编码调色板类**：`ingest-status-badge.tsx` 的 `bg-blue-100 text-blue-800` 等改为语义 token / Badge variant，保证暗色不穿帮

## 4. 全局布局（`AppShell`）

### 4.1 结构

三列，基于 shadcn Sidebar 体系组装：

```
┌──┬─────────┬──────────────────────────┐
│  │ 二级面板 │ 主内容区（SidebarInset）    │
│r │ w-64    │                          │
│a │ 可收起   │  各功能区自带 header       │
│i │         │                          │
│l │         │                          │
└──┴─────────┴──────────────────────────┘
```

**① 图标 rail**（约 `w-14`，深色背景，`collapsible="none"` 的窄 Sidebar）：

- 上部功能区：**聊天**（MessageSquare）、**知识库**（Library）——本阶段仅 2 个
- 底部：明暗切换、设置图标（占位，无页面）
- 选中态：紫罗兰底色圆角块

**② 二级面板**（`w-64`，可收起），随功能区切换：

- 聊天区：KB 切换下拉（DropdownMenu）→「新会话」按钮 → 当前 KB 会话列表
- 知识库区：KB 列表 +「新建知识库」按钮

**③ 主内容区**：各功能区自带顶部 header，**不设全局顶栏**：

- 聊天：会话标题 + RAG/Agent 模式切换（shadcn Tabs）
- 知识库：KB 名 + 上传按钮 + 文档表格；文档详情（chunks）仍是子路由

### 4.2 路由映射

5 条现有路由 URL 全部不变，收编进 AppShell：

| 路由 | 功能区 | 变化 |
|---|---|---|
| `/` | — | 改为重定向（D8）：有 KB → `/kbs/{id}/chats`，`{id}` 取 localStorage 记录的最近访问 KB（无记录则取列表第一个）；无 KB → 知识库区引导创建 |
| `/kbs/{kbId}/chats` `/chats/{convId}` | 聊天 | 页内 ChatSidebar 移除，会话列表上移到二级面板 |
| `/kbs/{kbId}/docs` | 知识库 | KB 列表上移到二级面板，主区专注文档管理 |
| `/kbs/{kbId}/docs/{docId}` | 知识库 | 收编进布局，结构保留 |

### 4.3 响应式

移动端（`< md`）：rail 保留；二级面板收成 Sheet 抽屉，主区 header 加汉堡键唤出。

## 5. 聊天界面重设计

### 5.1 消息区（文档流式，D5）

- **用户消息**：右侧紫罗兰圆角气泡（右下角小切角 `rounded-br-sm`）
- **assistant 回答**：通栏排版，左侧品牌方块标识（非圆形头像）；Markdown 管线升级为 `react-markdown + remark-gfm + rehype-highlight + @tailwindcss/typography`（`prose` 类，替换现在手写的 `[&_code]` 内联选择器），支持 GFM 表格
- **代码块**：macOS 窗口风格组件（三圆点 + 语言标签 + 悬浮复制按钮），从 credit-master `code-block.tsx` 拷贝，去除其 Radix 依赖点后接入 Markdown 渲染管线
- **流式反馈**：等待首 token 显示闪烁打字光标（纯 CSS blink）替代 "Thinking..." 文字；流式中光标跟在文末；错误消息用 Alert 样式

### 5.2 工具轨迹：`agent-timeline`（替代 `tool-call-trace`，D6）

- 左侧竖向时间线串起节点，两类节点：
  - **thought 节点**：引文体斜体展示（`text-muted-foreground italic`），时间线圆点弱化
  - **工具调用节点**：可展开卡片，收起显示 `✓ kb_retrieval("…") → 4 chunks · 0.3s`，展开显示 arguments / result 格式化 JSON（`font-mono` + 语法高亮可选）
- 状态视觉：运行中 = 紫色 spinner 节点；成功 = 绿勾 + 耗时 + 结果摘要；失败 = 红色节点 + 错误信息
- **自动折叠**：最终回答开始流入后，时间线整体折叠为一行摘要（`调用了 2 个工具 ▸`），点击可重新展开；历史消息默认折叠态
- 数据源不变：流式来自 `tool_call`/`tool_result` SSE 事件，回看来自 `messages.tool_calls` JSONB

### 5.3 引用体验

- 回答底部「来源」分隔栏 + 编号 chip（`[1] 文档名 · 0.92`），替代横向滚动卡片
- 点击 chip → 右侧抽屉改用 **Sheet 组件**（替换现在 Dialog 硬改的实现），展示相邻 chunks，主 chunk 紫罗兰高亮边框

### 5.4 输入区与其他

- ChatInput 换 shadcn **Textarea**，自动长高（上限约 6 行），Enter 发送 / Shift+Enter 换行不变；流式中显示停止按钮
- 模式切换：手写分段控件换 shadcn **Tabs**（RAG / Agent）
- 会话列表项（二级面板内）：hover 显操作、选中态紫罗兰左边条
- 空态：品牌图标 + 中文引导（如"向这个知识库提问吧"）
- 错误提示统一 **Alert** 组件，header 下内联；文案全部中文

## 6. 知识库区页面

- **文档管理主区**：上传区升级为虚线拖拽卡片（拖入高亮 `border-primary`）；表格精修（表头 `bg-muted/50`、边框 `border-border/50`、操作图标加 Tooltip）
- **状态 badge**：`ingest-status-badge` 改语义色（processing = primary 系 + spinner、completed = 绿、failed = destructive），适配暗色
- **删除确认**：原生 `confirm()` 全部替换为 **AlertDialog**（KB 删除、文档删除）
- **文档详情**：结构保留；卡片统一 `bg-card rounded-lg shadow-sm`；处理中加 **Skeleton** 骨架屏
- **空态**：无 KB → 居中图标 +「创建第一个知识库」按钮；无文档 → 图标 + 引导上传文案（通用 `empty-state` 组件）

## 7. 组件清单与依赖

### 7.1 新增 npm 依赖（2 个）

| 包 | 用途 |
|---|---|
| `@tailwindcss/typography` | assistant 回答 prose 排版 |
| `remark-gfm` | Markdown 表格/删除线支持 |

`next-themes` 已安装，接上即可。不引入 `motion`（D7）。

### 7.2 shadcn CLI 补装（Base UI / base-nova 体系，约 12 个）

`sidebar` `sheet` `textarea` `card` `tabs` `skeleton` `tooltip` `alert` `alert-dialog` `dropdown-menu` `separator` `scroll-area`

按 CLAUDE.md §5.3 流程：`pnpm dlx shadcn@latest add <component>` 后可改样式。若个别组件 base-nova registry 未提供，以官方 registry 对应组件为蓝本手工改写为 Base UI 版放入 `components/ui/`，并在 PR 里注明。

### 7.3 从 credit-master 拷贝改造

| 组件 | 说明 |
|---|---|
| `code-block` | macOS 窗口风格代码块，纯样式组件，去除 Radix 依赖点后使用 |

### 7.4 自研新组件

| 目录 | 组件 |
|---|---|
| `components/layout/` | `app-shell`（组装）、`rail`、`chat-panel`、`kb-panel` |
| `components/chat/` | `agent-timeline`（替代 tool-call-trace）、`streaming-cursor` |
| `components/common/` | `empty-state`（图标 + 文案 + 可选按钮） |

## 8. 动效规范

**原则：3.5 只做功能性反馈，不做装饰性动画（D7）。**

保留（不依赖 motion）：

- 打字光标 CSS blink
- 运行中 spinner（`animate-spin`）
- hover 一律 `transition-colors`
- shadcn Sidebar / Sheet 自带展开收起过渡
- 尊重 `prefers-reduced-motion`（CSS media query）

后移（3.5 之后可选迭代，届时引入 motion，组件外包 `motion.div` 即可，无结构改动）：路由切换淡入、时间线节点入场、AnimatePresence 展开收起。

## 9. 测试与验收

### 9.1 测试

- 现有 vitest 迁移：`tool-call-trace` 测试 → `agent-timeline`；`message-bubble`、`chat-header`（Tabs 化）等随新结构更新
- 新增：agent-timeline（thought / 步骤状态 / 自动折叠 / 错误态渲染）、code-block 复制、主题切换 class 生效、empty-state
- e2e 层面不新增（后端零改动，SSE 流程已有 e2e 覆盖）

### 9.2 验收标准

1. `pnpm build && pnpm lint && pnpm typecheck && pnpm vitest run` 全绿
2. **每个页面亮/暗两种模式截图对照验收**（聊天空态/流式中/带轨迹与引用、文档管理、文档详情、空 KB 引导）
3. 明暗切换即时生效且刷新后保持；暗色下无硬编码色穿帮
4. Agent 模式发问：时间线实时推进 → 回答开始后自动折叠 → 点击可展开 → 刷新后历史轨迹可回看
5. 全站文案中文，无中英混排
6. 移动端宽度下二级面板收成抽屉，聊天可正常收发

## 10. 对主 spec / CLAUDE.md 的同步

实施完成后：

- CLAUDE.md §2 当前阶段推进到"阶段 3.5 已完成"，下一阶段按主 spec §7 排（阶段 4）
- CLAUDE.md §5.4（agent tool 前端渲染说明）中 `tool-call-trace` 名称更新为 `agent-timeline`
- 主 spec §6.1 前端目录结构补充 `components/layout/`、`components/common/`
