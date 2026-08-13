import { describe, expect, it } from "vitest";
import { docsSlugStaticParams } from "./static-params";

// `source.generateParams()` hands back loosely-typed params (`lang: string`),
// so the inputs here mirror that shape — the `lang` strings are validated and
// narrowed by `docsSlugStaticParams` itself.
type RawParam = { lang: string; slug: string[] };

describe("docsSlugStaticParams", () => {
  it("returns every localized slug page and drops the home param", () => {
    const params: RawParam[] = [
      { lang: "zh", slug: [] },
      { lang: "zh", slug: ["agents"] },
      { lang: "zh", slug: ["cli", "reference"] },
      { lang: "en", slug: ["agents"] },
    ];

    expect(docsSlugStaticParams(params)).toEqual([
      { lang: "zh", slug: ["agents"] },
      { lang: "zh", slug: ["cli", "reference"] },
    ]);
  });

  it("drops unknown languages and de-duplicates repeated params", () => {
    const params: RawParam[] = [
      { lang: "zh", slug: ["agents"] },
      { lang: "zh", slug: ["agents"] },
      { lang: "fr", slug: ["agents"] },
    ];

    expect(docsSlugStaticParams(params)).toEqual([
      { lang: "zh", slug: ["agents"] },
    ]);
  });
});
