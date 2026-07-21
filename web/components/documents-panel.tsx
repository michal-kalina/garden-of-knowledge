"use client";

import React from "react";
import { LogOut, Sprout, Upload } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { listDocuments, uploadDocument, type Doc } from "@/lib/api";
import { clearSession, getEmail } from "@/lib/auth";

const ACCEPT = ".pdf,.md,.txt,application/pdf,text/markdown,text/plain";

function statusVariant(s: Doc["status"]) {
  return s; // badge variants share the status vocabulary
}

export function DocumentsPanel({ onLogout }: { onLogout: () => void }) {
  const [docs, setDocs] = React.useState<Doc[]>([]);
  const [error, setError] = React.useState<string | null>(null);
  const [uploading, setUploading] = React.useState(false);
  const fileInput = React.useRef<HTMLInputElement>(null);

  const refresh = React.useCallback(async () => {
    try {
      setDocs(await listDocuments());
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "listing documents failed");
    }
  }, []);

  // Poll faster while any document is still moving through the pipeline.
  React.useEffect(() => {
    void refresh();
    const active = docs.some((d) => d.status === "pending" || d.status === "processing");
    const id = setInterval(() => void refresh(), active ? 2500 : 10000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refresh, docs.some((d) => d.status === "pending" || d.status === "processing")]);

  async function onFileChosen(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      await uploadDocument(file);
      await refresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : "upload failed");
    } finally {
      setUploading(false);
    }
  }

  return (
    <aside className="flex h-full w-72 shrink-0 flex-col bg-sidebar text-sidebar-foreground">
      <header className="flex items-center gap-2.5 px-4 pb-5 pt-5">
        <Sprout className="h-5 w-5 text-primary-foreground/90" aria-hidden />
        <div>
          <h1 className="text-sm font-semibold tracking-tight">Garden of Knowledge</h1>
          <p className="font-mono text-[0.65rem] uppercase tracking-widest text-sidebar-muted">
            private knowledge base
          </p>
        </div>
      </header>

      <div className="px-4">
        <input
          ref={fileInput}
          type="file"
          accept={ACCEPT}
          className="hidden"
          onChange={onFileChosen}
        />
        <Button
          variant="sidebar"
          className="w-full"
          disabled={uploading}
          onClick={() => fileInput.current?.click()}
        >
          <Upload className="h-4 w-4" aria-hidden />
          {uploading ? "Uploading…" : "Upload document"}
        </Button>
        <p className="mt-2 font-mono text-[0.65rem] text-sidebar-muted">
          PDF, Markdown or plain text
        </p>
        {error && (
          <p className="mt-2 text-xs text-red-300" role="alert">
            {error}
          </p>
        )}
      </div>

      <div className="mt-5 min-h-0 flex-1 overflow-y-auto px-2">
        {docs.length === 0 ? (
          <p className="px-2 text-xs leading-relaxed text-sidebar-muted">
            No documents yet. Upload a file to grow the knowledge base — questions
            in the chat are answered only from what lives here.
          </p>
        ) : (
          <ul className="space-y-0.5">
            {docs.map((d) => (
              <li
                key={d.id}
                className="rounded-md px-2 py-2 hover:bg-sidebar-foreground/5"
                title={d.error ?? undefined}
              >
                <p className="truncate font-mono text-xs">{d.filename}</p>
                <div className="mt-1 flex items-center gap-2">
                  <Badge variant={statusVariant(d.status)}>
                    {d.status === "processing" && (
                      <span
                        className="germinate inline-block h-1.5 w-1.5 rounded-full bg-amber"
                        aria-hidden
                      />
                    )}
                    {d.status}
                  </Badge>
                  <span className="font-mono text-[0.65rem] text-sidebar-muted">
                    {(d.size_bytes / 1024).toFixed(0)} KB
                  </span>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
      <footer className="flex items-center justify-between gap-2 border-t border-sidebar-foreground/10 px-4 py-3">
        <span className="truncate font-mono text-[0.68rem] text-sidebar-muted" title={getEmail() ?? undefined}>
          {getEmail()}
        </span>
        <button
          type="button"
          className="flex shrink-0 items-center gap-1 font-mono text-[0.68rem] uppercase tracking-wide text-sidebar-muted transition-colors hover:text-sidebar-foreground"
          onClick={() => {
            clearSession();
            onLogout();
          }}
        >
          <LogOut className="h-3 w-3" aria-hidden />
          Sign out
        </button>
      </footer>
    </aside>
  );
}
