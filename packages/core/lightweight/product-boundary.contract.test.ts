import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const repoRoot = resolve(process.cwd(), "../..");

function read(relativePath: string): string {
  return readFileSync(resolve(repoRoot, relativePath), "utf8");
}

describe("Lightweight product boundary contract", () => {
  it("allows only the Tool Control Plane without raw MCP config or exited integrations", () => {
    const source = read("packages/core/lightweight/api.ts").toLowerCase();
    expect(source).toContain("/api/tool-sources");
    expect(source).toContain("/api/agents/${id}/tool-bundle");
    for (const forbidden of [
      "/api/integrations",
      "/api/lark",
      "/api/slack",
      "member_type",
      "human_member",
      "mcp_config",
      "autopilot",
      "billing",
      "inbox",
      "attachment",
    ]) {
      expect(source, forbidden).not.toContain(forbidden);
    }
  });

  it("keeps Gateway execution dependencies out of target frontend packages", () => {
    for (const manifest of [
      "packages/core/package.json",
      "packages/views/package.json",
      "apps/web/package.json",
    ]) {
      const source = read(manifest).toLowerCase();
      for (const forbidden of ["@modelcontextprotocol", "kin-openapi", "grpc-go", "protocompile"]) {
        expect(source, `${manifest}: ${forbidden}`).not.toContain(forbidden);
      }
    }
  });

  it("keeps agent detail routing from resurrecting retired MCP/integrations views", async () => {
    const { parseAgentDetailViewState } = await import("./agents/detail-view");
    expect(parseAgentDetailViewState(new URLSearchParams("view=mcp")).view).toBe("overview");
    expect(parseAgentDetailViewState(new URLSearchParams("view=integrations")).view).toBe("overview");
  });
});
