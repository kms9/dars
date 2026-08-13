"use client";

import { createContext, use, useCallback, useEffect, useState, useSyncExternalStore, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import type { StoreApi, UseBoundStore } from "zustand";
import { WSClient } from "../api/ws-client";
import type { WSEventType, StorageAdapter } from "../types";
import type { AuthState } from "../auth/store";
import type { ClientIdentity } from "../platform/types";
import { getCurrentSlug, getCurrentWsId, subscribeToCurrentSlug } from "../platform/workspace-storage";
import { createLogger } from "../logger";

type EventHandler = (payload: unknown, actorId?: string, actorType?: string) => void;
interface WSContextValue {
  subscribe: (event: WSEventType, handler: EventHandler) => () => void;
  onReconnect: (callback: () => void) => () => void;
}
const WSContext = createContext<WSContextValue | null>(null);

export interface WSProviderProps {
  children: ReactNode;
  wsUrl: string;
  authStore: UseBoundStore<StoreApi<AuthState>>;
  storage: StorageAdapter;
  cookieAuth?: boolean;
  identity?: ClientIdentity;
}

export function WSProvider({ children, wsUrl, authStore, storage, cookieAuth, identity }: WSProviderProps) {
  const user = authStore((state) => state.user);
  const slug = useSyncExternalStore(subscribeToCurrentSlug, getCurrentSlug, () => null);
  const queryClient = useQueryClient();
  const [client, setClient] = useState<WSClient | null>(null);

  useEffect(() => {
    const workspaceID = getCurrentWsId();
    if (!user || !slug || !workspaceID) return;
    const token = cookieAuth ? null : storage.getItem("dars_token");
    if (!cookieAuth && !token) return;
    const next = new WSClient(wsUrl, { logger: createLogger("ws"), cookieAuth, identity });
    next.setAuth(token, workspaceID);
    const invalidate = () => void queryClient.invalidateQueries();
    const unsubscribeAny = next.onAny(invalidate);
    const unsubscribeReconnect = next.onReconnect(invalidate);
    next.connect();
    setClient(next);
    return () => {
      unsubscribeAny();
      unsubscribeReconnect();
      next.disconnect();
      setClient(null);
    };
  }, [cookieAuth, identity, queryClient, slug, storage, user, wsUrl]);

  const subscribe = useCallback((event: WSEventType, handler: EventHandler) => client?.on(event, handler) ?? (() => undefined), [client]);
  const onReconnect = useCallback((callback: () => void) => client?.onReconnect(callback) ?? (() => undefined), [client]);
  return <WSContext.Provider value={{ subscribe, onReconnect }}>{children}</WSContext.Provider>;
}

export function useWS(): WSContextValue {
  const context = use(WSContext);
  if (!context) throw new Error("useWS must be used within WSProvider");
  return context;
}
