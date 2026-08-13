import { defineI18n } from "fumadocs-core/i18n";

// Docs content is Chinese-only. hideLocale: 'default-locale' keeps URLs
// prefix-free (`/docs/...`). parser: 'dot' still applies if more locales
// are added later as `page.<lang>.mdx` / `meta.<lang>.json`.
export const i18n = defineI18n({
  languages: ["zh"],
  defaultLanguage: "zh",
  hideLocale: "default-locale",
  parser: "dot",
});

export type Lang = (typeof i18n.languages)[number];
