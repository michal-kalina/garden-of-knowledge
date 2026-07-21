"use client";

import React from "react";
import { AuthPanel } from "@/components/auth-panel";
import { ChatPanel } from "@/components/chat-panel";
import { ConversationsPanel } from "@/components/conversations-panel";
import { DocumentsPanel } from "@/components/documents-panel";
import { getToken } from "@/lib/auth";

export default function Home() {
  // Session state is read after mount: localStorage does not exist during
  // server rendering, so deciding there would cause a hydration mismatch.
  const [session, setSession] = React.useState<"unknown" | "in" | "out">("unknown");

  // undefined = a fresh, not-yet-created conversation.
  const [activeId, setActiveId] = React.useState<string | undefined>(undefined);
  // Bumped whenever an exchange completes, so ConversationsPanel refetches
  // (new title, reordering) without a shared cache between the two panels.
  const [refreshKey, setRefreshKey] = React.useState(0);

  React.useEffect(() => {
    setSession(getToken() ? "in" : "out");
  }, []);

  if (session === "unknown") return null;
  if (session === "out") return <AuthPanel onSignedIn={() => setSession("in")} />;

  return (
    <div className="flex h-dvh">
      <DocumentsPanel onLogout={() => setSession("out")} />
      <ConversationsPanel
        activeId={activeId}
        refreshKey={refreshKey}
        onSelect={setActiveId}
        onNewChat={() => setActiveId(undefined)}
      />
      {/* Keying on activeId remounts ChatPanel on switch — a fresh component
          instance is simpler and safer than reconciling unrelated message
          state in place (see ChatPanelProps in chat-panel.tsx). */}
      <ChatPanel
        key={activeId ?? "new"}
        conversationId={activeId}
        onConversationStarted={setActiveId}
        onActivity={() => setRefreshKey((k) => k + 1)}
      />
    </div>
  );
}
