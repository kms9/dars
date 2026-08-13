export { ApiClient, ApiError } from "./client";
export type { ApiClientIdentity, ApiClientOptions, LoginResponse } from "./client";
export { WSClient } from "./ws-client";
export { parseWithFallback } from "./schema";

import type { ApiClient } from "./client";

let instance: ApiClient | null = null;

export function setApiInstance(next: ApiClient): void {
  instance = next;
}

export function getApi(): ApiClient {
  if (!instance) throw new Error("ApiClient not initialised — call setApiInstance() first");
  return instance;
}

export const api = new Proxy({} as ApiClient, {
  get(_target, property, receiver) {
    if (!instance) return undefined;
    const value = Reflect.get(instance, property, receiver) as unknown;
    return typeof value === "function" ? value.bind(instance) : value;
  },
});
