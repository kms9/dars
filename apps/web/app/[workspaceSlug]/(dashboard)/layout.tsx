"use client";

import { LightweightShell } from "@dars/views/lightweight";

export default function Layout({ children }: { children: React.ReactNode }) {
  return <LightweightShell>{children}</LightweightShell>;
}
