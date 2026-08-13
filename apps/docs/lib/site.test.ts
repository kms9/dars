import { beforeEach, describe, expect, it, vi } from "vitest";

const existingDocs = vi.hoisted(() => new Set<string>());

vi.mock("node:fs", () => ({
  existsSync: vi.fn((path: string) => {
    const normalized = path.replaceAll("\\", "/");
    return [...existingDocs].some((suffix) => normalized.endsWith(suffix));
  }),
}));

const pages = new Map<string, { url: string }>([
  ["zh:", { url: "/" }],
  ["zh:agents", { url: "/agents" }],
]);

vi.mock("@/lib/source", () => ({
  source: {
    getPage: vi.fn((slugs: string[], lang: string) => {
      return pages.get(`${lang}:${slugs.join("/")}`) ?? null;
    }),
  },
}));

beforeEach(() => {
  existingDocs.clear();
  existingDocs.add("index.mdx");
  existingDocs.add("agents.mdx");
});

describe("docsAlternates", () => {
  it("emits Chinese-only hreflang for pages with MDX sources", async () => {
    const { docsAlternates } = await import("./site");

    expect(docsAlternates(["agents"])).toEqual({
      canonical: "https://www.dars.ai/docs/agents",
      languages: {
        zh: "https://www.dars.ai/docs/agents",
        "x-default": "https://www.dars.ai/docs/agents",
      },
    });
  });

  it("keeps the locale root alternates limited to real MDX pages", async () => {
    const { docsAlternates } = await import("./site");

    expect(docsAlternates([])).toEqual({
      canonical: "https://www.dars.ai/docs",
      languages: {
        zh: "https://www.dars.ai/docs",
        "x-default": "https://www.dars.ai/docs",
      },
    });
  });

  it("omits hreflang when the MDX file is missing", async () => {
    existingDocs.clear();
    const { docsAlternates } = await import("./site");

    expect(docsAlternates(["agents"])).toEqual({
      canonical: "https://www.dars.ai/docs",
      languages: {},
    });
  });
});
