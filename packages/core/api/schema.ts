import type { ZodType } from "zod";

export function parseWithFallback<T>(schema: ZodType<T>, value: unknown, fallback: T): T {
  const parsed = schema.safeParse(value);
  return parsed.success ? parsed.data : fallback;
}
