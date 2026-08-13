"use client";

import { useEffect, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getApi } from "../api";
import { useAuthStore } from "../auth";
import { workspaceKeys } from "../workspace";
import type { StorageAdapter } from "../types";

export function AuthInitializer({
  children,
  onLogin,
  onLogout,
  storage,
  cookieAuth,
}: {
  children: ReactNode;
  onLogin?: () => void;
  onLogout?: () => void;
  storage: StorageAdapter;
  cookieAuth?: boolean;
}) {
  const queryClient = useQueryClient();

  useEffect(() => {
    let disposed = false;
    const api = getApi();
    const token = cookieAuth ? null : storage.getItem("dars_token");
    if (!cookieAuth && !token) {
      onLogout?.();
      useAuthStore.setState({ user: null, isLoading: false });
      return;
    }
    if (token) api.setToken(token);
    Promise.all([api.getMe(), api.listWorkspaces()])
      .then(([user, workspaces]) => {
        if (disposed) return;
        onLogin?.();
        useAuthStore.setState({ user, isLoading: false });
        queryClient.setQueryData(workspaceKeys.list(), workspaces);
      })
      .catch(() => {
        if (disposed) return;
        // A newer login may have completed while this anonymous bootstrap was
        // still in flight (React Strict Mode remounts). Never wipe that session
        // or clear the logged-in cookie.
        if (useAuthStore.getState().user) return;
        useAuthStore.setState({ user: null, isLoading: false });
      });
    return () => {
      disposed = true;
    };
  }, [cookieAuth, onLogin, onLogout, queryClient, storage]);

  return <>{children}</>;
}
