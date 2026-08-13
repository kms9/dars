import { readFileSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const repoRoot = resolve(process.cwd(), "../..");
const clientPath = resolve(repoRoot, "packages/core/lightweight/api.ts");
const manifestPath = resolve(
  repoRoot,
  "openspec/contracts/routes.txt",
);

function normalizePath(path: string): string {
  return path
    .replace(/\$\{[^}]+\}/g, "{}")
    .replace(/\{[^}]+\}/g, "{}")
    .split("?", 1)[0] ?? path;
}

describe("Lightweight API client contract", () => {
  const source = readFileSync(clientPath, "utf8");
  const manifestPaths = new Set(
    readFileSync(manifestPath, "utf8")
      .trim()
      .split("\n")
      .map((line) => normalizePath(line.split(" ", 2)[1] ?? "")),
  );

  it("keeps its complete HTTP path snapshot inside the frozen route manifest", () => {
    const paths = [
      ...new Set(
        [...source.matchAll(/["'`](\/(?:api|auth)\/[^"'`\s]*)["'`]/g)].map(
          (match) => normalizePath(match[1] ?? ""),
        ),
      ),
    ].sort();

    expect(paths).toEqual([
      "/api/agent-builder/sessions",
      "/api/agent-builder/sessions/{}/draft",
      "/api/agent-builder/sessions/{}/runtime",
      "/api/agents",
      "/api/agents/snapshot",
      "/api/agents/{}",
      "/api/agents/{}/archive",
      "/api/agents/{}/avatar",
      "/api/agents/{}/env",
      "/api/agents/{}/restore",
      "/api/agents/{}/skills",
      "/api/agents/{}/tasks",
      "/api/agents/{}/tasks/cancel",
      "/api/agents/{}/tool-bundle",
      "/api/chat/sessions",
      "/api/chat/sessions/{}",
      "/api/chat/sessions/{}/draft-restores",
      "/api/chat/sessions/{}/draft-restores/{}",
      "/api/chat/sessions/{}/messages",
      "/api/chat/sessions/{}/pending-task",
      "/api/issues",
      "/api/issues/{}",
      "/api/issues/{}/active-task",
      "/api/issues/{}/comments",
      "/api/issues/{}/task-runs",
      "/api/runtimes",
      "/api/runtimes/{}",
      "/api/runtimes/{}/local-skills",
      "/api/runtimes/{}/local-skills/import",
      "/api/runtimes/{}/local-skills/import/{}",
      "/api/runtimes/{}/local-skills/{}",
      "/api/skills",
      "/api/skills/import",
      "/api/skills/search",
      "/api/skills/{}",
      "/api/skills/{}/files",
      "/api/skills/{}/files/{}",
      "/api/squads",
      "/api/squads/{}",
      "/api/squads/{}/avatar",
      "/api/squads/{}/members",
      "/api/squads/{}/members/role",
      "/api/squads/{}/members/status",
      "/api/tasks/{}/cancel",
      "/api/tasks/{}/messages",
      "/api/tool-bundles/{}",
      "/api/tool-bundles/{}/revoke",
      "/api/tool-sources",
      "/api/tool-sources/import",
      "/api/tool-sources/{}",
      "/api/tool-sources/{}/disable",
      "/api/tool-sources/{}/enable",
      "/api/tool-sources/{}/tools",
      "/api/tool-sources/{}/validate",
      "/api/workspaces",
      "/api/workspaces/{}",
      "/api/workspaces/{}/members",
      "/api/workspaces/{}/runtime-profiles",
      "/api/workspaces/{}/runtime-profiles/{}",
    ]);

    for (const path of paths) {
      expect(manifestPaths, path).toContain(path);
    }
  });

  it("does not reintroduce exited product capabilities or old workspace context", () => {
    for (const forbidden of [
      "autopilot",
      "attachment",
      "billing",
      "channel",
      "inbox",
      "invitation",
      "label",
      "project",
      "property",
      "reaction",
      "subscriber",
      "X-Workspace-Slug",
    ]) {
      expect(source.toLowerCase()).not.toContain(forbidden.toLowerCase());
    }
  });
});
