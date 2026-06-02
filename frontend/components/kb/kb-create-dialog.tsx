"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useCreateKb } from "@/lib/hooks/use-kbs";
import { toast } from "sonner";

export function KBCreateDialog() {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [desc, setDesc] = useState("");
  const create = useCreateKb();

  function reset() {
    setName("");
    setDesc("");
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    create.mutate({ name: name.trim(), description: desc.trim() }, {
      onSuccess: () => {
        toast.success(`已创建 "${name.trim()}"`);
        reset();
        setOpen(false);
      },
      onError: (e) => toast.error("创建失败: " + (e as Error).message),
    });
  }

  return (
    <Dialog open={open} onOpenChange={(v) => { setOpen(v); if (!v) reset(); }}>
      <DialogTrigger render={<Button />}>
        新建知识库
      </DialogTrigger>
      <DialogContent>
        <form onSubmit={handleSubmit}>
          <DialogHeader>
            <DialogTitle>新建知识库</DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="kb-name">名称</Label>
              <Input id="kb-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：工程团队 KB" autoFocus />
            </div>
            <div className="space-y-2">
              <Label htmlFor="kb-desc">描述（可选）</Label>
              <Input id="kb-desc" value={desc} onChange={(e) => setDesc(e.target.value)} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setOpen(false)} disabled={create.isPending}>取消</Button>
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              {create.isPending ? "创建中..." : "创建"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
