// API client for the Go backend, reached through the /backend/* rewrite
// (see next.config.mjs) so the browser never deals with CORS.

export type DocumentStatus = "pending" | "processing" | "ready" | "failed";

export interface Doc {
  id: string;
  filename: string;
  content_type: string;
  size_bytes: number;
  status: DocumentStatus;
  error?: string;
  created_at: string;
  updated_at: string;
}

export interface Source {
  index: number;
  chunk_id: number;
  document_id: string;
  filename: string;
  heading?: string;
  page_start?: number;
  page_end?: number;
  content: string;
}

export async function listDocuments(): Promise<Doc[]> {
  const res = await fetch("/backend/documents");
  if (!res.ok) throw new Error(`listing documents failed (${res.status})`);
  const body = (await res.json()) as { documents: Doc[] };
  return body.documents;
}

export async function uploadDocument(file: File): Promise<Doc> {
  const form = new FormData();
  form.append("file", file, file.name);
  const res = await fetch("/backend/documents", { method: "POST", body: form });
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error ?? `upload failed (${res.status})`);
  }
  return (await res.json()) as Doc;
}

export interface ChatHandlers {
  onSources: (sources: Source[]) => void;
  onDelta: (text: string) => void;
}

// streamChat POSTs the question and consumes the SSE response manually.
// EventSource cannot send POST bodies, so we read the fetch body stream and
// parse frames ourselves: frames are separated by a blank line, each frame
// carries "event:" and "data:" lines.
export async function streamChat(
  query: string,
  handlers: ChatHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch("/backend/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ query }),
    signal,
  });
  if (!res.ok || !res.body) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error ?? `chat failed (${res.status})`);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    let sep: number;
    while ((sep = buffer.indexOf("\n\n")) >= 0) {
      const frame = buffer.slice(0, sep);
      buffer = buffer.slice(sep + 2);

      let event = "";
      let data = "";
      for (const line of frame.split("\n")) {
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (!data) continue;

      switch (event) {
        case "sources":
          handlers.onSources(JSON.parse(data) as Source[]);
          break;
        case "delta":
          handlers.onDelta((JSON.parse(data) as { text: string }).text);
          break;
        case "error":
          throw new Error((JSON.parse(data) as { error: string }).error);
        case "done":
          return;
      }
    }
  }
}
