import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const repoRoot = resolve(process.cwd(), "../..");
const appRoot = resolve(process.cwd(), "app");

function walk(dir: string, found: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) walk(path, found);
    else found.push(relative(appRoot, path));
  }
  return found;
}

describe("Lightweight Web product boundary contract", () => {
  it("exposes only the approved Tools routes and no raw MCP or exited integration routes", () => {
    const pages = walk(appRoot).filter((path) => path.endsWith("page.tsx"));
    for (const page of pages) {
      expect(page).not.toMatch(/mcp|integration|lark|slack|human-member/i);
    }
    expect(pages).not.toContain("(dashboard)/autopilots/page.tsx");
    expect(pages).not.toContain("(dashboard)/billing/page.tsx");
    expect(pages).not.toContain("lark/bind/page.tsx");
    expect(pages).not.toContain("slack/bind/page.tsx");
    expect(pages).toContain("[workspaceSlug]/(dashboard)/tools/page.tsx");
    expect(pages).toContain("[workspaceSlug]/(dashboard)/tools/[id]/page.tsx");
  });

  it("does not import desktop-only packages from the target web app", () => {
    const manifest = JSON.parse(readFileSync(resolve(process.cwd(), "package.json"), "utf8")) as {
      dependencies?: Record<string, string>;
      devDependencies?: Record<string, string>;
    };
    const deps = { ...manifest.dependencies, ...manifest.devDependencies };
    expect(deps["@dars/desktop"]).toBeUndefined();
  });

  it("keeps agent detail retired view params out of shared views sources", () => {
    const source = readFileSync(resolve(repoRoot, "packages/views/lightweight/agents-detail.tsx"), "utf8");
    expect(source).not.toMatch(/view=mcp|view=integrations/);
  });
});
