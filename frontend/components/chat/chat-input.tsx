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
