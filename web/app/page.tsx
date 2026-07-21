"use client";

import React from "react";
import { AuthPanel } from "@/components/auth-panel";
import { ChatPanel } from "@/components/chat-panel";
import { DocumentsPanel } from "@/components/documents-panel";
import { getToken } from "@/lib/auth";

export default function Home() {
  // Session state is read after mount: localStorage does not exist during
  // server rendering, so deciding there would cause a hydration mismatch.
  const [session, setSession] = React.useState<"unknown" | "in" | "out">("unknown");

  React.useEffect(() => {
    setSession(getToken() ? "in" : "out");
  }, []);

  if (session === "unknown") return null;
  if (session === "out") return <AuthPanel onSignedIn={() => setSession("in")} />;

  return (
    <div className="flex h-dvh">
      <DocumentsPanel onLogout={() => setSession("out")} />
      <ChatPanel />
    </div>
  );
}
