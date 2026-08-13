import { queryOptions } from "@tanstack/react-query";
import { lightweightApi } from "../api";

export const lightweightAgentSnapshotKeys = {
  all: (workspaceId: string) => ["lightweight", workspaceId, "agent-snapshot"] as const,
  detail: (workspaceId: string) => [...lightweightAgentSnapshotKeys.all(workspaceId), "detail"] as const,
};

export const lightweightAgentDetailKeys = {
  all: (workspaceId: string) => ["lightweight", workspaceId, "agent-detail"] as const,
  one: (workspaceId: string, agentId: string) =>
    [...lightweightAgentDetailKeys.all(workspaceId), agentId] as const,
};

export function lightweightAgentSnapshotOptions(workspaceId: string) {
  return queryOptions({
    queryKey: lightweightAgentSnapshotKeys.detail(workspaceId),
    queryFn: lightweightApi.getAgentSnapshot,
    enabled: !!workspaceId,
    staleTime: 30_000,
  });
}

export function lightweightAgentDetailOptions(workspaceId: string, agentId: string) {
  return queryOptions({
    queryKey: lightweightAgentDetailKeys.one(workspaceId, agentId),
    queryFn: () => lightweightApi.getAgent(agentId),
    enabled: !!workspaceId && !!agentId,
  });
}

export const lightweightAgentTasksKeys = {
  all: (workspaceId: string, agentId: string) =>
    [...lightweightAgentDetailKeys.one(workspaceId, agentId), "tasks"] as const,
  page: (workspaceId: string, agentId: string, cursor?: string | null) =>
    [...lightweightAgentTasksKeys.all(workspaceId, agentId), { cursor: cursor ?? null }] as const,
};

export function lightweightAgentTasksOptions(
  workspaceId: string,
  agentId: string,
  cursor?: string | null,
) {
  return queryOptions({
    queryKey: lightweightAgentTasksKeys.page(workspaceId, agentId, cursor),
    queryFn: () => lightweightApi.getAgentTasks(agentId, cursor ?? undefined),
    enabled: !!workspaceId && !!agentId,
  });
}
