import type { User, Workspace } from "../types";
import type { Logger } from "../logger";
import { noopLogger } from "../logger";
import { createRequestId } from "../utils";
import { getCurrentWsId } from "../platform/workspace-storage";

export interface ApiClientIdentity {
  platform?: string;
  version?: string;
  os?: string;
}

export interface ApiClientOptions {
  logger?: Logger;
  onUnauthorized?: () => void;
  identity?: ApiClientIdentity;
}

export interface LoginResponse {
  token: string;
  user: User;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly statusText: string,
    readonly body?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export class ApiClient {
  private token: string | null = null;
  private readonly logger: Logger;

  constructor(
    private readonly baseUrl: string,
    private readonly options: ApiClientOptions = {},
  ) {
    this.logger = options.logger ?? noopLogger;
  }

  getBaseUrl(): string {
    return this.baseUrl;
  }

  setToken(token: string | null): void {
    this.token = token;
  }

  private readCsrfToken(): string | null {
    if (typeof document === "undefined") return null;
    const value = document.cookie.split("; ").find((item) => item.startsWith("dars_csrf="));
    return value?.split("=")[1] ?? null;
  }

  private headers(init?: RequestInit): Record<string, string> {
    const headers: Record<string, string> = {
      "X-Request-ID": createRequestId(),
      ...((init?.headers as Record<string, string> | undefined) ?? {}),
    };
    // The server rejects a JSON mutation that omits `application/json`, including
    // action routes such as logout or archive that carry an empty body.
    // FormData must keep the browser-generated multipart boundary; do not force JSON.
    const method = (init?.method ?? "GET").toUpperCase();
    const jsonMutation = method === "POST" || method === "PUT" || method === "PATCH";
    const isFormData = typeof FormData !== "undefined" && init?.body instanceof FormData;
    if (!isFormData && (jsonMutation || init?.body !== undefined)) {
      headers["Content-Type"] ??= "application/json";
    }
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    const workspaceID = getCurrentWsId();
    if (workspaceID) headers["X-Workspace-ID"] = workspaceID;
    const csrf = this.readCsrfToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
    if (this.options.identity?.platform) headers["X-Client-Platform"] = this.options.identity.platform;
    if (this.options.identity?.version) headers["X-Client-Version"] = this.options.identity.version;
    if (this.options.identity?.os) headers["X-Client-OS"] = this.options.identity.os;
    return headers;
  }

  async request<T>(path: string, init?: RequestInit): Promise<T> {
    const method = init?.method ?? "GET";
    const started = Date.now();
    const response = await fetch(`${this.baseUrl}${path}`, {
      ...init,
      headers: this.headers(init),
      credentials: "include",
    });
    if (!response.ok) {
      if (response.status === 401) {
        this.token = null;
        this.options.onUnauthorized?.();
      }
      let body: unknown;
      let message = `API error: ${response.status} ${response.statusText}`;
      try {
        body = await response.json();
        if (body && typeof body === "object") {
          const error = (body as { error?: unknown; code?: unknown }).error;
          const code = (body as { code?: unknown }).code;
          if (typeof error === "string" && error) message = error;
          else if (error && typeof error === "object") {
            const detail = error as { message?: unknown; code?: unknown };
            if (typeof detail.message === "string" && detail.message) message = detail.message;
            else if (typeof detail.code === "string" && detail.code) message = detail.code;
          } else if (typeof code === "string" && code) message = code;
        }
      } catch {
        // Non-JSON error responses retain the status-based fallback.
      }
      this.logger.error(`← ${response.status} ${method} ${path}`, { duration_ms: Date.now() - started });
      throw new ApiError(message, response.status, response.statusText, body);
    }
    this.logger.debug(`← ${response.status} ${method} ${path}`, { duration_ms: Date.now() - started });
    const text = await response.text();
    return (text ? JSON.parse(text) : undefined) as T;
  }

  sendCode(email: string): Promise<void> {
    return this.request("/auth/send-code", { method: "POST", body: JSON.stringify({ email }) });
  }

  verifyCode(email: string, code: string): Promise<LoginResponse> {
    return this.request("/auth/verify-code", { method: "POST", body: JSON.stringify({ email, code }) });
  }

  logout(): Promise<void> {
    return this.request("/auth/logout", { method: "POST" });
  }

  getMe(): Promise<User> {
    return this.request("/api/me");
  }

  listWorkspaces(): Promise<Workspace[]> {
    return this.request("/api/workspaces");
  }
}
