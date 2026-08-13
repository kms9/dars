import { describe, expect, it } from "vitest";

import {
  resolveBrowserApiBaseUrl,
  resolveBrowserWsUrl,
  resolveDevRemoteApiUrl,
  resolveRemoteApiUrl,
  runtimeRewriteDestination,
} from "./runtime-urls";

describe("resolveRemoteApiUrl", () => {
  it("prefers REMOTE_API_URL when explicitly configured", () => {
    expect(
      resolveRemoteApiUrl({
        REMOTE_API_URL: "http://backend:8080",
        NEXT_PUBLIC_API_URL: "http://localhost:19000",
        PORT: "18080",
      }),
    ).toBe("http://backend:8080");
  });

  it("uses NEXT_PUBLIC_API_URL when REMOTE_API_URL is unset", () => {
    expect(
      resolveRemoteApiUrl({
        NEXT_PUBLIC_API_URL: "http://localhost:19000",
        PORT: "18080",
      }),
    ).toBe("http://localhost:19000");
  });

  it("does not infer a backend URL from frontend or backend port env vars", () => {
    expect(
      resolveRemoteApiUrl({
        BACKEND_PORT: "28080",
        API_PORT: "38080",
        SERVER_PORT: "48080",
        PORT: "3000",
      }),
    ).toBeUndefined();
    expect(resolveRemoteApiUrl({ PORT: "3000" })).toBeUndefined();
  });

  it("does not use relative public API URLs for server-side rewrites", () => {
    expect(
      resolveRemoteApiUrl({
        NEXT_PUBLIC_API_URL: "/api",
      }),
    ).toBeUndefined();
  });

  it("ignores whitespace-only or invalid backend URL values", () => {
    expect(
      resolveRemoteApiUrl({
        REMOTE_API_URL: "  ",
        NEXT_PUBLIC_API_URL: "ftp://api.example.com",
        PORT: "19080",
      }),
    ).toBeUndefined();
  });

  it("returns undefined when no API origin is configured", () => {
    expect(resolveRemoteApiUrl({})).toBeUndefined();
  });
});

describe("browser runtime URLs", () => {
  it("exposes NEXT_PUBLIC_API_URL at server render time", () => {
    expect(
      resolveBrowserApiBaseUrl({
        NEXT_PUBLIC_API_URL: " https://api.example.com/ ",
      }),
    ).toBe("https://api.example.com");
  });

  it("derives browser websocket URL from the public API URL", () => {
    expect(
      resolveBrowserWsUrl({
        NEXT_PUBLIC_API_URL: "https://api.example.com/base",
      }),
    ).toBe("wss://api.example.com/base/ws");
  });

  it("prefers an explicit browser websocket URL", () => {
    expect(
      resolveBrowserWsUrl({
        NEXT_PUBLIC_API_URL: "https://api.example.com",
        NEXT_PUBLIC_WS_URL: " wss://ws.example.com/socket/ ",
      }),
    ).toBe("wss://ws.example.com/socket");
  });

  it("falls back to same-origin websocket derivation for relative public API URLs", () => {
    expect(
      resolveBrowserWsUrl({
        NEXT_PUBLIC_API_URL: "/api",
      }),
    ).toBeUndefined();
  });
});

describe("runtimeRewriteDestination", () => {
  it("keeps same-origin fallback when no runtime upstreams are configured", () => {
    expect(runtimeRewriteDestination("/api/config", {})).toBeUndefined();
    expect(runtimeRewriteDestination("/auth/send-code", {})).toBeUndefined();
    expect(runtimeRewriteDestination("/ws", {})).toBeUndefined();
  });

  it("keeps same-origin fallback for runtime API paths when only frontend PORT is configured", () => {
    expect(
      runtimeRewriteDestination("/api/config", {
        PORT: "3000",
      }),
    ).toBeUndefined();
  });

  it("does not rewrite runtime API paths to relative public API URLs", () => {
    expect(
      runtimeRewriteDestination("/api/config", {
        NEXT_PUBLIC_API_URL: "/api",
      }),
    ).toBeUndefined();
  });

  it("maps backend HTTP paths to the runtime API origin", () => {
    expect(
      runtimeRewriteDestination("/api/config", {
        REMOTE_API_URL: "http://backend:8080",
      }),
    ).toBe("http://backend:8080/api/config");
    expect(
      runtimeRewriteDestination("/auth/send-code", {
        REMOTE_API_URL: "http://backend:8080",
      }),
    ).toBe("http://backend:8080/auth/send-code");
  });

  it("maps websocket paths to the runtime API origin", () => {
    expect(
      runtimeRewriteDestination("/ws", {
        REMOTE_API_URL: "http://backend:8080",
      }),
    ).toBe("http://backend:8080/ws");
  });
});

describe("dev-only fallbacks", () => {
  it("falls back to the conventional local backend port", () => {
    expect(resolveDevRemoteApiUrl({})).toBe("http://localhost:8080");
  });

  it("honors BACKEND_PORT for the dev backend fallback", () => {
    expect(resolveDevRemoteApiUrl({ BACKEND_PORT: "19080" })).toBe(
      "http://localhost:19080",
    );
  });

  it("ignores the frontend process PORT while honoring backend-specific aliases", () => {
    expect(resolveDevRemoteApiUrl({ PORT: "3000" })).toBe(
      "http://localhost:8080",
    );
    expect(resolveDevRemoteApiUrl({ API_PORT: "19082", PORT: "3000" })).toBe(
      "http://localhost:19082",
    );
    expect(
      resolveDevRemoteApiUrl({ SERVER_PORT: "19083", PORT: "3000" }),
    ).toBe("http://localhost:19083");
    expect(
      resolveDevRemoteApiUrl({ BACKEND_PORT: "19080", PORT: "3000" }),
    ).toBe("http://localhost:19080");
  });

  it("prefers a configured origin over the dev fallback", () => {
    expect(
      resolveDevRemoteApiUrl({ REMOTE_API_URL: "http://backend:8080" }),
    ).toBe("http://backend:8080");
  });
});
