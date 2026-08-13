import type {
  LightweightAgentActivityBucket,
  LightweightAgentSnapshot,
  LightweightAgentSnapshotItem,
  LightweightAgentSnapshotTask,
  LightweightRuntime,
} from "../types";
import { effectiveAccessScope } from "./effective-access";
import { deriveAgentPresenceDetail } from "./derive-presence";
import type {
  AgentListFilters,
  AgentListRow,
  AgentSortDirection,
  AgentSortField,
  AgentsScope,
} from "./types";

export function matchesAgentSearch(agent: LightweightAgentSnapshotItem, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  return [agent.name, agent.description].some((value) => value.toLowerCase().includes(needle));
}

export function rowMatchesFilters(
  row: AgentListRow,
  filters: AgentListFilters,
  query: string,
): boolean {
  if (!matchesAgentSearch(row.agent, query)) return false;
  if (
    filters.availability.length > 0 &&
    (!row.presence || !filters.availability.includes(row.presence.availability))
  ) {
    return false;
  }
  if (
    filters.runtimes.length > 0 &&
    (!row.agent.runtime_id || !filters.runtimes.includes(row.agent.runtime_id))
  ) {
    return false;
  }
  if (filters.owners.length > 0 && !filters.owners.includes(row.agent.owner_id)) return false;
  if (
    filters.models.length > 0 &&
    (!row.agent.model || !filters.models.includes(row.agent.model))
  ) {
    return false;
  }
  if (filters.access.length > 0) {
    const scope = effectiveAccessScope(row.agent.permission_mode, row.detail?.invocation_targets);
    if (!filters.access.includes(scope)) return false;
  }
  return true;
}

export function scopeMatches(agent: LightweightAgentSnapshotItem, scope: AgentsScope, currentUserId: string | null): boolean {
  const archived = !!agent.archived_at;
  if (scope === "archived") return archived;
  if (archived) return false;
  if (scope === "mine") return !!currentUserId && agent.owner_id === currentUserId;
  return true;
}

export function lastActiveFromActivity(
  agentId: string,
  activity: readonly LightweightAgentActivityBucket[],
): string | null {
  let latest: string | null = null;
  for (const bucket of activity) {
    if (bucket.agent_id !== agentId || bucket.task_count <= 0) continue;
    if (!latest || bucket.bucket_at > latest) latest = bucket.bucket_at;
  }
  return latest;
}

export function buildAgentListRows(input: {
  snapshot: LightweightAgentSnapshot;
  runtimes: readonly LightweightRuntime[];
  currentUserId: string | null;
  memberRole: "owner" | "admin" | "member" | null;
  detailsById?: ReadonlyMap<string, import("../types").LightweightAgent>;
}): AgentListRow[] {
  const runtimesById = new Map(input.runtimes.map((runtime) => [runtime.id, runtime]));
  const runCounts = new Map(input.snapshot.run_counts.map((row) => [row.agent_id, row.run_count]));
  const tasksByAgent = new Map<string, LightweightAgentSnapshotTask[]>();
  for (const task of input.snapshot.tasks) {
    const list = tasksByAgent.get(task.agent_id);
    if (list) list.push(task);
    else tasksByAgent.set(task.agent_id, [task]);
  }
  const ownersById = new Map(input.snapshot.filter_metadata.owners.map((owner) => [owner.id, owner.name]));

  return input.snapshot.agents.map((agent) => {
    const runtime = agent.runtime_id ? runtimesById.get(agent.runtime_id) ?? null : null;
    const presence = deriveAgentPresenceDetail({
      runtime,
      tasks: tasksByAgent.get(agent.id) ?? [],
      archived: !!agent.archived_at,
      maxConcurrentTasks: agent.max_concurrent_tasks,
    });
    const canManage =
      input.memberRole === "owner" ||
      input.memberRole === "admin" ||
      (!!input.currentUserId && agent.owner_id === input.currentUserId);
    return {
      agent,
      detail: input.detailsById?.get(agent.id) ?? null,
      runtime,
      presence,
      runCount: runCounts.get(agent.id) ?? 0,
      lastActiveAt: lastActiveFromActivity(agent.id, input.snapshot.activity),
      ownerName: ownersById.get(agent.owner_id) ?? agent.owner_id,
      isOwnedByMe: !!input.currentUserId && agent.owner_id === input.currentUserId,
      canManage,
    };
  });
}

function compareStrings(a: string | null | undefined, b: string | null | undefined): number {
  return (a ?? "").localeCompare(b ?? "");
}

export function sortAgentRows(
  rows: AgentListRow[],
  field: AgentSortField,
  direction: AgentSortDirection,
): AgentListRow[] {
  const factor = direction === "asc" ? 1 : -1;
  return [...rows].sort((left, right) => {
    let result = 0;
    switch (field) {
      case "name":
        result = compareStrings(left.agent.name, right.agent.name);
        break;
      case "runs":
        result = left.runCount - right.runCount;
        break;
      case "created":
        result = compareStrings(left.agent.updated_at, right.agent.updated_at);
        break;
      case "lastActive":
      default:
        result = compareStrings(left.lastActiveAt, right.lastActiveAt);
        break;
    }
    if (result === 0) result = compareStrings(left.agent.id, right.agent.id);
    return result * factor;
  });
}

export function paginateRows<T>(rows: T[], page: number, pageSize: number): {
  pageRows: T[];
  totalPages: number;
  page: number;
} {
  const totalPages = Math.max(1, Math.ceil(rows.length / pageSize));
  const safePage = Math.min(Math.max(page, 1), totalPages);
  const start = (safePage - 1) * pageSize;
  return {
    pageRows: rows.slice(start, start + pageSize),
    totalPages,
    page: safePage,
  };
}

export async function runBatchOperation<T extends { id: string }>(
  items: T[],
  operation: (item: T) => Promise<unknown>,
): Promise<{ results: Array<{ id: string; ok: boolean; error?: string }>; succeeded: number; failed: number }> {
  const settled = await Promise.allSettled(items.map((item) => operation(item)));
  const results = items.map((item, index) => {
    const outcome = settled[index];
    if (outcome?.status === "fulfilled") return { id: item.id, ok: true };
    const error = outcome?.reason instanceof Error ? outcome.reason.message : "Request failed";
    return { id: item.id, ok: false, error };
  });
  const failed = results.filter((result) => !result.ok).length;
  return { results, succeeded: results.length - failed, failed };
}
