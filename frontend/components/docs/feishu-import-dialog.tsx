"use client";

import { useState, type FormEvent } from "react";
import { CloudDownload } from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useFeishuImport } from "@/lib/hooks/use-feishu-import";

function validateFeishuURL(value: string): string | null {
  const parts = value.trim().split(/\s+/).filter(Boolean);
  if (parts.length > 1) return "每次只能导入一个链接。";
  if (parts.length === 0) return "请输入飞书文档链接。";

  try {
    const url = new URL(parts[0]);
    const validHost = url.hostname.endsWith(".feishu.cn") || url.hostname.endsWith(".larksuite.com");
    if (url.protocol !== "https:" || !validHost) return "请输入有效的飞书文档链接。";
  } catch {
    return "请输入有效的飞书文档链接。";
  }
  return null;
}

export function FeishuImportDialog({ kbId }: { kbId: string }) {
  const [open, setOpen] = useState(false);
  const [url, setURL] = useState("");
  const [validationError, setValidationError] = useState<string | null>(null);
  const imported = useFeishuImport(kbId);

  function onOpenChange(nextOpen: boolean) {
    setOpen(nextOpen);
    if (nextOpen) {
      setValidationError(null);
      if (!imported.isPending) imported.reset();
    }
  }

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const message = validateFeishuURL(url);
    setValidationError(message);
    if (message) return;

    const trimmedURL = url.trim();
    imported.mutate(trimmedURL, {
      onSuccess: () => {
        toast.success("飞书文档已加入导入队列");
        setURL("");
        setOpen(false);
      },
    });
  }

  const error = validationError ?? (imported.isError ? (imported.error as Error).message : null);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogTrigger render={<Button variant="outline" />}>
        <CloudDownload data-icon="inline-start" />
        从飞书导入
      </DialogTrigger>
      <DialogContent className="rounded-lg sm:max-w-md">
        <form className="grid gap-4" onSubmit={onSubmit} noValidate>
          <DialogHeader>
            <DialogTitle>从飞书导入</DialogTitle>
            <DialogDescription>粘贴一个你有权限访问的飞书文档链接。</DialogDescription>
          </DialogHeader>

          <div className="grid gap-2">
            <Label htmlFor="feishu-url">飞书文档链接</Label>
            <Input
              id="feishu-url"
              type="url"
              autoComplete="off"
              placeholder="https://team.feishu.cn/docx/..."
              value={url}
              disabled={imported.isPending}
              aria-invalid={Boolean(error)}
              aria-describedby={error ? "feishu-url-error" : undefined}
              onChange={(event) => {
                setURL(event.target.value);
                setValidationError(null);
                if (!imported.isPending) imported.reset();
              }}
              autoFocus
            />
          </div>

          {error && (
            <Alert id="feishu-url-error" variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="submit" disabled={imported.isPending}>
              {imported.isPending ? "导入中…" : "开始导入"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
