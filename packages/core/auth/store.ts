import { create } from "zustand";
import type { User, StorageAdapter } from "../types";
import { ApiError, type ApiClient } from "../api/client";
import { setCurrentWorkspace } from "../platform/workspace-storage";

export interface AuthStoreOptions {
  api: ApiClient;
  storage: StorageAdapter;
  onLogin?: () => void;
  onLogout?: () => void;
  cookieAuth?: boolean;
}

export interface AuthState {
  user: User | null;
  isLoading: boolean;
  initialize: () => Promise<void>;
  sendCode: (email: string) => Promise<void>;
  verifyCode: (email: string, code: string) => Promise<User>;
  logout: () => void;
  setUser: (user: User | null) => void;
  refreshMe: () => Promise<void>;
}

export function createAuthStore({ api, storage, onLogin, onLogout, cookieAuth }: AuthStoreOptions) {
  return create<AuthState>((set) => ({
    user: null,
    isLoading: true,
    initialize: async () => {
      const token = cookieAuth ? null : storage.getItem("dars_token");
      if (!cookieAuth && !token) {
        set({ user: null, isLoading: false });
        return;
      }
      if (token) api.setToken(token);
      try {
        set({ user: await api.getMe(), isLoading: false });
      } catch (error) {
        if (error instanceof ApiError && error.status === 401) setCurrentWorkspace(null, null);
        set({ user: null, isLoading: false });
      }
    },
    sendCode: (email) => api.sendCode(email),
    verifyCode: async (email, code) => {
      const { token, user } = await api.verifyCode(email, code);
      if (!cookieAuth) {
        storage.setItem("dars_token", token);
        api.setToken(token);
      }
      onLogin?.();
      set({ user, isLoading: false });
      return user;
    },
    logout: () => {
      if (cookieAuth) void api.logout().catch(() => undefined);
      storage.removeItem("dars_token");
      api.setToken(null);
      setCurrentWorkspace(null, null);
      onLogout?.();
      set({ user: null, isLoading: false });
    },
    setUser: (user) => set({ user, isLoading: false }),
    refreshMe: async () => set({ user: await api.getMe(), isLoading: false }),
  }));
}
