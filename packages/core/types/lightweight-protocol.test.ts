import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import {
  LIGHTWEIGHT_DAEMON_PROTOCOL_VERSION,
  LIGHTWEIGHT_WEB_SOCKET_PATH,
  LIGHTWEIGHT_WEB_SOCKET_WORKSPACE_PARAM,
  LIGHTWEIGHT_WS_EVENT_TYPES,
  isLightweightWSEventType,
} from "./lightweight-protocol";

const contractRoot = new URL(
  "../../../openspec/contracts/",
  import.meta.url,
);

function contractLines(name: string): string[] {
  return readFileSync(new URL(name, contractRoot), "utf8")
    .split(/\r?\n/u)
    .map((line) => line.trim())
    .filter(Boolean)
    .sort();
}

describe("Lightweight Web protocol", () => {
  it("matches the frozen Web event allowlist exactly", () => {
    expect([...LIGHTWEIGHT_WS_EVENT_TYPES].sort()).toEqual(
      contractLines("web-events.txt"),
    );
    expect(new Set(LIGHTWEIGHT_WS_EVENT_TYPES).size).toBe(
      LIGHTWEIGHT_WS_EVENT_TYPES.length,
    );
  });

  it("uses only the breaking-cutover version and workspace-id WS path", () => {
    expect(LIGHTWEIGHT_DAEMON_PROTOCOL_VERSION).toBe("lightweight-runtime-v1");
    expect(LIGHTWEIGHT_WEB_SOCKET_PATH).toBe("/ws");
    expect(LIGHTWEIGHT_WEB_SOCKET_WORKSPACE_PARAM).toBe("workspace_id");
    expect(isLightweightWSEventType("task:running")).toBe(true);
    expect(isLightweightWSEventType("inbox:new")).toBe(false);
    expect(isLightweightWSEventType("chat:quick_actions")).toBe(false);
  });
});
