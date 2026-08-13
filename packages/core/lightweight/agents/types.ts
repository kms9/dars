import type {
  LightweightAgent,
  LightweightAgentSnapshotItem,
  LightweightAgentSnapshotTask,
  LightweightRuntime,
} from "../types";

export type AgentAvailability = "online" | "unstable" | "offline" | "archived";

export type AgentWorkload = "working" | "queued" | "idle";

export type AgentPresenceDetail = {
  availability: AgentAvailability;
  workload: AgentWorkload;
  runningCount: number;
  queuedCount: number;
  capacity: number;
};

export type AccessScope = "workspace" | "specific-people" | "owner-only";

export type AgentPermissionScope = "private" | "workspace" | "members";

export type AgentsScope = "mine" | "all" | "archived";

export type AgentSortField = "lastActive" | "name" | "runs" | "created";

export type AgentSortDirection = "asc" | "desc";

export type AgentColumnKey =
  | "status"
  | "owner"
  | "access"
  | "runtime"
  | "lastActive"
  | "runs"
  | "model"
  | "created";

export type AgentListFilters = {
  availability: string[];
  runtimes: string[];
  owners: string[];
  models: string[];
  access: AccessScope[];
};

export type AgentInvocationTargetInput = {
  target_type: "workspace" | "member";
  target_id?: string;
};

export type AgentDraft = {
  name: string;
  description: string;
  instructions: string;
  avatarUrl: string | null;
  avatarFile: File | null;
  runtimeId: string;
  model: string;
  thinkingLevel: string;
  serviceTier: string;
  skillIds: Set<string>;
  permissionScope: AgentPermissionScope;
  memberIds: Set<string>;
  maxConcurrentTasks: number;
};

export type StoredAgentDraft = {
  name: string;
  description: string;
  instructions: string;
  avatar_url: string | null;
  model: string;
  thinking_level: string;
  service_tier: string;
  skill_ids: string[];
  permission_scope: AgentPermissionScope;
  member_ids: string[];
  max_concurrent_tasks: number;
};

export type AgentBuilderSessionSummary = {
  session_id: string;
  title: string;
  runtime_id: string;
  created_at: string;
  updated_at: string;
  last_message_content: string;
  last_message_role: string;
  last_message_at: string;
  draft?: StoredAgentDraft;
};

export type AgentBuilderSession = {
  session_id: string;
  builder_agent_id: string;
  runtime_id: string;
};

export type ManualDraftEntry = {
  runtimeId: string;
  draft: StoredAgentDraft;
};

export type ManualAgentDrafts = {
  byOwner: Record<string, ManualDraftEntry>;
};

export type AgentListRow = {
  agent: LightweightAgentSnapshotItem;
  detail: LightweightAgent | null;
  runtime: LightweightRuntime | null;
  presence: AgentPresenceDetail | null;
  runCount: number;
  lastActiveAt: string | null;
  ownerName: string;
  isOwnedByMe: boolean;
  canManage: boolean;
};

export type BatchItemResult = {
  id: string;
  ok: boolean;
  error?: string;
};

export type BatchOperationResult = {
  results: BatchItemResult[];
  succeeded: number;
  failed: number;
};

export const ACTIVE_TASK_STATUSES = new Set([
  "queued",
  "dispatched",
  "waiting_local_directory",
  "running",
]);

export type { LightweightAgentSnapshotTask as AgentSnapshotTask };
