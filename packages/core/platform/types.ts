import type { StorageAdapter } from "../types";

export interface ClientIdentity {
  platform?: string;
  version?: string;
  os?: string;
}

export interface CoreProviderProps {
  children: React.ReactNode;
  apiBaseUrl?: string;
  wsUrl?: string;
  storage?: StorageAdapter;
  cookieAuth?: boolean;
  onLogin?: () => void;
  onLogout?: () => void;
  identity?: ClientIdentity;
}
