// API client for the Go backend, reached through the /backend/* rewrite
// (see next.config.mjs) so the browser never deals with CORS.

import { clearSession, getToken } from "@/lib/auth";

export interface User {
  id: string;
  email: string;
}

interface Session {
  token: string;
  user: User;
}

async function credentials(
  path: "register" | "login",
  email: string,
  password: string,
): Promise<Session> {
  const res = await fetch(`/backend/auth/${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email, password }),
  });
  const body = (await res.json().catch(() => null)) as
    | (Session & { error?: string })
    | null;
  if (!res.ok) throw new Error(body?.error ?? `${path} failed (${res.status})`);
  return body as Session;
}

export const register = (email: string, password: string) =>
  credentials("register", email, password);
export const login = (email: string, password: string) =>
  credentials("login", email, password);

function authHeaders(): Record<string, string> {
  const token = getToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

// checkSession centralizes 401 handling: an expired or revoked token drops
// the session and sends the user back to the sign-in screen.
function checkSession(res: Response): Response {
  if (res.status === 401 && getToken()) {
    clearSession();
    window.location.reload();
  }
  return res;
}

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

export interface ConversationSummary {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

export interface StoredMessage {
  id: number;
  role: "user" | "assistant";
  content: string;
  sources?: Source[];
  created_at: string;
}

export interface ConversationDetail extends ConversationSummary {
  messages: StoredMessage[];
}

export async function listConversations(): Promise<ConversationSummary[]> {
  const res = checkSession(await fetch("/backend/conversations", { headers: authHeaders() }));
  if (!res.ok) throw new Error(`listing conversations failed (${res.status})`);
  const body = (await res.json()) as { conversations: ConversationSummary[] };
  return body.conversations;
}

export async function getConversation(id: string): Promise<ConversationDetail> {
  const res = checkSession(
    await fetch(`/backend/conversations/${id}`, { headers: authHeaders() }),
  );
  if (!res.ok) throw new Error(`loading conversation failed (${res.status})`);
  return (await res.json()) as ConversationDetail;
}

export async function deleteConversation(id: string): Promise<void> {
  const res = checkSession(
    await fetch(`/backend/conversations/${id}`, { method: "DELETE", headers: authHeaders() }),
  );
  if (!res.ok && res.status !== 404) {
    throw new Error(`deleting conversation failed (${res.status})`);
  }
}

export async function listDocuments(): Promise<Doc[]> {
  const res = checkSession(await fetch("/backend/documents", { headers: authHeaders() }));
  if (!res.ok) throw new Error(`listing documents failed (${res.status})`);
  const body = (await res.json()) as { documents: Doc[] };
  return body.documents;
}

export async function uploadDocument(file: File): Promise<Doc> {
  const form = new FormData();
  form.append("file", file, file.name);
  const res = checkSession(
    await fetch("/backend/documents", {
      method: "POST",
      body: form,
      headers: authHeaders(),
    }),
  );
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error ?? `upload failed (${res.status})`);
  }
  return (await res.json()) as Doc;
}

// retryDocument re-enqueues a document that failed ingestion. The backend
// only allows this from the 'failed' state (409 otherwise).
export async function retryDocument(id: string): Promise<Doc> {
  const res = checkSession(
    await fetch(`/backend/documents/${id}/retry`, { method: "POST", headers: authHeaders() }),
  );
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error ?? `retry failed (${res.status})`);
  }
  return (await res.json()) as Doc;
}

// deleteDocument removes the document and everything derived from it
// (chunks, ingestion job) via the backend's cascade, plus the underlying
// object in storage.
export async function deleteDocument(id: string): Promise<void> {
  const res = checkSession(
    await fetch(`/backend/documents/${id}`, { method: "DELETE", headers: authHeaders() }),
  );
  if (!res.ok && res.status !== 404) {
    throw new Error(`delete failed (${res.status})`);
  }
}

export interface ChatHandlers {
  onConversation?: (id: string) => void;
  onSources: (sources: Source[]) => void;
  onDelta: (text: string) => void;
}

// streamChat POSTs the question and consumes the SSE response manually.
// EventSource cannot send POST bodies, so we read the fetch body stream and
// parse frames ourselves: frames are separated by a blank line, each frame
// carries "event:" and "data:" lines.
//
// conversationId continues an existing conversation; omit it to start a new
// one — the backend creates it and reports its id via onConversation, which
// fires before onSources so the caller can key state on it immediately.
export async function streamChat(
  query: string,
  conversationId: string | undefined,
  handlers: ChatHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const res = checkSession(
    await fetch("/backend/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json", ...authHeaders() },
      body: JSON.stringify({ query, conversation_id: conversationId ?? "" }),
      signal,
    }),
  );
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
        case "conversation":
          handlers.onConversation?.((JSON.parse(data) as { id: string }).id);
          break;
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
