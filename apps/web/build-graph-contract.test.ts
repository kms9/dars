import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";

import { describe, expect, it } from "vitest";

const repoRoot = resolve(process.cwd(), "../..");
const targetPackages = [
  "@dars/web",
  "@dars/core",
  "@dars/ui",
  "@dars/views",
] as const;
const excludedApps = [
  "@dars/desktop",
  "@dars/mobile",
  "@dars/docs",
] as const;

type PackageManifest = {
  name: string;
  scripts?: Record<string, string>;
  dependencies?: Record<string, string>;
  devDependencies?: Record<string, string>;
  peerDependencies?: Record<string, string>;
  optionalDependencies?: Record<string, string>;
};

function readJson<T>(relativePath: string): T {
  return JSON.parse(readFileSync(resolve(repoRoot, relativePath), "utf8")) as T;
}

function workspaceManifests() {
  const manifests = new Map<string, { path: string; manifest: PackageManifest }>();

  for (const root of ["apps", "packages"] as const) {
    for (const entry of readdirSync(resolve(repoRoot, root), { withFileTypes: true })) {
      if (!entry.isDirectory()) continue;
      const path = `${root}/${entry.name}/package.json`;
      const manifest = readJson<PackageManifest>(path);
      manifests.set(manifest.name, { path, manifest });
    }
  }

  return manifests;
}

describe("Lightweight target build graph", () => {
  it("uses an exact positive package set for every root Turbo command", () => {
    const root = readJson<PackageManifest>("package.json");
    const filters = targetPackages.map((name) => `--filter=${name}`).join(" ");

    for (const command of ["build", "typecheck", "test", "lint"] as const) {
      expect(root.scripts?.[command]).toBe(`turbo ${command} ${filters}`);
      expect(root.scripts?.[command]).not.toContain("--filter=!");
      for (const excluded of excludedApps) {
        expect(root.scripts?.[command]).not.toContain(excluded);
      }
    }
  });

  it("cannot reach a non-target app through workspace package dependencies", () => {
    const manifests = workspaceManifests();
    const pending = [...targetPackages];
    const reachable = new Set<string>();

    while (pending.length > 0) {
      const name = pending.pop();
      if (!name || reachable.has(name)) continue;
      reachable.add(name);

      const record = manifests.get(name);
      expect(record, `missing workspace package ${name}`).toBeDefined();
      if (!record) continue;

      const dependencies = {
        ...record.manifest.dependencies,
        ...record.manifest.devDependencies,
        ...record.manifest.peerDependencies,
        ...record.manifest.optionalDependencies,
      };
      for (const dependency of Object.keys(dependencies)) {
        if (manifests.has(dependency) && !reachable.has(dependency)) {
          pending.push(dependency as (typeof targetPackages)[number]);
        }
      }
    }

    const reachableApps = [...reachable]
      .filter((name) => manifests.get(name)?.path.startsWith("apps/"))
      .sort();
    expect(reachableApps).toEqual(["@dars/web"]);
  });

  it("keeps make check on the target commands and an isolated database", () => {
    const makefile = readFileSync(resolve(repoRoot, "Makefile"), "utf8");
    const check = readFileSync(resolve(repoRoot, "scripts/check.sh"), "utf8");

    expect(makefile).toMatch(
      /^check: ## Run the full target pipeline against an isolated fresh Lightweight check database$/m,
    );
    expect(check).toContain("pnpm typecheck");
    expect(check).toContain("pnpm build");
    expect(check).toContain("pnpm test");
    expect(check).toContain("bash scripts/test-go.sh");
    expect(check).toContain("pnpm exec playwright test");
    expect(check).toContain("export DARS_EDITION=lightweight");
    expect(check).toContain('export LIGHTWEIGHT_DATABASE_URL="$CHECK_DATABASE_URL"');
    expect(check).toContain('export EXPECTED_DATABASE_NAME="$CHECK_DATABASE_NAME"');
    expect(check).toContain("export DARS_AGENT_SECRET_KEY=");
    expect(check).toContain("dars_lightweight_check_");
    expect(check).toContain('kill -TERM -- "-$process_group"');
    expect(check).toContain('BACKEND_PGID="$(ps -o pgid=');
    expect(check).toContain('FRONTEND_PGID="$(ps -o pgid=');
    for (const excluded of excludedApps) {
      expect(check).not.toContain(excluded);
    }
  });

  it("pins every runtime database entrypoint to the Lightweight identity", () => {
    const expectedFiles = [
      ".env.example",
      "Makefile",
      "docker-compose.yml",
      "docker-compose.selfhost.yml",
      "scripts/ensure-postgres.sh",
      "scripts/local-env.sh",
      "scripts/init-worktree-env.sh",
      "scripts/check.sh",
      ".github/workflows/ci.yml",
    ];

    for (const path of expectedFiles) {
      const source = readFileSync(resolve(repoRoot, path), "utf8");
      expect(source, path).toContain("dars_lightweight");
    }

    const envExample = readFileSync(resolve(repoRoot, ".env.example"), "utf8");
    expect(envExample).toContain("EXPECTED_DATABASE_NAME=dars_lightweight");
    expect(envExample).toContain("DARS_EDITION=lightweight");
    expect(envExample).toContain("DATABASE_MAX_CONNS=25");
    expect(envExample).toContain("DATABASE_MIN_CONNS=5");
    expect(envExample).toContain("DATABASE_MAX_IDLE_CONNS=5");
    expect(envExample).toContain("DATABASE_MAX_CONN_LIFETIME=300s");
    expect(envExample).toMatch(/^DARS_AGENT_SECRET_KEY=$/m);

    const makefile = readFileSync(resolve(repoRoot, "Makefile"), "utf8");
    expect(makefile).toContain("^dars_lightweight(_[a-z0-9_]+)?$$");
    expect(makefile).toContain(
      "Refusing to reset: DATABASE_URL database does not match POSTGRES_DB.",
    );

    const ensurePostgres = readFileSync(
      resolve(repoRoot, "scripts/ensure-postgres.sh"),
      "utf8",
    );
    expect(ensurePostgres).toContain(
      "Refusing database outside the Lightweight name allowlist",
    );
    expect(ensurePostgres).toContain(
      "does not match EXPECTED_DATABASE_NAME",
    );
    expect(ensurePostgres).not.toContain('pg_isready -d "$DATABASE_URL"');
  });

  it("keeps required CI positive-filtered and non-target apps independent", () => {
    const ci = readFileSync(resolve(repoRoot, ".github/workflows/ci.yml"), "utf8");
    const filters = [...ci.matchAll(/--filter=(@dars\/[a-z-]+)/g)].map(
      (match) => match[1],
    );

    expect([...new Set(filters)].sort()).toEqual([...targetPackages].sort());
    expect(ci).not.toContain("--filter=!");
    expect(ci).not.toContain("- 'apps/desktop/**'");
    expect(ci).not.toContain("- 'apps/mobile/**'");
    expect(ci).not.toContain("- 'apps/docs/**'");

    const desktop = readFileSync(
      resolve(repoRoot, ".github/workflows/desktop-smoke.yml"),
      "utf8",
    );
    const mobile = readFileSync(
      resolve(repoRoot, ".github/workflows/mobile-verify.yml"),
      "utf8",
    );
    const docs = readFileSync(
      resolve(repoRoot, ".github/workflows/docs-smoke.yml"),
      "utf8",
    );

    expect(desktop).toMatch(/on:\n\s+workflow_dispatch:/);
    expect(mobile).toContain("name: Mobile Verify");
    expect(docs).toMatch(/on:\n\s+workflow_dispatch:/);
  });

  it("keeps desktop outside the target web dependency graph", () => {
    const manifests = workspaceManifests();
    const pending = ["@dars/web"];
    const reachable = new Set<string>();
    while (pending.length > 0) {
      const name = pending.pop();
      if (!name || reachable.has(name)) continue;
      reachable.add(name);
      const record = manifests.get(name);
      if (!record) continue;
      const dependencies = {
        ...record.manifest.dependencies,
        ...record.manifest.devDependencies,
        ...record.manifest.peerDependencies,
        ...record.manifest.optionalDependencies,
      };
      for (const dependency of Object.keys(dependencies)) {
        if (manifests.has(dependency) && !reachable.has(dependency)) {
          pending.push(dependency);
        }
      }
    }
    expect(reachable.has("@dars/desktop")).toBe(false);
    expect([...reachable].filter((name) => name.startsWith("@dars/"))).not.toContain("@dars/desktop");
  });

  it("does not wire agent/squad management changes through desktop sources", () => {
    const desktopPackage = readJson<{ scripts?: Record<string, string> }>("apps/desktop/package.json");
    expect(desktopPackage.scripts?.build).toBeDefined();
    const root = readJson<{ scripts?: Record<string, string> }>("package.json");
    for (const command of ["build", "typecheck", "test", "lint"] as const) {
      expect(root.scripts?.[command]).not.toContain("@dars/desktop");
    }
  });
});
