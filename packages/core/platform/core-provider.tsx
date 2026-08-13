"use client";

import { useMemo } from "react";
import { ApiClient, setApiInstance } from "../api";
import { createAuthStore, registerAuthStore } from "../auth";
import { createLogger } from "../logger";
import { QueryProvider } from "../provider";
import { WSProvider } from "../realtime";
import { defaultStorage } from "./storage";
import { AuthInitializer } from "./auth-initializer";
import type { CoreProviderProps } from "./types";

let initialized = false;
let authStore: ReturnType<typeof createAuthStore>;

export function CoreProvider({
  children,
  apiBaseUrl = "",
  wsUrl = "ws://localhost:8080/ws",
  storage = defaultStorage,
  cookieAuth,
  onLogin,
  onLogout,
  identity,
}: CoreProviderProps) {
  useMemo(() => {
    if (initialized) return;
    const api = new ApiClient(apiBaseUrl, {
      logger: createLogger("api"),
      onUnauthorized: () => storage.removeItem("dars_token"),
      identity,
    });
    setApiInstance(api);
    if (!cookieAuth) {
      const token = storage.getItem("dars_token");
      if (token) api.setToken(token);
    }
    authStore = createAuthStore({ api, storage, onLogin, onLogout, cookieAuth });
    registerAuthStore(authStore);
    initialized = true;
  }, [apiBaseUrl, cookieAuth, identity, onLogin, onLogout, storage]);

  return (
    <QueryProvider>
      <AuthInitializer storage={storage} cookieAuth={cookieAuth} onLogin={onLogin} onLogout={onLogout}>
        <WSProvider wsUrl={wsUrl} authStore={authStore} storage={storage} cookieAuth={cookieAuth} identity={identity}>
          {children}
        </WSProvider>
      </AuthInitializer>
    </QueryProvider>
  );
}
