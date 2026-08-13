import type { LightweightRuntime } from "../types";

export function isRuntimeOnline(runtime: LightweightRuntime | null | undefined): boolean {
  return runtime?.status === "online";
}

export function isRuntimeUsableForUser(
  runtime: LightweightRuntime,
  currentUserId: string | null,
): boolean {
  if (!isRuntimeOnline(runtime)) return false;
  if (!currentUserId) return false;
  return runtime.owner_id === currentUserId;
}

export function runtimeDisplayLabel(runtime: LightweightRuntime): string {
  return runtime.name || runtime.provider;
}
