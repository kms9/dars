import type { LightweightAgent, LightweightRuntime } from "../types";
import type { AgentDraft, AgentInvocationTargetInput, AgentPermissionScope, StoredAgentDraft } from "./types";
import { isRuntimeUsableForUser } from "./runtime-access";

export const AGENT_DESCRIPTION_MAX_LENGTH = 2000;
export const AGENT_MAX_CONCURRENT_TASKS_MIN = 1;
export const AGENT_MAX_CONCURRENT_TASKS_MAX = 32;

export const EMPTY_AGENT_DRAFT: AgentDraft = {
  name: "",
  description: "",
  instructions: "",
  avatarUrl: null,
  avatarFile: null,
  runtimeId: "",
  model: "",
  thinkingLevel: "",
  serviceTier: "",
  skillIds: new Set(),
  permissionScope: "private",
  memberIds: new Set(),
  maxConcurrentTasks: 1,
};

export function applyDraftRuntimeChange(draft: AgentDraft, runtimeId: string): AgentDraft {
  return {
    ...draft,
    runtimeId,
    model: "",
    thinkingLevel: "",
    serviceTier: "",
  };
}

export function applyDraftModelChange(draft: AgentDraft, model: string): AgentDraft {
  if (model === draft.model) return draft;
  return { ...draft, model, thinkingLevel: "", serviceTier: "" };
}

export function isDraftDescriptionWithinLimit(description: string): boolean {
  return [...description].length <= AGENT_DESCRIPTION_MAX_LENGTH;
}

export function buildInvocationTargets(draft: AgentDraft): AgentInvocationTargetInput[] {
  if (draft.permissionScope === "private") return [];
  if (draft.permissionScope === "workspace") return [{ target_type: "workspace" }];
  return [...draft.memberIds].map((targetId) => ({
    target_type: "member" as const,
    target_id: targetId,
  }));
}

export function deriveDuplicateAccess(
  agent: Pick<LightweightAgent, "permission_mode" | "invocation_targets">,
): Pick<AgentDraft, "permissionScope" | "memberIds"> {
  if (agent.permission_mode !== "public_to") {
    return { permissionScope: "private", memberIds: new Set() };
  }
  const targets = agent.invocation_targets ?? [];
  if (targets.some((target) => target.target_type === "workspace")) {
    return { permissionScope: "workspace", memberIds: new Set() };
  }
  const memberIds = targets
    .filter((target) => target.target_type === "member" && target.target_id)
    .map((target) => target.target_id);
  if (memberIds.length === 0) {
    return { permissionScope: "private", memberIds: new Set() };
  }
  return { permissionScope: "members", memberIds: new Set(memberIds) };
}

export function buildDuplicateDraft(
  source: LightweightAgent,
  options: {
    runtimes: LightweightRuntime[];
    currentUserId: string | null;
    fallbackRuntimeId: string;
    nameSuffix: string;
  },
): AgentDraft {
  const keepsRuntime =
    !!source.runtime_id &&
    options.runtimes.some(
      (runtime) =>
        runtime.id === source.runtime_id &&
        isRuntimeUsableForUser(runtime, options.currentUserId),
    );
  return {
    ...EMPTY_AGENT_DRAFT,
    name: `${source.name}${options.nameSuffix}`,
    description: source.description ?? "",
    instructions: source.instructions ?? "",
    avatarUrl: null,
    avatarFile: null,
    runtimeId: keepsRuntime ? (source.runtime_id as string) : options.fallbackRuntimeId,
    model: keepsRuntime ? source.model ?? "" : "",
    thinkingLevel: keepsRuntime ? source.thinking_level ?? "" : "",
    serviceTier: keepsRuntime ? source.service_tier ?? "" : "",
    skillIds: new Set(source.skills.map((skill) => skill.id)),
    maxConcurrentTasks: source.max_concurrent_tasks,
    ...deriveDuplicateAccess(source),
  };
}

export function buildCreateAgentBody(options: {
  draft: AgentDraft;
  runtimeId: string;
  duplicateSource?: LightweightAgent | null;
}): Record<string, unknown> {
  const { draft, runtimeId, duplicateSource } = options;
  const body: Record<string, unknown> = {
    name: draft.name.trim(),
    description: draft.description.trim(),
    instructions: draft.instructions.trim(),
    runtime_id: runtimeId,
    permission_mode: draft.permissionScope === "private" ? "private" : "public_to",
    invocation_targets: buildInvocationTargets(draft),
    max_concurrent_tasks: draft.maxConcurrentTasks,
    runtime_config: duplicateSource?.runtime_config ?? {},
    custom_args: duplicateSource?.custom_args ?? [],
    disabled_runtime_skills: duplicateSource?.disabled_runtime_skills ?? [],
  };
  if (draft.model.trim()) body.model = draft.model.trim();
  if (draft.thinkingLevel.trim()) body.thinking_level = draft.thinkingLevel.trim();
  if (draft.serviceTier.trim()) body.service_tier = draft.serviceTier.trim();
  return body;
}

export function toStoredAgentDraft(draft: AgentDraft): StoredAgentDraft {
  return {
    name: draft.name,
    description: draft.description,
    instructions: draft.instructions,
    avatar_url: draft.avatarUrl,
    model: draft.model,
    thinking_level: draft.thinkingLevel,
    service_tier: draft.serviceTier,
    skill_ids: [...draft.skillIds],
    permission_scope: draft.permissionScope,
    member_ids: [...draft.memberIds],
    max_concurrent_tasks: draft.maxConcurrentTasks,
  };
}

export function fromStoredAgentDraft(stored: StoredAgentDraft, runtimeId: string): AgentDraft {
  return {
    ...EMPTY_AGENT_DRAFT,
    name: stored.name,
    description: stored.description,
    instructions: stored.instructions,
    avatarUrl: stored.avatar_url,
    runtimeId,
    model: stored.model,
    thinkingLevel: stored.thinking_level,
    serviceTier: stored.service_tier,
    skillIds: new Set(stored.skill_ids),
    permissionScope: stored.permission_scope,
    memberIds: new Set(stored.member_ids),
    maxConcurrentTasks: stored.max_concurrent_tasks,
  };
}

export function storedAgentDraftsEqual(a: StoredAgentDraft, b: StoredAgentDraft): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

export const MANUAL_DRAFT_BLANK_OWNER = "blank";

export function manualDraftOwner(duplicateId: string | null): string {
  return duplicateId ? `duplicate:${duplicateId}` : MANUAL_DRAFT_BLANK_OWNER;
}

export function manualDraftEntryHasContent(entry: {
  runtimeId: string;
  draft: StoredAgentDraft;
}): boolean {
  return !storedAgentDraftsEqual(entry.draft, toStoredAgentDraft(EMPTY_AGENT_DRAFT));
}

export function setManualDraftEntry(
  drafts: { byOwner: Record<string, { runtimeId: string; draft: StoredAgentDraft }> },
  owner: string,
  entry: { runtimeId: string; draft: StoredAgentDraft } | null,
): { byOwner: Record<string, { runtimeId: string; draft: StoredAgentDraft }> } {
  const byOwner = { ...drafts.byOwner };
  if (entry && manualDraftEntryHasContent(entry)) byOwner[owner] = entry;
  else delete byOwner[owner];
  return { byOwner };
}

export function canManageAgent(
  agentOwnerId: string,
  currentUserId: string | null,
  memberRole: "owner" | "admin" | "member" | null,
): boolean {
  if (memberRole === "owner" || memberRole === "admin") return true;
  return !!currentUserId && currentUserId === agentOwnerId;
}

export type { AgentPermissionScope };
