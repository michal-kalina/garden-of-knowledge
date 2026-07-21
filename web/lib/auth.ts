// Client-side session storage.
//
// The token lives in localStorage — the pragmatic choice for this stage;
// its known trade-off is XSS exposure. The production-grade alternative
// (httpOnly cookie set by the API, with CSRF handling) is tracked in the
// roadmap and would leave this module's interface unchanged.

const TOKEN_KEY = "gok_token";
const EMAIL_KEY = "gok_email";

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function getEmail(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(EMAIL_KEY);
}

export function setSession(token: string, email: string): void {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(EMAIL_KEY, email);
}

export function clearSession(): void {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(EMAIL_KEY);
}
