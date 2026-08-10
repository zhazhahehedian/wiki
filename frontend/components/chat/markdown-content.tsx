"use client";

import { Children, isValidElement, memo, type ReactElement, type ReactNode } from "react";
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

// memo：props 仅一个字符串，流式渲染时避免已完成的兄弟消息随每个 token 重新解析 markdown
export const MarkdownContent = memo(function MarkdownContent({ content }: { content: string }) {
  return (
    <div className="prose prose-sm max-w-none dark:prose-invert prose-pre:m-0 prose-pre:bg-transparent prose-pre:p-0">
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]} components={{ pre: Pre }}>
        {content}
      </ReactMarkdown>
    </div>
  );
});
