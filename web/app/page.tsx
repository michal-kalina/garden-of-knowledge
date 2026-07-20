import { ChatPanel } from "@/components/chat-panel";
import { DocumentsPanel } from "@/components/documents-panel";

export default function Home() {
  return (
    <div className="flex h-dvh">
      <DocumentsPanel />
      <ChatPanel />
    </div>
  );
}
