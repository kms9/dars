import type { LightweightAgent, LightweightInvocationTarget } from "../types";
import { canManageAgent } from "./draft";

export function canInvokeAgent(
  agent: Pick<LightweightAgent, "owner_id" | "permission_mode" | "archived_at">,
  invocationTargets: readonly LightweightInvocationTarget[] | undefined | null,
  currentUserId: string | null,
  currentMemberId: string | null,
): boolean {
  if (agent.archived_at) return false;
  if (!currentUserId) return false;
  if (agent.owner_id === currentUserId) return true;
  if (agent.permission_mode !== "public_to") return false;
  if (!invocationTargets || invocationTargets.length === 0) return false;
  if (invocationTargets.some((target) => target.target_type === "workspace")) return true;
  if (!currentMemberId) return false;
  return invocationTargets.some(
    (target) => target.target_type === "member" && target.target_id === currentMemberId,
  );
}

export function canViewAgentSettings(
  agent: Pick<LightweightAgent, "owner_id">,
  currentUserId: string | null,
  memberRole: "owner" | "admin" | "member" | null,
): boolean {
  if (memberRole === "owner" || memberRole === "admin") return true;
  return !!currentUserId && agent.owner_id === currentUserId;
}

export { canManageAgent };
