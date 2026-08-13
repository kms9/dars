"use client";

import { useMemo } from "react";
import { CoreProvider } from "@dars/core/platform";
import packageJson from "../package.json";
import { WebNavigationProvider } from "@/platform/navigation";
import { setLoggedInCookie, clearLoggedInCookie } from "@/features/auth/auth-cookie";
import { detectWebOS } from "@/platform/client-os";

function deriveWsUrl(): string | undefined {
  if (typeof window === "undefined") return undefined;
  return `${window.location.protocol === "https:" ? "wss:" : "ws:"}//${window.location.host}/ws`;
}

const WEB_VERSION = process.env.NEXT_PUBLIC_APP_VERSION || packageJson.version || "dev";

export function WebProviders({ children, apiBaseUrl, wsUrl }: { children: React.ReactNode; apiBaseUrl?: string; wsUrl?: string }) {
  const identity = useMemo(() => ({ platform: "web", version: WEB_VERSION, os: detectWebOS() }), []);
  return (
    <CoreProvider
      apiBaseUrl={apiBaseUrl}
      wsUrl={wsUrl || deriveWsUrl()}
      cookieAuth
      onLogin={setLoggedInCookie}
      onLogout={clearLoggedInCookie}
      identity={identity}
    >
      <WebNavigationProvider>{children}</WebNavigationProvider>
    </CoreProvider>
  );
}
