import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";
import { proxy } from "./proxy";

function request(path: string, cookies: Record<string, string> = {}) {
  const cookie = Object.entries(cookies).map(([key, value]) => `${key}=${value}`).join("; ");
  return new NextRequest(`https://app.dars.test${path}`, { headers: cookie ? { cookie } : undefined });
}

describe("Lightweight proxy", () => {
  it("redirects an authenticated root request to the last workspace", () => {
    const response = proxy(request("/", { dars_logged_in: "1", last_workspace_slug: "acme" }));
    expect(response.status).toBe(307);
    expect(response.headers.get("location")).toBe("https://app.dars.test/acme/issues");
  });

  it("does not restore retired routes", () => {
    for (const path of ["/projects", "/inbox", "/autopilots", "/usage"]) {
      expect(proxy(request(path)).headers.get("location")).toBeNull();
    }
  });

  it("rewrites API and WebSocket requests only when an upstream is configured", () => {
    const previous = process.env.REMOTE_API_URL;
    process.env.REMOTE_API_URL = "http://backend:8080";
    try {
      expect(proxy(request("/api/me?x=1")).headers.get("x-middleware-rewrite")).toBe("http://backend:8080/api/me?x=1");
      expect(proxy(request("/ws")).headers.get("x-middleware-rewrite")).toBe("http://backend:8080/ws");
    } finally {
      if (previous === undefined) delete process.env.REMOTE_API_URL;
      else process.env.REMOTE_API_URL = previous;
    }
  });
});
