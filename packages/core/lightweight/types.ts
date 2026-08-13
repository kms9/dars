export type LightweightPage<T> = {
  items: T[];
  next_cursor: string | null;
};

export type LightweightMember = {
  id: string;
  workspace_id: string;
  user_id: string;
  role: "owner" | "admin" | "member";
  name: string;
  email: string;
  created_at: string;
};

export type LightweightRuntime = {
  id: string;
  workspace_id: string;
  daemon_id: string;
  name: string;
  provider: string;
  status: string;
  device_info: string;
  metadata: Record<string, unknown>;
  owner_id: string;
  profile_id: string | null;
  last_seen_at: string | null;
  created_at: string;
  updated_at: string;
};

export type LightweightRuntimeProfile = {
  id: string;
  workspace_id: string;
  display_name: string;
  protocol_family: string;
  command_name: string;
  description: string | null;
  fixed_args: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

export type LightweightSkill = {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  content: string;
  config: Record<string, unknown>;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type LightweightSkillFile = {
  id: string;
  skill_id: string;
  path: string;
  content: string;
  created_at: string;
  updated_at: string;
};

export type LightweightSkillOnConflict = "fail" | "overwrite" | "rename" | "skip";

export type LightweightSkillImportExisting = {
  id: string;
  name: string;
  created_by?: string;
  can_overwrite?: boolean;
};

export type LightweightSkillImportResult = {
  status: "created" | "updated" | "skipped" | "conflict" | "failed";
  reason?: string;
  skill?: LightweightSkill & { files?: LightweightSkillFile[] };
  existing_skill?: LightweightSkillImportExisting;
};

export type LightweightSkillSearchCandidate = {
  name: string;
  url: string;
  source: string;
  repo?: string | null;
  install_count?: number | null;
  github_stars?: number | null;
  description: string;
};

export type LightweightRuntimeLocalSkillStatus =
  | "pending"
  | "running"
  | "completed"
  | "conflict"
  | "failed"
  | "timeout";

export type LightweightRuntimeLocalSkillSummary = {
  key: string;
  name: string;
  description?: string;
  source_path: string;
  provider: string;
  root?: "provider" | "universal" | "plugin" | string;
  plugin?: string;
  can_disable?: boolean;
  file_count: number;
};

export type LightweightRuntimeLocalSkillListRequest = {
  id: string;
  runtime_id: string;
  status: LightweightRuntimeLocalSkillStatus;
  skills?: LightweightRuntimeLocalSkillSummary[];
  supported: boolean;
  mcp_servers?: Array<{
    name: string;
    transport?: string;
    source?: string;
    enabled: boolean;
  }>;
  mcp_supported?: boolean;
  error?: string;
  created_at: string;
  updated_at: string;
};

export type LightweightRuntimeLocalSkillImportAction = "overwrite";

export type LightweightCreateRuntimeLocalSkillImportRequest = {
  skill_key: string;
  name?: string;
  description?: string;
  action?: LightweightRuntimeLocalSkillImportAction;
  target_skill_id?: string;
  supports_conflict?: boolean;
};

export type LightweightRuntimeLocalSkillImportConflict = {
  existing_skill_id: string;
  existing_created_by?: string;
  can_overwrite: boolean;
};

export type LightweightRuntimeLocalSkillImportRequest = {
  id: string;
  runtime_id: string;
  skill_key: string;
  name?: string;
  description?: string;
  action?: LightweightRuntimeLocalSkillImportAction;
  target_skill_id?: string;
  supports_conflict?: boolean;
  status: LightweightRuntimeLocalSkillStatus;
  skill?: LightweightSkill & { files?: LightweightSkillFile[] };
  conflict?: LightweightRuntimeLocalSkillImportConflict;
  error?: string;
  created_at: string;
  updated_at: string;
};

export type LightweightRuntimeLocalSkillsResult = {
  skills: LightweightRuntimeLocalSkillSummary[];
  supported: boolean;
};

export type LightweightRuntimeLocalSkillImportResult = {
  status: "created" | "updated" | "conflict";
  skill?: LightweightSkill;
  conflict?: LightweightRuntimeLocalSkillImportConflict;
};

export type LightweightAgentSkill = {
  id: string;
  name: string;
  enabled: boolean;
};

export type LightweightDisabledRuntimeSkill = {
  runtime_id: string;
  provider: string;
  root: string;
  key: string;
  name?: string;
  plugin?: string;
};

export type LightweightAgentTask = {
  id: string;
  workspace_id: string;
  agent_id: string;
  runtime_id: string;
  issue_id?: string;
  chat_session_id?: string;
  squad_id?: string;
  status: string;
  failure_reason?: string;
  error?: string;
  trigger_summary?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
};

export type LightweightAgentTaskSummary30d = {
  run_count: number;
  success_count: number;
  fail_count: number;
  success_rate: number;
  avg_duration_ms: number;
};

export type LightweightAgentTasksPage = {
  items: LightweightAgentTask[];
  next_cursor?: string | null;
  summary_30d: LightweightAgentTaskSummary30d;
};

export type LightweightInvocationTarget = {
  id: string;
  target_type: "workspace" | "member";
  target_id: string;
};

export type LightweightAgentSnapshotScopeCounts = {
  mine: number;
  all: number;
  archived: number;
};

export type LightweightAgentSnapshotItem = {
  id: string;
  workspace_id: string;
  owner_id: string;
  name: string;
  description: string;
  avatar_url: string | null;
  runtime_id: string | null;
  status: string;
  permission_mode: string;
  model: string | null;
  thinking_level: string | null;
  service_tier: string | null;
  max_concurrent_tasks: number;
  archived_at: string | null;
  updated_at: string;
};

export type LightweightAgentSnapshotTask = {
  id: string;
  agent_id: string;
  status: string;
  issue_id?: string | null;
  chat_session_id?: string | null;
  failure_reason?: string | null;
  started_at?: string | null;
  completed_at?: string | null;
  created_at: string;
};

export type LightweightAgentRunCount = {
  agent_id: string;
  run_count: number;
};

export type LightweightAgentActivityBucket = {
  agent_id: string;
  bucket_at: string;
  task_count: number;
  failed_count: number;
};

export type LightweightAgentFilterOwner = {
  id: string;
  name: string;
};

export type LightweightAgentFilterRuntime = {
  id: string;
  name: string;
  status: string;
};

export type LightweightAgentSnapshot = {
  scope_counts: LightweightAgentSnapshotScopeCounts;
  agents: LightweightAgentSnapshotItem[];
  tasks: LightweightAgentSnapshotTask[];
  run_counts: LightweightAgentRunCount[];
  activity: LightweightAgentActivityBucket[];
  filter_metadata: {
    owners: LightweightAgentFilterOwner[];
    runtimes: LightweightAgentFilterRuntime[];
  };
};

export type LightweightAgent = {
  id: string;
  workspace_id: string;
  runtime_id: string | null;
  owner_id: string;
  name: string;
  description: string;
  instructions: string;
  avatar_url?: string | null;
  runtime_config: Record<string, unknown>;
  status: string;
  max_concurrent_tasks: number;
  custom_env_keys: string[];
  custom_args: unknown[];
  mcp_configured: boolean;
  model: string | null;
  thinking_level: string | null;
  service_tier: string | null;
  permission_mode: string;
  disabled_runtime_skills: LightweightDisabledRuntimeSkill[];
  archived_at: string | null;
  skills: LightweightAgentSkill[];
  invocation_targets: LightweightInvocationTarget[];
  created_at: string;
  updated_at: string;
};

export type LightweightSquad = {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  instructions: string;
  avatar_url: string | null;
  leader_id: string;
  creator_id: string;
  archived_at: string | null;
  archived_by: string | null;
  member_count: number;
  member_preview: Array<{ agent_id: string; name: string; role: "leader" | "member" }>;
  created_at: string;
  updated_at: string;
};

export type LightweightSquadMember = {
  id: string;
  squad_id: string;
  agent_id: string;
  name: string;
  role: "leader" | "member";
  status: string;
  archived_at: string | null;
  created_at: string;
};

export type LightweightSquadMemberStatus = {
  agent_id: string;
  status: "working" | "idle" | "unstable" | "offline" | "archived";
  last_active_at: string | null;
  active_issue_id: string | null;
  active_issue_title: string | null;
};

export const LIGHTWEIGHT_RUN_STATUSES = [
  "backlog",
  "todo",
  "in_progress",
  "in_review",
  "done",
  "blocked",
  "cancelled",
] as const;
export type LightweightRunStatus = (typeof LIGHTWEIGHT_RUN_STATUSES)[number];

export type LightweightRun = {
  id: string;
  workspace_id: string;
  identifier: string;
  title: string;
  description: string;
  status: LightweightRunStatus;
  assignee_type: "agent" | "squad";
  assignee_id: string;
  creator_type: string;
  creator_id: string;
  acceptance_criteria: unknown[];
  context_refs: unknown[];
  number: number;
  first_executed_at: string | null;
  created_at: string;
  updated_at: string;
};

export type LightweightComment = {
  id: string;
  workspace_id: string;
  issue_id: string;
  author_type: string;
  author_id: string | null;
  author_display: string;
  content: string;
  type: string;
  source_task_id: string | null;
  created_at: string;
};

export type LightweightTaskUsage = {
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cache_read_tokens: number;
  cache_write_tokens: number;
  cost_usd_ticks: number | null;
};

export type LightweightTaskRun = {
  id: string;
  workspace_id: string;
  issue_id: string;
  agent: { id: string; name: string };
  runtime: { id: string; name: string };
  squad: { id: string; name: string } | null;
  is_leader_task: boolean;
  status: string;
  priority: number;
  attempt: number;
  max_attempts: number;
  parent_task_id: string | null;
  session_id: string | null;
  work_dir: string | null;
  result: unknown;
  error: string | null;
  failure_reason: string | null;
  trigger_summary: string | null;
  wait_reason: string | null;
  usage: LightweightTaskUsage[];
  dispatched_at: string | null;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
};

export type LightweightTaskMessage = {
  id: string;
  task_id: string;
  seq: number;
  type: string;
  tool: string | null;
  content: string | null;
  input: Record<string, unknown> | null;
  output: string | null;
  created_at: string;
};

export type LightweightChatSession = {
  id: string;
  workspace_id: string;
  agent_id: string;
  creator_id: string;
  runtime_id: string;
  title: string;
  session_id: string | null;
  work_dir: string | null;
  status: "active" | "archived";
  created_at: string;
  updated_at: string;
};

export type LightweightChatMessage = {
  id: string;
  chat_session_id: string;
  role: "user" | "assistant" | "system";
  content: string;
  task_id: string | null;
  failure_reason: string | null;
  elapsed_ms: number | null;
  message_kind: string;
  created_at: string;
};

export type LightweightDraftRestore = {
  id: string;
  chat_session_id: string;
  task_id: string;
  content: string;
  created_at: string;
};

export type LightweightPendingTask = {
  task_id?: string;
  status?: string;
  created_at?: string;
};
