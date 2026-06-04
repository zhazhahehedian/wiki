"use client";

import { FormEvent, KeyboardEvent, useState } from "react";
import { Send, Square } from "lucide-react";

import { Button } from "@/components/ui/button";

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

  function submit() {
    const trimmed = value.trim();
    if (!trimmed || disabled) return;
    onSend(trimmed);
    setValue("");
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    submit();
  }

  function onKeyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      submit();
    }
  }

  return (
    <form onSubmit={onSubmit} className="border-t bg-background p-3">
      <div className="flex items-end gap-2">
        <textarea
          value={value}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={onKeyDown}
          rows={2}
          disabled={disabled}
          placeholder="Ask a question about this KB"
          className="min-h-16 flex-1 resize-none rounded-md border bg-background px-3 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:opacity-50"
        />
        <div className="flex shrink-0 gap-2">
          <Button type="submit" size="icon" disabled={disabled || value.trim() === ""}>
            <Send className="size-4" />
          </Button>
          {disabled && (
            <Button type="button" size="icon" variant="outline" onClick={onStop}>
              <Square className="size-4" />
            </Button>
          )}
        </div>
      </div>
    </form>
  );
}
