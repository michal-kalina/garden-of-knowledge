"use client";

import React from "react";
import { SendHorizontal } from "lucide-react";
import { AnswerText } from "@/components/answer-text";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  getConversation,
  streamChat,
  type Source,
  type StoredMessage,
} from "@/lib/api";
import { cn } from "@/lib/utils";

interface AssistantMessage {
  role: "assistant";
  text: string;
  sources: Source[];
  error?: string;
}
interface UserMessage {
  role: "user";
  text: string;
}
type Message = AssistantMessage | UserMessage;

function fromStored(m: StoredMessage): Message {
  return m.role === "user"
    ? { role: "user", text: m.content }
    : { role: "assistant", text: m.content, sources: m.sources ?? [] };
}

function sourceLabel(s: Source): string {
  let label = s.filename;
  if (s.heading) label += ` — ${s.heading}`;
  if (s.page_start != null) {
    label +=
      s.page_end != null && s.page_end !== s.page_start
        ? ` · pp. ${s.page_start}–${s.page_end}`
        : ` · p. ${s.page_start}`;
  }
  return label;
}

function SourceCards({ msgIndex, sources }: { msgIndex: number; sources: Source[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="mt-3">
      <p className="mb-1.5 font-mono text-[0.65rem] uppercase tracking-widest text-muted-foreground">
        Sources
      </p>
      <div className="grid gap-1.5 sm:grid-cols-2">
        {sources.map((s) => (
          <Card key={s.index} id={`src-${msgIndex}-${s.index}`} className="min-w-0">
            <CardContent className="p-2.5">
              <p className="flex items-baseline gap-1.5 font-mono text-[0.7rem] text-primary">
                <span className="font-semibold">[{s.index}]</span>
                <span className="truncate text-foreground/80">{sourceLabel(s)}</span>
              </p>
              <p className="mt-1 line-clamp-3 text-xs leading-snug text-muted-foreground">
                {s.content}
              </p>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

interface ChatPanelProps {
  // undefined = a fresh, not-yet-created conversation. The parent should
  // pass a React `key` keyed on this value so switching conversations
  // remounts the panel instead of trying to reconcile unrelated state.
  conversationId?: string;
  // Fired once, the moment the backend assigns an id to a brand-new
  // conversation (from the first message's "conversation" SSE event) — lets
  // the parent select it in the sidebar without waiting for the answer.
  onConversationStarted?: (id: string) => void;
  // Fired after an exchange is successfully persisted, so the parent can
  // refresh the conversation list (new title, reordering).
  onActivity?: () => void;
}

export function ChatPanel({
  conversationId,
  onConversationStarted,
  onActivity,
}: ChatPanelProps) {
  const [messages, setMessages] = React.useState<Message[]>([]);
  const [loadingHistory, setLoadingHistory] = React.useState(conversationId != null);
  const [input, setInput] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const bottom = React.useRef<HTMLDivElement>(null);
  // Tracks the conversation this panel instance is writing to. Seeded from
  // the prop; updated the moment a fresh chat gets its id assigned, so the
  // SECOND message in the same "new chat" session continues it instead of
  // starting yet another conversation.
  const convId = React.useRef<string | undefined>(conversationId);

  React.useEffect(() => {
    if (!conversationId) return;
    let cancelled = false;
    setLoadingHistory(true);
    getConversation(conversationId)
      .then((conv) => {
        if (!cancelled) setMessages(conv.messages.map(fromStored));
      })
      .catch(() => {
        if (!cancelled) {
          setMessages([
            { role: "assistant", text: "", sources: [], error: "Could not load this conversation." },
          ]);
        }
      })
      .finally(() => {
        if (!cancelled) setLoadingHistory(false);
      });
    return () => {
      cancelled = true;
    };
    // conversationId is stable for the lifetime of this component instance
    // (the parent remounts via `key` on change) — see ChatPanelProps.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  React.useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: "smooth", block: "end" });
  }, [messages]);

  function flashSource(msgIndex: number, n: number) {
    const el = document.getElementById(`src-${msgIndex}-${n}`);
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    el.classList.remove("source-flash");
    // Force a reflow so re-adding the class restarts the animation.
    void el.offsetWidth;
    el.classList.add("source-flash");
  }

  async function ask(e: React.FormEvent) {
    e.preventDefault();
    const query = input.trim();
    if (query === "" || busy) return;

    setInput("");
    setBusy(true);
    setMessages((m) => [
      ...m,
      { role: "user", text: query },
      { role: "assistant", text: "", sources: [] },
    ]);

    const patchLast = (patch: Partial<AssistantMessage>) =>
      setMessages((m) => {
        const next = [...m];
        const last = next[next.length - 1] as AssistantMessage;
        next[next.length - 1] = { ...last, ...patch, text: patch.text ?? last.text };
        return next;
      });

    try {
      let acc = "";
      await streamChat(query, convId.current, {
        onConversation: (id) => {
          if (convId.current !== id) {
            convId.current = id;
            onConversationStarted?.(id);
          }
        },
        onSources: (sources) => patchLast({ sources }),
        onDelta: (text) => {
          acc += text;
          patchLast({ text: acc });
        },
      });
      onActivity?.();
    } catch (err) {
      patchLast({ error: err instanceof Error ? err.message : "chat failed" });
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="flex h-full min-w-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto max-w-2xl px-4 py-6">
          {loadingHistory ? (
            <p className="mt-24 text-center font-mono text-xs text-muted-foreground">
              <span className="germinate inline-block h-1.5 w-1.5 rounded-full bg-primary align-middle" />{" "}
              loading conversation…
            </p>
          ) : messages.length === 0 ? (
            <div className="mt-24 text-center">
              <p className="font-mono text-[0.7rem] uppercase tracking-widest text-muted-foreground">
                ask the garden
              </p>
              <p className="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-muted-foreground">
                Answers come only from your uploaded documents, with every claim
                citing its source. Try &ldquo;What does this project support?&rdquo;
              </p>
            </div>
          ) : null}

          <div className="space-y-6">
            {messages.map((msg, i) =>
              msg.role === "user" ? (
                <div key={i} className="flex justify-end">
                  <p className="max-w-[85%] rounded-lg bg-primary px-3.5 py-2 text-sm text-primary-foreground">
                    {msg.text}
                  </p>
                </div>
              ) : (
                <div key={i}>
                  {msg.text === "" && !msg.error ? (
                    <p className="font-mono text-xs text-muted-foreground">
                      <span className="germinate inline-block h-1.5 w-1.5 rounded-full bg-primary align-middle" />{" "}
                      searching the garden…
                    </p>
                  ) : (
                    <AnswerText text={msg.text} onCite={(n) => flashSource(i, n)} />
                  )}
                  {msg.error && (
                    <p className="mt-2 text-sm text-destructive" role="alert">
                      {msg.error}
                    </p>
                  )}
                  <SourceCards msgIndex={i} sources={msg.sources} />
                </div>
              ),
            )}
          </div>
          <div ref={bottom} />
        </div>
      </div>

      <form onSubmit={ask} className="border-t bg-background">
        <div className="mx-auto flex max-w-2xl gap-2 px-4 py-3">
          <Input
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Ask about your documents…"
            aria-label="Question"
            disabled={busy || loadingHistory}
          />
          <Button
            type="submit"
            size="icon"
            disabled={busy || loadingHistory || input.trim() === ""}
          >
            <SendHorizontal className="h-4 w-4" aria-hidden />
            <span className="sr-only">Send</span>
          </Button>
        </div>
      </form>
    </main>
  );
}
