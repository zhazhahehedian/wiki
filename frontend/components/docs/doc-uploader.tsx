"use client";

import { useRef, useState } from "react";
import { UploadCloud } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useUploadDoc } from "@/lib/hooks/use-docs";
import { toast } from "sonner";
import { APIError } from "@/lib/api/client";

const ACCEPT = ".md,.markdown,.pdf,.docx,.xlsx,text/markdown,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

export function DocUploader({ kbId }: { kbId: string }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const upload = useUploadDoc(kbId);

  function pickFile() {
    inputRef.current?.click();
  }

  function handleFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    for (const f of Array.from(files)) {
      upload.mutate(f, {
        onSuccess: () => toast.success(`已上传 ${f.name}, 摄入中...`),
        onError: (e) => {
          if (e instanceof APIError && e.code === "duplicate_checksum") {
            toast.info(`${f.name} 已存在 (existing_doc_id=${e.details?.existing_doc_id})`);
          } else if (e instanceof APIError && e.code === "payload_too_large") {
            toast.error(`${f.name} 超过 50MB 限制`);
          } else {
            toast.error(`${f.name} 上传失败: ${(e as Error).message}`);
          }
        },
      });
    }
  }

  return (
    <div
      onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
      onDragLeave={() => setDragging(false)}
      onDrop={(e) => {
        e.preventDefault();
        setDragging(false);
        handleFiles(e.dataTransfer.files);
      }}
      className={`border-2 border-dashed rounded-lg p-6 text-center transition-colors ${
        dragging ? "border-primary bg-primary/5" : "border-muted-foreground/30"
      }`}
    >
      <UploadCloud className="size-8 mx-auto mb-2 text-muted-foreground" />
      <p className="text-sm text-muted-foreground mb-3">
        拖入文件到此处，或
      </p>
      <Button onClick={pickFile} disabled={upload.isPending}>
        {upload.isPending ? "上传中..." : "选择文件"}
      </Button>
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        accept={ACCEPT}
        multiple
        onChange={(e) => handleFiles(e.target.files)}
      />
      <p className="text-xs text-muted-foreground mt-3">
        支持 .md / .pdf / .docx / .xlsx, 单文件 ≤ 50MB
      </p>
    </div>
  );
}
