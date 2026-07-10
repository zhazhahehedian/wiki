"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, usePathname } from "next/navigation";
import { BookOpen, Library, MessageSquare, Settings } from "lucide-react";

import { ThemeToggle } from "@/components/layout/theme-toggle";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { getLastKbId } from "@/lib/last-kb";
import { cn } from "@/lib/utils";

export type RailSection = "chats" | "kbs";

function RailItem({
  section,
  active,
  href,
  icon: Icon,
  label,
}: {
  section: RailSection;
  active: boolean;
  href: string;
  icon: typeof MessageSquare;
  label: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Link
            href={href}
            data-section={section}
            aria-label={label}
            className={cn(
              "flex size-9 items-center justify-center rounded-lg transition-colors",
              active
                ? "bg-primary text-primary-foreground"
                : "text-muted-foreground hover:bg-accent hover:text-foreground",
            )}
          >
            <Icon className="size-4" />
          </Link>
        }
      />
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

export function Rail() {
  const pathname = usePathname();
  const params = useParams<{ kbId?: string }>();
  const activeSection: RailSection = pathname.includes("/chats") ? "chats" : "kbs";
  // localStorage 只能客户端读；用 state + effect 避免 SSR/CSR href 不一致的 hydration 告警
  const [lastKb, setLastKb] = useState<string | null>(null);
  useEffect(() => setLastKb(getLastKbId()), [pathname]);
  const kbId = params.kbId ?? lastKb;

  return (
    <div className="flex w-14 shrink-0 flex-col items-center gap-2 border-r border-sidebar-border bg-sidebar py-3">
      <div className="mb-2 flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
        <BookOpen className="size-5" />
      </div>
      <RailItem
        section="chats"
        active={activeSection === "chats"}
        href={kbId ? `/kbs/${kbId}/chats` : "/"}
        icon={MessageSquare}
        label="聊天"
      />
      <RailItem
        section="kbs"
        active={activeSection === "kbs"}
        href={kbId ? `/kbs/${kbId}/docs` : "/kbs"}
        icon={Library}
        label="知识库"
      />
      <div className="mt-auto flex flex-col items-center gap-1">
        <ThemeToggle />
        <Button type="button" variant="ghost" size="icon" aria-label="设置（即将上线）" disabled>
          <Settings className="size-4" />
        </Button>
      </div>
    </div>
  );
}
