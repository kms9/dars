import { describe, expect, it } from "vitest";
import { prefixLocale } from "./locale-link";

describe("prefixLocale", () => {
  it("leaves the default language untouched (URLs are prefix-less)", () => {
    expect(prefixLocale("/workspaces", "zh")).toBe("/workspaces");
    expect(prefixLocale("/agents-create", "zh")).toBe("/agents-create");
    expect(prefixLocale("/", "zh")).toBe("/");
  });

  it("does not double-prefix paths that already carry a known locale", () => {
    expect(prefixLocale("/zh/workspaces", "zh")).toBe("/zh/workspaces");
  });

  it("leaves external URLs alone", () => {
    expect(prefixLocale("https://dars.ai/download", "zh")).toBe(
      "https://dars.ai/download",
    );
    expect(prefixLocale("mailto:hello@dars.ai", "zh")).toBe(
      "mailto:hello@dars.ai",
    );
    expect(prefixLocale("tel:+1234567890", "zh")).toBe("tel:+1234567890");
  });

  it("leaves in-page anchors and relative paths alone", () => {
    expect(prefixLocale("#section", "zh")).toBe("#section");
    expect(prefixLocale("./sibling", "zh")).toBe("./sibling");
    expect(prefixLocale("../sibling", "zh")).toBe("../sibling");
  });

  it("returns empty/undefined hrefs unchanged", () => {
    expect(prefixLocale("", "zh")).toBe("");
  });
});
