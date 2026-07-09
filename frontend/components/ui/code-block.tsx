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

  const onCopy = () => {
    navigator.clipboard.writeText(code);
    setHasCopied(true);
    setTimeout(() => setHasCopied(false), 2000);
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
          aria-label="复制代码"
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
