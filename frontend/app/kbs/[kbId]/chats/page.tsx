"use client";

import { use, useState } from "react";

import { ChatHeader } from "@/components/chat/chat-header";
import { ChatInput } from "@/components/chat/chat-input";
import { CitationDrawer } from "@/components/chat/citation-drawer";
import { MessageList } from "@/components/chat/message-list";
import type { Citation } from "@/lib/schemas";

export default function KBChatsPage({ params }: { params: Promise<{ kbId: string }> }) {
  const { kbId } = use(params);
  const [selectedCitation, setSelectedCitation] = useState<Citation | null>(null);

  return (
    <>
      <ChatHeader kbId={kbId} />
      <MessageList messages={[]} onCitationClick={setSelectedCitation} />
      <ChatInput disabled onSend={() => undefined} onStop={() => undefined} />
      <CitationDrawer kbId={kbId} citation={selectedCitation} onOpenChange={setSelectedCitation} />
    </>
  );
}
