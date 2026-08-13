import { api } from "../api";
import type { Workspace } from "../types";
import type { AgentBuilderSession, AgentBuilderSessionSummary, StoredAgentDraft } from "./agents/types";
import {
  parseAgentToolBundle,
  parseToolBundle,
  parseToolDefinitions,
  parseToolSource,
  parseToolSourceImportResult,
  parseToolSources,
} from "./tools/schemas";
import type {
  AgentToolBundle,
  CreateToolSourceInput,
  ImportToolSourceInput,
  ToolBundle,
  ToolBundleSelectionItem,
  ToolSource,
  ToolSourceImportResult,
} from "./tools/types";
import type {
  LightweightAgent,
  LightweightAgentSkill,
  LightweightAgentSnapshot,
  LightweightAgentTasksPage,
  LightweightChatMessage,
  LightweightChatSession,
  LightweightComment,
  LightweightDraftRestore,
  LightweightMember,
  LightweightPage,
  LightweightPendingTask,
  LightweightRun,
  LightweightCreateRuntimeLocalSkillImportRequest,
  LightweightRuntime,
  LightweightRuntimeLocalSkillImportRequest,
  LightweightRuntimeLocalSkillListRequest,
  LightweightRuntimeProfile,
  LightweightSkill,
  LightweightSkillFile,
  LightweightSkillImportResult,
  LightweightSkillOnConflict,
  LightweightSkillSearchCandidate,
  LightweightSquad,
  LightweightSquadMember,
  LightweightSquadMemberStatus,
  LightweightTaskMessage,
  LightweightTaskRun,
} from "./types";

type JSONValue = Record<string, unknown>;

function json(method: string, body?: unknown, headers?: Record<string, string>): RequestInit {
  return {
    method,
    body: body === undefined ? undefined : JSON.stringify(body),
    headers,
  };
}

function pagePath(path: string, params?: Record<string, string | string[] | undefined>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params ?? {})) {
    if (Array.isArray(value)) value.forEach((item) => search.append(key, item));
    else if (value) search.set(key, value);
  }
  const query = search.toString();
  return query ? `${path}?${query}` : path;
}

export function newIdempotencyKey(scope: string): string {
  const id = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `web-${scope}-${id}`;
}

function requireParsed<T>(value: T | null, endpoint: string): T {
  if (value !== null) return value;
  throw new Error(`Invalid API response from ${endpoint}`);
}

export const lightweightApi = {
  listWorkspaces: () => api.request<Workspace[]>("/api/workspaces"),
  createWorkspace: (body: { name: string; slug?: string }) =>
    api.request<Workspace>("/api/workspaces", json("POST", body)),
  getWorkspace: (id: string) => api.request<Workspace>(`/api/workspaces/${id}`),
  updateWorkspace: (id: string, body: JSONValue) =>
    api.request<Workspace>(`/api/workspaces/${id}`, json("PUT", body)),
  deleteWorkspace: (id: string) => api.request<void>(`/api/workspaces/${id}`, json("DELETE")),
  listMembers: (id: string) =>
    api.request<LightweightMember[]>(`/api/workspaces/${id}/members`),

  listRuntimes: () => api.request<LightweightRuntime[]>("/api/runtimes"),
  updateRuntime: (id: string, customName: string | null) =>
    api.request<LightweightRuntime>(`/api/runtimes/${id}`, json("PATCH", { custom_name: customName })),
  deleteRuntime: (id: string) => api.request<void>(`/api/runtimes/${id}`, json("DELETE")),
  listRuntimeProfiles: (workspaceId: string) =>
    api.request<LightweightRuntimeProfile[]>(`/api/workspaces/${workspaceId}/runtime-profiles`),
  getRuntimeProfile: (workspaceId: string, id: string) =>
    api.request<LightweightRuntimeProfile>(`/api/workspaces/${workspaceId}/runtime-profiles/${id}`),
  createRuntimeProfile: (workspaceId: string, body: JSONValue) =>
    api.request<LightweightRuntimeProfile>(`/api/workspaces/${workspaceId}/runtime-profiles`, json("POST", body)),
  updateRuntimeProfile: (workspaceId: string, id: string, body: JSONValue) =>
    api.request<LightweightRuntimeProfile>(`/api/workspaces/${workspaceId}/runtime-profiles/${id}`, json("PUT", body)),
  deleteRuntimeProfile: (workspaceId: string, id: string) =>
    api.request<void>(`/api/workspaces/${workspaceId}/runtime-profiles/${id}`, json("DELETE")),

  listAgents: () => api.request<LightweightAgent[]>("/api/agents"),
  getAgentSnapshot: () => api.request<LightweightAgentSnapshot>("/api/agents/snapshot"),
  getAgent: (id: string) => api.request<LightweightAgent>(`/api/agents/${id}`),
  createAgent: (body: JSONValue) => api.request<LightweightAgent>("/api/agents", json("POST", body)),
  updateAgent: (id: string, body: JSONValue) =>
    api.request<LightweightAgent>(`/api/agents/${id}`, json("PUT", body)),
  archiveAgent: (id: string) => api.request<LightweightAgent>(`/api/agents/${id}/archive`, json("POST")),
  restoreAgent: (id: string) => api.request<LightweightAgent>(`/api/agents/${id}/restore`, json("POST")),
  cancelAgentTasks: (id: string) =>
    api.request<{ cancelled: number }>(`/api/agents/${id}/tasks/cancel`, json("POST")),
  getAgentTasks: (id: string, cursor?: string) =>
    api.request<LightweightAgentTasksPage>(
      pagePath(`/api/agents/${id}/tasks`, { limit: "30", cursor }),
    ),
  uploadAgentAvatar: (id: string, file: File | Blob, filename: string) => {
    const form = new FormData();
    form.append("file", file, filename);
    return api.request<LightweightAgent>(`/api/agents/${id}/avatar`, { method: "POST", body: form });
  },
  revealAgentEnv: (id: string) => api.request<{ custom_env: Record<string, string> }>(`/api/agents/${id}/env`),
  updateAgentEnv: (id: string, customEnv: Record<string, string>) =>
    api.request<{ agent_id: string; custom_env: Record<string, string> }>(`/api/agents/${id}/env`, json("PUT", { custom_env: customEnv })),
  setAgentSkills: (id: string, skills: Array<{ skill_id: string; enabled: boolean }>) =>
    api.request<LightweightAgentSkill[]>(`/api/agents/${id}/skills`, json("PUT", { skills })),

  listToolSources: () => api.request<unknown>("/api/tool-sources").then(parseToolSources),
  getToolSource: (id: string) =>
    api.request<unknown>(`/api/tool-sources/${id}`).then((value) =>
      requireParsed(parseToolSource(value), `/api/tool-sources/${id}`)),
  createToolSource: (input: CreateToolSourceInput): Promise<ToolSource> =>
    api.request<unknown>("/api/tool-sources", json("POST", {
      name: input.name,
      kind: input.kind,
      ...(input.endpoint ? { endpoint: input.endpoint } : {}),
      ...(input.transportConfig ? { transport_config: input.transportConfig } : {}),
      ...(input.auth ? { auth: input.auth } : {}),
    })).then((value) => requireParsed(parseToolSource(value), "/api/tool-sources")),
  updateToolSource: (id: string, input: Omit<CreateToolSourceInput, "name" | "kind">): Promise<ToolSource> =>
    api.request<unknown>(`/api/tool-sources/${id}`, json("PUT", {
      ...(input.endpoint ? { endpoint: input.endpoint } : {}),
      ...(input.transportConfig ? { transport_config: input.transportConfig } : {}),
      ...(input.auth ? { auth: input.auth } : {}),
    })).then((value) => requireParsed(parseToolSource(value), `/api/tool-sources/${id}`)),
  validateToolSource: (id: string, revisionId: string): Promise<ToolSource> =>
    api.request<unknown>(`/api/tool-sources/${id}/validate`, json("POST", { revision_id: revisionId }))
      .then((value) => requireParsed(parseToolSource(value), `/api/tool-sources/${id}/validate`)),
  enableToolSource: (id: string): Promise<ToolSource> =>
    api.request<unknown>(`/api/tool-sources/${id}/enable`, json("POST", {}))
      .then((value) => requireParsed(parseToolSource(value), `/api/tool-sources/${id}/enable`)),
  disableToolSource: (id: string): Promise<ToolSource> =>
    api.request<unknown>(`/api/tool-sources/${id}/disable`, json("POST", {}))
      .then((value) => requireParsed(parseToolSource(value), `/api/tool-sources/${id}/disable`)),
  deleteToolSource: (id: string) => api.request<void>(`/api/tool-sources/${id}`, json("DELETE")),
  listToolSourceDefinitions: (id: string) =>
    api.request<unknown>(`/api/tool-sources/${id}/tools`).then(parseToolDefinitions),
  importToolSource: (input: ImportToolSourceInput): Promise<ToolSourceImportResult> => {
    const form = new FormData();
    if (input.sourceId) form.append("source_id", input.sourceId);
    form.append("name", input.name);
    form.append("kind", input.kind);
    form.append("endpoint", input.endpoint);
    form.append("transport_config", JSON.stringify(input.transportConfig ?? {}));
    if (input.auth) form.append("auth", JSON.stringify(input.auth));
    if (input.mediaType) form.append("media_type", input.mediaType);
    form.append("file", input.file, input.filename);
    return api.request<unknown>("/api/tool-sources/import", { method: "POST", body: form })
      .then((value) => requireParsed(parseToolSourceImportResult(value), "/api/tool-sources/import"));
  },
  getToolBundle: (id: string): Promise<ToolBundle> =>
    api.request<unknown>(`/api/tool-bundles/${id}`)
      .then((value) => requireParsed(parseToolBundle(value), `/api/tool-bundles/${id}`)),
  revokeToolBundle: (id: string): Promise<ToolBundle> =>
    api.request<unknown>(`/api/tool-bundles/${id}/revoke`, json("POST", {}))
      .then((value) => requireParsed(parseToolBundle(value), `/api/tool-bundles/${id}/revoke`)),
  getAgentToolBundle: (id: string): Promise<AgentToolBundle> =>
    api.request<unknown>(`/api/agents/${id}/tool-bundle`).then(parseAgentToolBundle),
  publishAgentToolBundle: (id: string, items: ToolBundleSelectionItem[]): Promise<ToolBundle> =>
    api.request<unknown>(`/api/agents/${id}/tool-bundle`, json("PUT", {
      items: items.map((item) => ({
        tool_definition_id: item.toolDefinitionId,
        exported_name: item.exportedName,
      })),
    }))
      .then((value) => requireParsed(parseToolBundle(value), `/api/agents/${id}/tool-bundle`)),
  clearAgentToolBundle: (id: string): Promise<AgentToolBundle> =>
    api.request<unknown>(`/api/agents/${id}/tool-bundle`, json("DELETE")).then(parseAgentToolBundle),

  listAgentBuilderSessions: () =>
    api.request<{ sessions: AgentBuilderSessionSummary[] }>("/api/agent-builder/sessions").then((value) => value.sessions),
  createAgentBuilderSession: (body: { runtime_id: string; model?: string }) =>
    api.request<AgentBuilderSession>("/api/agent-builder/sessions", json("POST", body)),
  saveAgentBuilderDraft: (sessionId: string, draft: StoredAgentDraft, options?: { finalize?: boolean; idempotencyKey?: string }) =>
    api.request<{ agent_id: string } | void>(`/api/agent-builder/sessions/${sessionId}/draft`, {
      method: "PUT",
      body: JSON.stringify({ draft, finalize: options?.finalize === true }),
      headers: options?.idempotencyKey ? { "Idempotency-Key": options.idempotencyKey } : undefined,
    }),
  switchAgentBuilderRuntime: (sessionId: string, runtimeId: string) =>
    api.request<{ runtime_id: string }>(`/api/agent-builder/sessions/${sessionId}/runtime`, json("PATCH", { runtime_id: runtimeId })),

  listSkills: () => api.request<LightweightSkill[]>("/api/skills"),
  getSkill: (id: string) => api.request<LightweightSkill>(`/api/skills/${id}`),
  createSkill: (body: JSONValue) => api.request<LightweightSkill>("/api/skills", json("POST", body)),
  updateSkill: (id: string, body: JSONValue) => api.request<LightweightSkill>(`/api/skills/${id}`, json("PUT", body)),
  deleteSkill: (id: string) => api.request<void>(`/api/skills/${id}`, json("DELETE")),
  listSkillFiles: (id: string) => api.request<LightweightSkillFile[]>(`/api/skills/${id}/files`),
  replaceSkillFiles: (id: string, files: Array<{ path: string; content: string }>) =>
    api.request<LightweightSkillFile[]>(`/api/skills/${id}/files`, json("PUT", { files })),
  deleteSkillFile: (skillId: string, fileId: string) =>
    api.request<void>(`/api/skills/${skillId}/files/${fileId}`, json("DELETE")),
  importSkill: (body: { url: string; on_conflict?: LightweightSkillOnConflict }) =>
    api.request<LightweightSkill | LightweightSkillImportResult>("/api/skills/import", json("POST", body)),
  importSkillArchive: (
    file: File | Blob,
    filename: string,
    onConflict?: LightweightSkillOnConflict,
  ) => {
    const form = new FormData();
    form.append("file", file, filename);
    if (onConflict) form.append("on_conflict", onConflict);
    return api.request<LightweightSkill | LightweightSkillImportResult>("/api/skills/import", {
      method: "POST",
      body: form,
    });
  },
  searchSkills: (q: string) =>
    api.request<LightweightSkillSearchCandidate[]>(pagePath("/api/skills/search", { q })),
  initiateListLocalSkills: (runtimeId: string) =>
    api.request<LightweightRuntimeLocalSkillListRequest>(`/api/runtimes/${runtimeId}/local-skills`, json("POST")),
  getListLocalSkillsResult: (runtimeId: string, requestId: string) =>
    api.request<LightweightRuntimeLocalSkillListRequest>(`/api/runtimes/${runtimeId}/local-skills/${requestId}`),
  initiateImportLocalSkill: (runtimeId: string, body: LightweightCreateRuntimeLocalSkillImportRequest) =>
    api.request<LightweightRuntimeLocalSkillImportRequest>(
      `/api/runtimes/${runtimeId}/local-skills/import`,
      json("POST", body),
    ),
  getImportLocalSkillResult: (runtimeId: string, requestId: string) =>
    api.request<LightweightRuntimeLocalSkillImportRequest>(
      `/api/runtimes/${runtimeId}/local-skills/import/${requestId}`,
    ),

  listSquads: () => api.request<LightweightSquad[]>("/api/squads"),
  getSquad: (id: string) => api.request<LightweightSquad>(`/api/squads/${id}`),
  createSquad: (body: JSONValue) => api.request<LightweightSquad>("/api/squads", json("POST", body)),
  updateSquad: (id: string, body: JSONValue) => api.request<LightweightSquad>(`/api/squads/${id}`, json("PUT", body)),
  archiveSquad: (id: string) => api.request<void>(`/api/squads/${id}`, json("DELETE")),
  listSquadMembers: (id: string) => api.request<LightweightSquadMember[]>(`/api/squads/${id}/members`),
  listSquadMemberStatus: (id: string) =>
    api.request<{ members: LightweightSquadMemberStatus[] }>(`/api/squads/${id}/members/status`),
  addSquadMember: (id: string, agentId: string, role: "leader" | "member") =>
    api.request<LightweightSquadMember>(`/api/squads/${id}/members`, json("POST", { agent_id: agentId, role })),
  setSquadMemberRole: (id: string, agentId: string, role: "leader" | "member") =>
    api.request<LightweightSquadMember>(`/api/squads/${id}/members/role`, json("PATCH", { agent_id: agentId, role })),
  removeSquadMember: (id: string, agentId: string) =>
    api.request<void>(`/api/squads/${id}/members`, json("DELETE", { agent_id: agentId })),
  uploadSquadAvatar: (id: string, file: File | Blob, filename: string) => {
    const form = new FormData();
    form.append("file", file, filename);
    return api.request<LightweightSquad>(`/api/squads/${id}/avatar`, { method: "POST", body: form });
  },

  listRuns: (params?: {
    cursor?: string;
    statuses?: string[];
    assigneeType?: string;
    assigneeId?: string;
    creatorType?: string;
    creatorId?: string;
  }) =>
    api.request<LightweightPage<LightweightRun>>(pagePath("/api/issues", {
      limit: "30",
      cursor: params?.cursor,
      status: params?.statuses,
      assignee_type: params?.assigneeType,
      assignee_id: params?.assigneeId,
      creator_type: params?.creatorType,
      creator_id: params?.creatorId,
    })),
  getRun: (id: string) => api.request<LightweightRun>(`/api/issues/${id}`),
  createRun: (body: JSONValue, key = newIdempotencyKey("run")) =>
    api.request<LightweightRun>("/api/issues", json("POST", body, { "Idempotency-Key": key })),
  updateRun: (id: string, body: JSONValue) => api.request<LightweightRun>(`/api/issues/${id}`, json("PUT", body)),
  deleteRun: (id: string) => api.request<void>(`/api/issues/${id}`, json("DELETE")),
  listComments: (id: string, cursor?: string) =>
    api.request<LightweightPage<LightweightComment>>(pagePath(`/api/issues/${id}/comments`, { limit: "100", cursor })),
  createComment: (id: string, content: string, key = newIdempotencyKey("comment")) =>
    api.request<LightweightComment>(`/api/issues/${id}/comments`, json("POST", { content }, { "Idempotency-Key": key })),
  listTaskRuns: (id: string, cursor?: string) =>
    api.request<LightweightPage<LightweightTaskRun>>(pagePath(`/api/issues/${id}/task-runs`, { limit: "100", cursor })),
  activeRunTasks: (id: string) => api.request<{ tasks: LightweightTaskRun[] }>(`/api/issues/${id}/active-task`),
  cancelTask: (id: string) => api.request<{ status: string }>(`/api/tasks/${id}/cancel`, json("POST")),
  listTaskMessages: (id: string, cursor?: string) =>
    api.request<LightweightPage<LightweightTaskMessage>>(pagePath(`/api/tasks/${id}/messages`, { limit: "100", cursor })),

  listChatSessions: (cursor?: string) =>
    api.request<LightweightPage<LightweightChatSession>>(pagePath("/api/chat/sessions", { limit: "100", cursor })),
  getChatSession: (id: string) => api.request<LightweightChatSession>(`/api/chat/sessions/${id}`),
  createChatSession: (agentId: string, title: string) =>
    api.request<LightweightChatSession>("/api/chat/sessions", json("POST", { agent_id: agentId, title })),
  updateChatSession: (id: string, body: { title?: string; status?: "active" | "archived" }) =>
    api.request<LightweightChatSession>(`/api/chat/sessions/${id}`, json("PATCH", body)),
  deleteChatSession: (id: string) => api.request<void>(`/api/chat/sessions/${id}`, json("DELETE")),
  listChatMessages: (id: string, cursor?: string) =>
    api.request<LightweightPage<LightweightChatMessage>>(pagePath(`/api/chat/sessions/${id}/messages`, { limit: "100", cursor })),
  sendChatMessage: (id: string, content: string, key = newIdempotencyKey("chat")) =>
    api.request<{ message_id: string; task_id: string; created_at: string }>(`/api/chat/sessions/${id}/messages`, json("POST", { content }, { "Idempotency-Key": key })),
  getPendingChatTask: (id: string) => api.request<LightweightPendingTask>(`/api/chat/sessions/${id}/pending-task`),
  listDraftRestores: (id: string) => api.request<LightweightDraftRestore[]>(`/api/chat/sessions/${id}/draft-restores`),
  consumeDraftRestore: (sessionId: string, restoreId: string) =>
    api.request<void>(`/api/chat/sessions/${sessionId}/draft-restores/${restoreId}`, json("DELETE")),
};
