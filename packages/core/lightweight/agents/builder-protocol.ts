import type { StoredAgentDraft } from "./types";

export type BuilderDraftPayload = {
  name?: unknown;
  description?: unknown;
  instructions?: unknown;
  model?: unknown;
  skill_ids?: unknown;
  permission_scope?: unknown;
  member_ids?: unknown;
};

export function parseBuilderDraft(content: string): BuilderDraftPayload | null {
  const match = content.match(/<agent_draft>([\s\S]*?)<\/agent_draft>/);
  if (!match?.[1]) return null;
  try {
    const value = JSON.parse(match[1]);
    return value && typeof value === "object" ? (value as BuilderDraftPayload) : null;
  } catch {
    try {
      const value = JSON.parse(escapeJsonStringControlCharacters(match[1]));
      return value && typeof value === "object" ? (value as BuilderDraftPayload) : null;
    } catch {
      return null;
    }
  }
}

function escapeJsonStringControlCharacters(value: string): string {
  let result = "";
  let inString = false;
  let escaped = false;
  for (const character of value) {
    if (!inString) {
      result += character;
      if (character === '"') inString = true;
      continue;
    }
    if (escaped) {
      result += character;
      escaped = false;
      continue;
    }
    if (character === "\\") {
      result += character;
      escaped = true;
      continue;
    }
    if (character === '"') {
      result += character;
      inString = false;
      continue;
    }
    if (character === "\n") result += "\\n";
    else if (character === "\r") result += "\\r";
    else if (character === "\t") result += "\\t";
    else result += character;
  }
  return result;
}

export function stripBuilderDraft(content: string): string {
  return content
    .replace(/\s*<agent_draft>[\s\S]*?<\/agent_draft>/g, "")
    .replace(/\s*<agent_draft>[\s\S]*$/, "")
    .trim();
}

const BUILDER_INPUT_PREFIX = "DARS_AGENT_BUILDER_INPUT\n";

export function encodeBuilderInput(request: string, draft: StoredAgentDraft): string {
  return `${BUILDER_INPUT_PREFIX}${JSON.stringify({
    user_request: request,
    current_draft: {
      name: draft.name,
      description: draft.description,
      instructions: draft.instructions,
      model: draft.model,
      skill_ids: [...draft.skill_ids],
      permission_scope: draft.permission_scope,
      member_ids: [...draft.member_ids],
    },
  }, null, 2)}`;
}

export function decodeBuilderInput(content: string): string {
  if (!content.startsWith(BUILDER_INPUT_PREFIX)) return content;
  try {
    const parsed = JSON.parse(content.slice(BUILDER_INPUT_PREFIX.length)) as { user_request?: unknown };
    return typeof parsed.user_request === "string" ? parsed.user_request : content;
  } catch {
    return content;
  }
}

export function mergeBuilderDraft(
  current: StoredAgentDraft,
  payload: BuilderDraftPayload,
  validSkillIds: ReadonlySet<string>,
  validMemberIds: ReadonlySet<string>,
): StoredAgentDraft {
  const permissionScope =
    payload.permission_scope === "workspace" ||
    payload.permission_scope === "members" ||
    payload.permission_scope === "private"
      ? payload.permission_scope
      : current.permission_scope;
  const skillIds = Array.isArray(payload.skill_ids)
    ? payload.skill_ids.filter((id): id is string => typeof id === "string" && validSkillIds.has(id))
    : [...current.skill_ids];
  const memberIds = Array.isArray(payload.member_ids)
    ? payload.member_ids.filter((id): id is string => typeof id === "string" && validMemberIds.has(id))
    : [...current.member_ids];
  return {
    ...current,
    name: typeof payload.name === "string" ? payload.name : current.name,
    description: typeof payload.description === "string" ? payload.description : current.description,
    instructions: typeof payload.instructions === "string" ? payload.instructions : current.instructions,
    model: typeof payload.model === "string" ? payload.model : current.model,
    thinking_level: current.thinking_level,
    service_tier: current.service_tier,
    skill_ids: skillIds,
    permission_scope: permissionScope,
    member_ids: permissionScope === "members" ? memberIds : [],
    max_concurrent_tasks: current.max_concurrent_tasks,
    avatar_url: current.avatar_url,
  };
}
