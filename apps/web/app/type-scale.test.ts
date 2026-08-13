import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";

const repoRoot = resolve(process.cwd(), "../..");
const scale = [
  ["micro", 11, 15],
  ["caption", 12, 16],
  ["label", 13, 18],
  ["body", 14, 20],
  ["title-sm", 16, 24],
  ["title-lg", 20, 28],
  ["display-sm", 24, 32],
] as const;

const scanRoots = ["packages/ui", "packages/views/lightweight", "apps/web"];
const skipDirs = new Set(["node_modules", ".next", "dist", "out", "build", ".turbo"]);

function sourceFiles(dir: string, found: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (skipDirs.has(entry)) continue;
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) sourceFiles(path, found);
    else if (/\.(?:ts|tsx|css)$/.test(entry) && !/\.test\.tsx?$/.test(entry)) found.push(path);
  }
  return found;
}

function stripComments(source: string) {
  return source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/(^|[^:])\/\/[^\n]*/g, "$1");
}

describe("Lightweight type scale", () => {
  const tokens = readFileSync(resolve(repoRoot, "packages/ui/styles/tokens.css"), "utf8");

  it.each(scale)("defines --text-%s with a line height", (name, size, lineHeight) => {
    expect(tokens).toContain(`--text-${name}: ${size}px;`);
    expect(tokens).toContain(`--text-${name}--line-height: ${lineHeight}px;`);
  });

  it("registers every role with tailwind-merge", () => {
    const utils = readFileSync(resolve(repoRoot, "packages/ui/lib/utils.ts"), "utf8");
    for (const [name] of scale) expect(utils).toContain(`"${name}"`);
  });

  it("contains no retired or arbitrary product font sizes", () => {
    const allowed = new Set(scale.map(([name]) => `text-${name}`));
    const violations: string[] = [];
    for (const root of scanRoots) {
      for (const path of sourceFiles(resolve(repoRoot, root))) {
        const source = stripComments(readFileSync(path, "utf8"));
        for (const match of source.matchAll(/\btext-(?:\[[^\]]+\]|xs|sm|base|lg|xl|2xl|3xl|micro|caption|label|body|title-sm|title-lg|display-sm)\b/g)) {
          if (!allowed.has(match[0])) violations.push(`${relative(repoRoot, path)}: ${match[0]}`);
        }
      }
    }
    expect(violations).toEqual([]);
  });
});
