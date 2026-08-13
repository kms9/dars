import { readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const appRoot = resolve(process.cwd(), "app");

function pages(dir: string, found: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) pages(path, found);
    else if (entry === "page.tsx") found.push(relative(appRoot, path));
  }
  return found;
}

describe("Lightweight Web route tree", () => {
  it("contains only the approved Lightweight pages including the Tools control plane", () => {
    expect(pages(appRoot).sort()).toEqual([
      "(auth)/login/page.tsx",
      "(auth)/workspaces/new/page.tsx",
      "(landing)/page.tsx",
      "[workspaceSlug]/(dashboard)/agents/[id]/page.tsx",
      "[workspaceSlug]/(dashboard)/agents/new/ai/[sessionId]/page.tsx",
      "[workspaceSlug]/(dashboard)/agents/new/ai/page.tsx",
      "[workspaceSlug]/(dashboard)/agents/new/blank/page.tsx",
      "[workspaceSlug]/(dashboard)/agents/new/page.tsx",
      "[workspaceSlug]/(dashboard)/agents/page.tsx",
      "[workspaceSlug]/(dashboard)/chat/page.tsx",
      "[workspaceSlug]/(dashboard)/issues/[id]/page.tsx",
      "[workspaceSlug]/(dashboard)/issues/page.tsx",
      "[workspaceSlug]/(dashboard)/runtimes/[id]/page.tsx",
      "[workspaceSlug]/(dashboard)/runtimes/page.tsx",
      "[workspaceSlug]/(dashboard)/settings/page.tsx",
      "[workspaceSlug]/(dashboard)/skills/[id]/page.tsx",
      "[workspaceSlug]/(dashboard)/skills/page.tsx",
      "[workspaceSlug]/(dashboard)/squads/[id]/page.tsx",
      "[workspaceSlug]/(dashboard)/squads/page.tsx",
      "[workspaceSlug]/(dashboard)/tools/[id]/page.tsx",
      "[workspaceSlug]/(dashboard)/tools/page.tsx",
    ]);
  });
});
