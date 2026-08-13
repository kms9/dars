import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiClient } from "./client";
import { setCurrentWorkspace } from "../platform/workspace-storage";

afterEach(() => {
  vi.unstubAllGlobals();
  setCurrentWorkspace(null, null);
});

describe("Lightweight ApiClient", () => {
  it("sends only the server-resolved workspace id identity header", async () => {
    setCurrentWorkspace("ignored-slug", "workspace-1");
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ id: "user-1" }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new ApiClient("https://api.example", { identity: { platform: "web", version: "1", os: "macos" } });
    await client.getMe();
    const headers = fetchMock.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(headers["X-Workspace-ID"]).toBe("workspace-1");
    expect(headers["X-Workspace-Slug"]).toBeUndefined();
    expect(headers["X-Agent-ID"]).toBeUndefined();
    expect(headers["X-Task-ID"]).toBeUndefined();
    expect(headers["X-Client-Platform"]).toBe("web");
  });

  it("uses the frozen email login endpoints and cookie credentials", async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(null, { status: 202 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ token: "token", user: { id: "user-1" } }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new ApiClient("");
    await client.sendCode("person@example.com");
    await client.verifyCode("person@example.com", "123456");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/auth/send-code");
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/auth/verify-code");
    expect(fetchMock.mock.calls[0]?.[1]?.credentials).toBe("include");
  });

  it("declares application/json on mutations that carry no body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new ApiClient("");
    await client.logout();
    await client.getMe();
    const logoutHeaders = fetchMock.mock.calls[0]?.[1]?.headers as Record<string, string>;
    const readHeaders = fetchMock.mock.calls[1]?.[1]?.headers as Record<string, string>;
    expect(logoutHeaders["Content-Type"]).toBe("application/json");
    expect(readHeaders["Content-Type"]).toBeUndefined();
  });

  it("skips Content-Type when the body is FormData", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const client = new ApiClient("");
    const body = new FormData();
    body.append("file", new Blob(["x"]), "demo.skill");
    await client.request("/api/skills/import", { method: "POST", body });
    const headers = fetchMock.mock.calls[0]?.[1]?.headers as Record<string, string>;
    expect(headers["Content-Type"]).toBeUndefined();
  });

  it("surfaces structured failures and invokes the unauthorized boundary", async () => {
    const unauthorized = vi.fn();
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { code: "unauthorized", message: "sign in" } }), { status: 401, statusText: "Unauthorized" })));
    const client = new ApiClient("", { onUnauthorized: unauthorized });
    await expect(client.getMe()).rejects.toEqual(expect.objectContaining({ status: 401, message: "sign in" }));
    expect(unauthorized).toHaveBeenCalledOnce();
  });
});
