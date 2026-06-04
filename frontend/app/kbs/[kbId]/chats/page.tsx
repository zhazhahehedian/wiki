"use client";

import { use, useState } from "react";

import { ChatHeader } from "@/components/chat/chat-header";
import { ChatInput } from "@/components/chat/chat-input";
import { ChatSidebar } from "@/components/chat/chat-sidebar";
import { CitationDrawer } from "@/components/chat/citation-drawer";
import { MessageList } from "@/components/chat/message-list";
import type { Citation } from "@/lib/schemas";

export default function KBChatsPage({ params }: { params: Promise<{ kbId: string }> }) {
  const { kbId } = use(params);
  const [selectedCitation, setSelectedCitation] = useState<Citation | null>(null);

  return (
    <main className="min-h-screen bg-background">
      <div className="mx-auto flex min-h-screen max-w-7xl flex-col md:flex-row">
        <ChatSidebar kbId={kbId} />
        <section className="flex min-h-[70vh] min-w-0 flex-1 flex-col border-x">
          <ChatHeader kbId={kbId} />
          <MessageList messages={[]} onCitationClick={setSelectedCitation} />
          <ChatInput disabled onSend={() => undefined} onStop={() => undefined} />
        </section>
        <CitationDrawer kbId={kbId} citation={selectedCitation} onOpenChange={setSelectedCitation} />
      </div>
    </main>
  );
}
