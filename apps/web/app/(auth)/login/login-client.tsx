"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@dars/ui/components/ui/button";
import { Input } from "@dars/ui/components/ui/input";
import { Label } from "@dars/ui/components/ui/label";
import { useAuthStore } from "@dars/core/auth";
import { lightweightApi } from "@dars/core/lightweight";
import { paths } from "@dars/core/paths";
import { useNavigation } from "@dars/views/navigation";
import type { DevAutoLoginConfig } from "@/config/dev-auto-login";

const DEV_DEMO_WORKSPACE = { name: "Demo", slug: "demo" } as const;

async function enterWorkspace(
  navigation: { replace: (path: string) => void },
  createIfEmpty: boolean,
): Promise<void> {
  const workspaces = await lightweightApi.listWorkspaces();
  const first = workspaces[0];
  if (first) {
    navigation.replace(paths.workspace(first.slug).runs());
    return;
  }
  if (!createIfEmpty) {
    navigation.replace(paths.newWorkspace());
    return;
  }
  const created = await lightweightApi.createWorkspace({
    name: DEV_DEMO_WORKSPACE.name,
    slug: DEV_DEMO_WORKSPACE.slug,
  });
  navigation.replace(paths.workspace(created.slug).runs());
}

export interface LoginPageClientProps {
  autoLogin: DevAutoLoginConfig;
}

export default function LoginPageClient({ autoLogin }: LoginPageClientProps) {
  const navigation = useNavigation();
  const user = useAuthStore((state) => state.user);
  const isLoading = useAuthStore((state) => state.isLoading);
  const sendCode = useAuthStore((state) => state.sendCode);
  const verifyCode = useAuthStore((state) => state.verifyCode);
  const [email, setEmail] = useState(autoLogin.enabled ? autoLogin.email : "");
  const [code, setCode] = useState(autoLogin.enabled ? autoLogin.code : "");
  const [codeSent, setCodeSent] = useState(false);
  const [pending, setPending] = useState(autoLogin.enabled);
  const [error, setError] = useState<string | null>(null);
  const [manualMode, setManualMode] = useState(!autoLogin.enabled);
  const autoLoginAttempted = useRef(false);
  const workspaceEntryAttempted = useRef(false);

  useEffect(() => {
    if (isLoading || !user || workspaceEntryAttempted.current) return;
    workspaceEntryAttempted.current = true;
    setPending(true);
    void enterWorkspace(navigation, autoLogin.enabled)
      .catch((cause: unknown) => {
        workspaceEntryAttempted.current = false;
        setManualMode(true);
        setError(cause instanceof Error ? cause.message : "Could not open workspace");
      })
      .finally(() => setPending(false));
  }, [autoLogin.enabled, isLoading, navigation, user]);

  useEffect(() => {
    if (!autoLogin.enabled || isLoading || user || autoLoginAttempted.current) return;
    autoLoginAttempted.current = true;
    setPending(true);
    setError(null);
    setEmail(autoLogin.email);
    setCode(autoLogin.code);
    void sendCode(autoLogin.email)
      .then(() => {
        setCodeSent(true);
        return verifyCode(autoLogin.email, autoLogin.code);
      })
      .catch((cause: unknown) => {
        setManualMode(true);
        setError(cause instanceof Error ? cause.message : "Sign-in failed");
      })
      .finally(() => setPending(false));
  }, [autoLogin, isLoading, manualMode, sendCode, user, verifyCode]);

  const showBootstrap = autoLogin.enabled && !manualMode && !error;

  return (
    <main className="grid h-svh place-items-center bg-muted/20 p-6">
      <div className="w-full max-w-sm rounded-2xl border bg-card p-6 shadow-sm">
        <div className="mb-6">
          <div className="text-caption font-semibold uppercase tracking-[0.2em] text-muted-foreground">
            DARS Lightweight
          </div>
          <h1 className="mt-2 text-display-sm font-semibold">
            {showBootstrap ? "Signing you in" : "Sign in with email"}
          </h1>
          <p className="mt-1 text-body text-muted-foreground">
            {showBootstrap
              ? `Using ${autoLogin.email} and opening the default workspace.`
              : autoLogin.enabled
                ? "Development auto-login failed. You can retry or continue manually."
                : "Email verification is the only supported login method."}
          </p>
        </div>
        {showBootstrap ? (
          <div className="grid gap-4">
            <p className="text-body text-muted-foreground" aria-live="polite">
              {pending || isLoading || user ? "Please wait…" : "Starting…"}
            </p>
          </div>
        ) : (
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              setPending(true);
              setError(null);
              const form = new FormData(event.currentTarget);
              const nextCode = String(form.get("code") ?? code).trim();
              const action = codeSent
                ? verifyCode(email, nextCode)
                : sendCode(email).then(() => {
                    setCodeSent(true);
                    if (autoLogin.enabled && !code) setCode(autoLogin.code);
                  });
              void action
                .catch((cause: unknown) => setError(cause instanceof Error ? cause.message : "Sign-in failed"))
                .finally(() => setPending(false));
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                name="email"
                type="email"
                autoComplete="email"
                required
                disabled={codeSent || pending}
                value={email}
                onChange={(event) => setEmail(event.target.value)}
              />
            </div>
            {codeSent ? (
              <div className="grid gap-2">
                <Label htmlFor="code">Verification code</Label>
                <Input
                  id="code"
                  name="code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  minLength={6}
                  maxLength={6}
                  required
                  autoFocus
                  disabled={pending}
                  value={code}
                  onChange={(event) => setCode(event.target.value)}
                />
              </div>
            ) : null}
            {error ? (
              <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-body text-destructive">
                {error}
              </p>
            ) : null}
            <Button type="submit" disabled={pending || !email}>
              {pending ? "Please wait…" : codeSent ? "Verify and continue" : "Send code"}
            </Button>
            {autoLogin.enabled ? (
              <Button
                type="button"
                variant="ghost"
                disabled={pending}
                onClick={() => {
                  autoLoginAttempted.current = false;
                  workspaceEntryAttempted.current = false;
                  setManualMode(false);
                  setCodeSent(false);
                  setError(null);
                  setPending(true);
                }}
              >
                Retry auto-login
              </Button>
            ) : null}
            {codeSent ? (
              <Button
                type="button"
                variant="ghost"
                disabled={pending}
                onClick={() => {
                  setCodeSent(false);
                  setError(null);
                }}
              >
                Use another email
              </Button>
            ) : null}
          </form>
        )}
      </div>
    </main>
  );
}
