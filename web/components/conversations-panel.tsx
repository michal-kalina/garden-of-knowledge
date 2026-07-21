"use client";

import React from "react";
import { MessageSquarePlus, Trash2 } from "lucide-react";
import {
  deleteConversation,
  listConversations,
  type ConversationSummary,
} from "@/lib/api";
import { cn } from "@/lib/utils";

function relativeDay(iso: string): string {
  const d = new Date(iso);
  const days = Math.floor((Date.now() - d.getTime()) / 86_400_000);
  if (days <= 0) return "today";
  if (days === 1) return "yesterday";
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString();
}

interface ConversationsPanelProps {
  activeId?: string;
  // Bump this number (e.g. from a parent counter) to force a refetch —
  // simpler than plumbing a shared cache for a list this small.
  refreshKey: number;
  onSelect: (id: string) => void;
  onNewChat: () => void;
}

export function ConversationsPanel({
  activeId,
  refreshKey,
  onSelect,
  onNewChat,
}: ConversationsPanelProps) {
  const [items, setItems] = React.useState<ConversationSummary[]>([]);
  const [loaded, setLoaded] = React.useState(false);

  const refresh = React.useCallback(() => {
    listConversations()
      .then(setItems)
      .catch(() => {
        // Non-critical: the chat itself still works without a sidebar list.
      })
      .finally(() => setLoaded(true));
  }, []);

  React.useEffect(() => {
    refresh();
  }, [refresh, refreshKey]);

  async function remove(e: React.MouseEvent, id: string) {
    e.stopPropagation();
    setItems((prev) => prev.filter((c) => c.id !== id));
    if (id === activeId) onNewChat();
    try {
      await deleteConversation(id);
    } catch {
      refresh(); // roll back the optimistic removal if it didn't actually work
    }
  }

  return (
    <aside className="flex h-full w-60 shrink-0 flex-col border-r bg-background">
      <div className="p-3">
        <button
          type="button"
          onClick={onNewChat}
          className="flex w-full items-center gap-2 rounded-md border border-dashed px-3 py-2 text-sm text-muted-foreground transition-colors hover:border-primary hover:text-primary"
        >
          <MessageSquarePlus className="h-4 w-4" aria-hidden />
          New chat
        </button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        {!loaded ? null : items.length === 0 ? (
          <p className="px-2 text-xs leading-relaxed text-muted-foreground">
            Past conversations will show up here once you ask something.
          </p>
        ) : (
          <ul className="space-y-0.5">
            {items.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => onSelect(c.id)}
                  className={cn(
                    "group flex w-full items-center justify-between gap-1 rounded-md px-2 py-2 text-left transition-colors hover:bg-muted",
                    c.id === activeId && "bg-muted",
                  )}
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-xs text-foreground/90">
                      {c.title || "Untitled conversation"}
                    </span>
                    <span className="font-mono text-[0.65rem] text-muted-foreground">
                      {relativeDay(c.updated_at)}
                    </span>
                  </span>
                  <span
                    role="button"
                    tabIndex={0}
                    aria-label="Delete conversation"
                    onClick={(e) => remove(e, c.id)}
                    className="shrink-0 rounded p-1 text-muted-foreground opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100"
                  >
                    <Trash2 className="h-3.5 w-3.5" aria-hidden />
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </aside>
  );
}
