"use client";

import { useState } from "react";
import Link from "next/link";
import { ArrowDown, ArrowRight, BookOpen, Boxes, Check, Code2, FileText, Layers3, Pause, Play, Search, ShieldCheck, Sparkles, Workflow } from "lucide-react";
import { useAuth } from "@/lib/hooks/use-auth";
import { getAuthenticatedUser } from "@/lib/auth-state";
import { useSessionExpired } from "@/components/auth/auth-session-boundary";
import styles from "./landing.module.css";

const scenes = [
  { name: "发现能力", icon: Boxes, label: "CAPABILITY MARKET", title: "让好用的能力，被更多人发现。", description: "集中发现团队共享的 MCP 与 Skill，了解使用方式，把已有成果带进下一次工作。", input: "团队工具与技能", action: "发现 · 了解 · 复用", output: "你的工作流", tags: ["MCP 服务", "Skill 技能", "使用说明"], href: "/hub", link: "进入能力市场" },
  { name: "连接知识", icon: BookOpen, label: "KNOWLEDGE SERVICE", title: "让沉淀的经验，成为可检索的知识。", description: "上传和管理知识文档，检索相关内容，并通过授权接口为业务应用提供知识支持。", input: "文档与团队经验", action: "解析 · 索引 · 检索", output: "你的业务应用", tags: ["知识库管理", "检索测试", "应用授权"], href: "/hub/knowledge", link: "进入知识服务" },
  { name: "探索模型", icon: Sparkles, label: "MODEL PLAYGROUND", title: "让新的想法，从一次尝试开始。", description: "配置自己的模型连接，在游乐场里调整参数、尝试提示词，探索适合任务的使用方式。", input: "问题与提示词", action: "连接 · 调整 · 体验", output: "下一步灵感", tags: ["模型连接", "参数调整", "对话体验"], href: "/hub/playground", link: "进入模型游乐场" },
];

export function LandingPage() {
  const auth = useAuth();
  const expired = useSessionExpired();
  const authenticated = Boolean(getAuthenticatedUser(auth, expired));
  const [selected, setSelected] = useState(0);
  const [paused, setPaused] = useState(false);
  const scene = scenes[selected];
  const SceneIcon = scene.icon;
  const entry = authenticated ? "/hub" : "/login";
  const entryLabel = authenticated ? "进入工作台" : "飞书登录";

  return (
    <div className={`${styles.page} ${paused ? styles.paused : ""}`}>
      <a href="#main-content" className={styles.skip}>跳到主要内容</a>
      <header className={styles.header}>
        <Link href="/" className={styles.brand} aria-label="Capability Hub 首页"><span className={styles.brandIcon}><Boxes size={23} /></span><span>Capability<span className={styles.brandLight}> Hub</span></span></Link>
        <nav aria-label="首页导航" className={styles.nav}><a href="#explore">平台能力</a><a href="#start">如何开始</a><Link href="/guide">操作说明</Link></nav>
        <Link href={entry} className={styles.headerEntry}>{entryLabel}<ArrowRight size={15} /></Link>
      </header>

      <main id="main-content">
        <section className={styles.hero} aria-labelledby="hero-title">
          <div className={styles.heroCopy}>
            <div className={styles.eyebrow}><span /> BUILT FOR YOUR TEAM <span className={styles.beta}>内部试运行</span></div>
            <h1 id="hero-title">让团队的能力，<br /><span>连接更多可能。</span></h1>
            <p className={styles.intro}>一个入口，发现共享工具，连接团队知识。<br className={styles.desktopBreak} />把积累变成下一次工作的起点。</p>
            <div className={styles.heroActions}><Link href={entry} className={styles.primary}>{entryLabel}<ArrowRight size={18} /></Link><a href="#explore" className={styles.secondary}>探索平台<ArrowDown size={17} /></a></div>
            <p className={styles.accessNote}><ShieldCheck size={15} />{authenticated ? "已登录，按你的权限访问团队内容" : "使用公司飞书账号 · 内部内容登录后可见"}</p>
          </div>

          <div className={styles.visual} aria-label="能力、知识和应用的连接示意">
            <div className={styles.visualGrid} />
            <div className={styles.orbit} /><div className={styles.orbitInner} />
            <svg className={styles.connections} viewBox="0 0 540 420" fill="none" aria-hidden="true"><path d="M108 105 C108 210 270 105 270 210 S430 210 430 318 M426 100 C426 200 270 120 270 210 S108 220 108 326" /><path className={styles.flow} d="M108 105 C108 210 270 105 270 210 S430 210 430 318 M426 100 C426 200 270 120 270 210 S108 220 108 326" /></svg>
            <div className={`${styles.node} ${styles.nodeOne}`}><span className={styles.blueIcon}><Code2 size={21} /></span><div><strong>MCP 服务</strong><small>连接团队工具</small></div></div>
            <div className={`${styles.node} ${styles.nodeTwo}`}><span className={styles.violetIcon}><Sparkles size={21} /></span><div><strong>Skill 技能</strong><small>复用实践方法</small></div></div>
            <div className={`${styles.node} ${styles.nodeThree}`}><span className={styles.greenIcon}><BookOpen size={21} /></span><div><strong>团队知识</strong><small>沉淀经验与资料</small></div></div>
            <div className={`${styles.node} ${styles.nodeFour}`}><span className={styles.amberIcon}><Workflow size={21} /></span><div><strong>业务应用</strong><small>在工作中接入</small></div></div>
            <div className={styles.hubCore}><Boxes size={37} strokeWidth={1.4} /><strong>Capability Hub</strong><span>团队能力，由此连接</span></div>
            <div className={styles.visualCaption}><span>CONNECTIONS, NOT SILOS.</span><button type="button" onClick={() => setPaused(!paused)} aria-label={paused ? "播放装饰动画" : "暂停装饰动画"}>{paused ? <Play size={13} /> : <Pause size={13} />}</button></div>
          </div>
        </section>

        <div className={styles.principles}><span><Layers3 size={16} />能力集中发现</span><span><BookOpen size={16} />知识持续沉淀</span><span><ShieldCheck size={16} />内容按权限访问</span><span><Workflow size={16} />面向业务接入</span></div>

        <section id="explore" className={styles.explore} aria-labelledby="explore-title">
          <div className={styles.sectionHeading}><div><p className={styles.kicker}>ONE HUB. MORE POSSIBILITIES.</p><h2 id="explore-title">从你想做的事开始。</h2></div><p>把工具、知识与灵感，放在触手可及的地方。</p></div>
          <div className={styles.selector} role="group" aria-label="选择平台能力">{scenes.map((item, index) => <button type="button" key={item.name} aria-pressed={selected === index} className={selected === index ? styles.selected : ""} onClick={() => setSelected(index)}><item.icon size={18} />{item.name}<span>0{index + 1}</span></button>)}</div>
          <div className={styles.scene} key={selected}>
            <div className={styles.sceneCopy}><p className={styles.kicker}>{scene.label}</p><h3>{scene.title}</h3><p>{scene.description}</p><div className={styles.tags}>{scene.tags.map(tag => <span key={tag}><Check size={12} />{tag}</span>)}</div><Link href={authenticated ? scene.href : "/login"} className={styles.textLink}>{authenticated ? scene.link : `登录后${scene.link}`}<ArrowRight size={16} /></Link></div>
            <div className={styles.preview} aria-label={`${scene.name}流程示意`}><div className={styles.previewTop}><span><span className={styles.smallDot} />工作流预览</span><span>示意</span></div><div className={styles.source}><FileText size={17} />{scene.input}<span>INPUT</span></div><div className={styles.verticalLine} /><div className={styles.process}><SceneIcon size={22} /><strong>{scene.action}</strong><span>Capability Hub</span></div><div className={styles.verticalLine} /><div className={styles.result}><span><Check size={15} /></span>{scene.output}<ArrowRight size={15} /></div></div>
          </div>
        </section>

        <section id="start" className={styles.start} aria-labelledby="start-title"><div className={styles.sectionHeading}><div><p className={styles.kicker}>YOUR NEXT STEP</p><h2 id="start-title">从这里，开始协作。</h2></div><Link href={entry} className={styles.textLink}>{entryLabel}<ArrowRight size={16} /></Link></div><div className={styles.steps}>{[{title:"登录团队账号",text:"通过飞书进入平台，访问权限范围内的内容。",icon:ShieldCheck},{title:"找到所需能力",text:"浏览能力市场，或创建知识库、探索模型。",icon:Search},{title:"带入实际工作",text:"按说明使用共享能力，让应用接入授权知识。",icon:Workflow}].map((step,index)=><div className={styles.step} key={step.title}><div><span>0{index+1}</span><step.icon size={20}/></div><h3>{step.title}</h3><p>{step.text}</p></div>)}</div></section>
      </main>
      <footer className={styles.footer}><Link href="/" className={styles.footerBrand}><Boxes size={18} />Capability Hub</Link><p>团队能力，共同创造。</p><Link href="/guide">操作说明</Link><span>内部协作平台 · 试运行</span></footer>
    </div>
  );
}
