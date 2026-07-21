"use client";

import React from "react";
import { Sprout } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { login, register } from "@/lib/api";
import { setSession } from "@/lib/auth";

export function AuthPanel({ onSignedIn }: { onSignedIn: () => void }) {
  const [mode, setMode] = React.useState<"login" | "register">("login");
  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [busy, setBusy] = React.useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const session =
        mode === "login"
          ? await login(email, password)
          : await register(email, password);
      setSession(session.token, session.user.email);
      onSignedIn();
    } catch (err) {
      setError(err instanceof Error ? err.message : "something went wrong");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex h-dvh items-center justify-center bg-sidebar px-4">
      <Card className="w-full max-w-sm border-0 bg-background shadow-xl">
        <CardContent className="p-6">
          <div className="mb-6 flex items-center gap-2.5">
            <Sprout className="h-5 w-5 text-primary" aria-hidden />
            <div>
              <h1 className="text-sm font-semibold tracking-tight">
                Garden of Knowledge
              </h1>
              <p className="font-mono text-[0.65rem] uppercase tracking-widest text-muted-foreground">
                {mode === "login" ? "sign in" : "create account"}
              </p>
            </div>
          </div>

          <form onSubmit={submit} className="space-y-3">
            <Input
              type="email"
              required
              autoComplete="email"
              placeholder="Email"
              aria-label="Email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <Input
              type="password"
              required
              minLength={mode === "register" ? 8 : undefined}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              placeholder={mode === "register" ? "Password (min. 8 characters)" : "Password"}
              aria-label="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            {error && (
              <p className="text-sm text-destructive" role="alert">
                {error}
              </p>
            )}
            <Button type="submit" className="w-full" disabled={busy}>
              {busy ? "…" : mode === "login" ? "Sign in" : "Create account"}
            </Button>
          </form>

          <button
            type="button"
            className="mt-4 w-full text-center text-xs text-muted-foreground underline-offset-2 hover:underline"
            onClick={() => {
              setMode(mode === "login" ? "register" : "login");
              setError(null);
            }}
          >
            {mode === "login"
              ? "No account yet? Create one"
              : "Already have an account? Sign in"}
          </button>
        </CardContent>
      </Card>
    </div>
  );
}
