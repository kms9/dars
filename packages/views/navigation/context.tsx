"use client";

import { createContext, startTransition, use, useMemo } from "react";
import type { NavigationAdapter } from "./types";

const NavigationContext = createContext<NavigationAdapter | null>(null);

export function NavigationProvider({
  value,
  children,
}: {
  value: NavigationAdapter;
  children: React.ReactNode;
}) {
  // Wrap push/replace in a transition so route commits do not block urgent UI.
  const wrapped = useMemo<NavigationAdapter>(
    () => ({
      ...value,
      push: (path: string) => startTransition(() => value.push(path)),
      replace: (path: string) => startTransition(() => value.replace(path)),
    }),
    [value],
  );
  return (
    <NavigationContext.Provider value={wrapped}>
      {children}
    </NavigationContext.Provider>
  );
}

export function useNavigation(): NavigationAdapter {
  const ctx = use(NavigationContext);
  if (!ctx)
    throw new Error("useNavigation must be used within NavigationProvider");
  return ctx;
}
