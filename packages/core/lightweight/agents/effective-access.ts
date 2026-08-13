import type { AccessScope, AgentInvocationTargetInput } from "./types";
import type { LightweightInvocationTarget } from "../types";

export function snapshotAccessScope(permissionMode: string): AccessScope {
  return permissionMode === "public_to" ? "workspace" : "owner-only";
}

export function effectiveAccessScope(
  permissionMode: string | undefined | null,
  invocationTargets: readonly LightweightInvocationTarget[] | undefined | null,
): AccessScope {
  if (permissionMode !== "public_to") return "owner-only";
  if (!invocationTargets || invocationTargets.length === 0) {
    return snapshotAccessScope(permissionMode);
  }
  if (invocationTargets.some((target) => target.target_type === "workspace")) {
    return "workspace";
  }
  return "specific-people";
}

export const ALL_ACCESS_SCOPES: readonly AccessScope[] = [
  "workspace",
  "specific-people",
  "owner-only",
];

export function isAccessChangeReady(change: {
  permission_mode: "private" | "public_to";
  invocation_targets: readonly AgentInvocationTargetInput[];
} | null): boolean {
  if (!change) return false;
  if (change.permission_mode === "private") return true;
  return change.invocation_targets.length > 0;
}

export function accessScopeLabel(scope: AccessScope): string {
  switch (scope) {
    case "workspace":
      return "Workspace";
    case "specific-people":
      return "Specific members";
    default:
      return "Private";
  }
}
