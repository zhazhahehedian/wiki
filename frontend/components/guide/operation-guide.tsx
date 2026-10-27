import Link from "next/link";
import { ArrowRight, BookOpen, CircleHelp, FlaskConical, Library, Send, ShieldCheck } from "lucide-react";

const chapters = [
  { id: "first-login", title: "首次登录", label: "先确认账号，再开始使用", href: "/hub/settings", action: "查看账号设置", steps: [
    "打开平台，点击「飞书登录」，使用公司飞书账号完成授权。登录成功后会进入仪表盘。",
    "进入「设置」，核对姓名、邮箱与平台角色。普通用户可以使用已获授权的能力；管理员额外拥有审核等治理入口。",
    "需要部门范围的能力或知识库时，请联系平台管理员核定可信部门。飞书应用管理员身份不会自动变成平台管理员。",
  ], note: "首次使用不必先配置模型密钥。浏览能力市场与使用已授权的知识检索，不依赖你的个人模型接口。" },
  { id: "market", title: "查找与使用团队能力", label: "适合：希望直接复用已有工具或技能", href: "/hub/registry", action: "打开能力市场", steps: [
    "进入「能力市场」，查找需要的 MCP 服务或 Skill，打开详情查看用途、当前版本和使用说明。市场只显示你有权访问的内容。",
    "使用 Skill：点击「下载 Skill 包」，解压并保留全部附件；阅读 SKILL.md，按所用 AI 工具的说明导入，再在该工具中调用。",
    "接入 MCP：查看详情中的 Endpoint、传输协议、鉴权方式和工具清单，按客户端的 MCP 配置说明接入。需要凭证时联系服务提供者。",
    "希望调整已有 Skill 时，可选择「基于此创建」，修改后保存为自己的草稿，再按发布流程提交。",
  ], note: "平台负责发现、说明和分发；Skill 与 MCP 的实际执行发生在接入它们的工具或应用中。" },
  { id: "knowledge", title: "上传资料并检索知识", label: "适合：沉淀制度、流程、项目说明等文本资料", href: "/hub/knowledge", action: "打开知识服务", steps: [
    "进入「知识服务」，创建知识库，填写名称并按实际需要设置访问范围。已有知识库需拥有管理权限才能上传或修改文档。",
    "打开知识库，使用「上传文档」选择文件。支持 TXT、Markdown、DOCX、文字型 PDF，单文件最大 20 MiB。",
    "等待解析和索引完成，再打开预览检查提取正文和诊断。若处理失败，依据提示调整原文件；扫描件需要先转换为可读取的文字文档。",
    "进入「检索测试」，选择有权使用的知识库，输入一个具体问题，查看命中的原文片段及出处。可以用资料中的明确问题检验效果。",
    "资料变化后在文档列表更新版本；新版成功后切换，处理失败时原来已发布的版本仍可检索。",
  ], note: "知识检索返回相关原文和出处，不直接生成完整问答。图片 OCR 和飞书云文档自动同步目前尚未开放。" },
  { id: "playground", title: "配置模型并体验对话", label: "适合：尝试提示词、模型和参数", href: "/hub/playground", action: "打开模型游乐场", steps: [
    "在「设置」中点击「配置模型接口」，选择 OpenAI 兼容或 Claude（Anthropic）协议。",
    "填写服务商提供的接口地址和个人 API Key，选择或填写可用模型，设置默认模型后保存。接口地址格式以配置页提示为准。",
    "打开「游乐场」，选择模型并输入问题。需要时再调整温度、输出长度等参数，参数是否可用取决于模型服务商。",
    "需要留存的回答请及时自行保存。平台不将对话内容持久化保存到服务器，刷新或离开前请确认重要内容已留存。",
  ], note: "模型配置保存在你的账号下，API Key 加密保存。个人游乐场连接与平台知识服务的模型配置分别管理；调用可能产生服务商费用。" },
  { id: "publish", title: "发布自己的能力", label: "适合：希望把成果分享给团队的发布者", href: "/hub/registry/new", action: "创建能力草稿", steps: [
    "从仪表盘点击「创建能力」，填写名称、唯一标识、部门、能力描述和能力类型。",
    "MCP 填写服务地址、协议、鉴权方式和工具清单；Skill 上传 SKILL.md 与完整附件，按页面提示检查文件头及引用。",
    "填写版本号、版本说明和上线后的可见范围，先「保存草稿」，再从详情页提交审核。保存草稿不会直接向同事公开。",
    "管理员审核通过后，授权范围内的同事才能在市场发现该版本。被驳回时，根据审核意见修改后重新提交。",
    "通过「我的发布」维护后续版本。新版审核期间，原有已上线版本继续可用。",
  ], note: "发布说明应写清用途、输入要求、使用步骤和必要依赖；不要把 API Key、访问令牌等凭证写进介绍或打包到附件。" },
  { id: "application", title: "让业务应用接入知识", label: "进阶：面向业务应用开发人员", href: "/hub/knowledge/applications", action: "管理知识应用", steps: [
    "在知识服务的应用管理中创建应用，选择它需要访问的知识库，并配置相应授权。",
    "签发应用凭证并立即妥善保存，凭证只在签发时显示一次。由业务应用后端保管和使用凭证。",
    "通过知识检索接口获取授权范围内的片段与出处，再由业务系统组织对话或生成回答。先用少量明确问题验证效果与权限边界。",
    "凭证需要更换时执行轮换，旧凭证会立即失效；应用停用时撤销凭证。更新调用方配置后再验证一次检索。",
  ], note: "应用知识授权独立配置，不能通过传入某个人的身份来获得其权限。不要把应用凭证放在网页前端或公开文档中。" },
];

const questions = [
  ["登录失败，或提示应用不可用怎么办？", "确认使用公司账号，并联系应用管理员核对飞书应用发布状态和可用人员范围。若仍失败，提供发生时间、页面错误提示及操作步骤；不要发送密钥、授权码或包含授权参数的完整回调链接。"],
  ["为什么看不到某项能力或知识库？", "可能尚未审核上线，或你不在它的可见与授权范围内。先向发布者确认名称和上线状态，再请平台管理员核对账号、可信部门及授权。"],
  ["为什么我没有审核入口？", "审核队列、审计日志和部门授权面向平台管理员。普通用户看不到这些入口属于正常行为；公司飞书应用管理员也不会自动获得平台管理员角色。"],
  ["上传成功，为什么还不能检索？", "上传后需要异步解析和索引。检查文档状态及解析诊断；成功后再测试。当前不支持扫描 PDF 的 OCR，复杂版式也可能需要整理成更清晰的文本。"],
  ["知识检索没有结果怎么办？", "先确认选中了正确知识库且文档已就绪，再用文档中确实包含的事实提问。问题尽量明确；检索结果是相关片段，不代表完整答案，使用前请核对出处和适用条件。"],
  ["模型接口报错或模型列表为空怎么办？", "核对协议、接口地址、Key、模型名称，以及服务商的额度和模型访问权限。更换地址或协议时重新填写 Key；不要把其他服务商的密钥混用。"],
];

export function OperationGuide() {
  return (
    <div className="mx-auto max-w-6xl text-zinc-900">
      <header className="border-b border-zinc-200 pb-8">
        <p className="mb-3 text-xs font-medium tracking-[0.16em] text-blue-700">CAPABILITY HUB · 使用指南</p>
        <div className="flex flex-wrap items-start justify-between gap-5">
          <div><h1 className="text-3xl font-semibold tracking-tight">操作说明</h1><p className="mt-3 max-w-2xl text-sm leading-7 text-zinc-500">第一次使用，从登录和查找能力开始。需要沉淀资料、体验模型或分享成果时，再按对应章节操作。</p></div>
          <Link href="/hub" className="inline-flex items-center gap-2 rounded-lg bg-blue-600 px-4 py-2.5 text-sm font-medium text-white hover:bg-blue-700">进入工作台<ArrowRight className="size-4" /></Link>
        </div>
        <div className="mt-5 flex flex-wrap gap-x-5 gap-y-2 text-xs text-zinc-500"><span>面向：团队成员与能力发布者</span><span>更新：2026-10-27</span><Link className="text-blue-700 underline underline-offset-4" href="/guide">打开可分享的说明页</Link></div>
      </header>

      <section aria-label="按目标快速开始" className="grid gap-3 py-7 sm:grid-cols-2 xl:grid-cols-4">
        {[
          { title: "使用现成能力", text: "找到工具与 Skill，按说明复用", href: "#market", icon: Library },
          { title: "查找团队知识", text: "上传资料，检索原文与出处", href: "#knowledge", icon: BookOpen },
          { title: "体验 AI 模型", text: "配置自己的接口，尝试对话", href: "#playground", icon: FlaskConical },
          { title: "分享我的成果", text: "创建草稿，提交审核并发布", href: "#publish", icon: Send },
        ].map(({ title, text, href, icon: Icon }) => <a key={href} href={href} className="rounded-xl border border-zinc-200 bg-white p-5 transition-colors hover:border-blue-300 focus-visible:outline-2 focus-visible:outline-blue-600"><Icon className="mb-4 size-5 text-blue-600" aria-hidden="true" /><h2 className="text-sm font-semibold">{title}</h2><p className="mt-2 text-xs leading-6 text-zinc-500">{text}</p></a>)}
      </section>

      <div className="grid items-start gap-8 lg:grid-cols-[185px_minmax(0,1fr)]">
        <nav aria-label="操作说明目录" className="rounded-xl border border-zinc-200 bg-white p-4 lg:sticky lg:top-6"><p className="mb-3 text-xs font-semibold text-zinc-400">本页目录</p><ol className="space-y-1">{chapters.map((chapter, index) => <li key={chapter.id}><a href={`#${chapter.id}`} className="flex gap-2 rounded-md px-2 py-2 text-xs leading-5 text-zinc-600 hover:bg-blue-50 hover:text-blue-700"><span className="text-zinc-400">0{index + 1}</span>{chapter.title}</a></li>)}<li><a href="#faq" className="flex items-center gap-2 rounded-md px-2 py-2 text-xs text-zinc-600 hover:bg-blue-50"><CircleHelp className="size-3.5" />常见问题</a></li></ol></nav>
        <div className="min-w-0 space-y-6">
          {chapters.map((chapter, index) => <section id={chapter.id} key={chapter.id} aria-labelledby={`${chapter.id}-title`} className="scroll-mt-6 rounded-xl border border-zinc-200 bg-white p-5 sm:p-7"><p className="mb-2 text-xs text-zinc-500">{chapter.label}</p><h2 id={`${chapter.id}-title`} className="flex gap-3 text-lg font-semibold"><span className="text-blue-600">0{index + 1}</span>{chapter.title}</h2><ol className="my-6 space-y-4">{chapter.steps.map((step, stepIndex) => <li key={step} className="flex gap-3 text-sm leading-7 text-zinc-600"><span className="mt-1 flex size-5 shrink-0 items-center justify-center rounded-full bg-zinc-100 text-[10px] font-medium text-zinc-500">{stepIndex + 1}</span><span>{step}</span></li>)}</ol><div className="mb-5 flex gap-2 rounded-lg bg-blue-50/70 p-4 text-xs leading-6 text-slate-600"><ShieldCheck className="mt-1 size-4 shrink-0 text-blue-600" aria-hidden="true" /><p>{chapter.note}</p></div><Link href={chapter.href} className="inline-flex items-center gap-2 text-sm font-medium text-blue-700 hover:underline">{chapter.action}<ArrowRight className="size-4" /></Link></section>)}
          <section id="faq" aria-labelledby="faq-title" className="scroll-mt-6 rounded-xl border border-zinc-200 bg-white p-5 sm:p-7"><h2 id="faq-title" className="text-lg font-semibold">常见问题</h2><div className="mt-4 divide-y divide-zinc-100">{questions.map(([question, answer]) => <details key={question} className="group py-4"><summary className="cursor-pointer text-sm font-medium leading-6 text-zinc-700 marker:text-blue-600">{question}</summary><p className="mt-3 pl-4 text-sm leading-7 text-zinc-500">{answer}</p></details>)}</div><p className="mt-4 border-t border-zinc-100 pt-5 text-xs leading-6 text-zinc-500">仍需协助时，请联系平台维护人，附上所在页面、操作步骤、发生时间及错误提示。涉及公司资料时，按内部要求处理截图和内容。</p></section>
          <p className="pb-6 text-center text-xs text-zinc-400">本文仅包含通用操作指导，内部能力、知识内容和个人配置仍需登录并按权限访问。</p>
        </div>
      </div>
    </div>
  );
}
