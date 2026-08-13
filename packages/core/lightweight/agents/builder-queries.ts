import { queryOptions } from "@tanstack/react-query";
import { lightweightApi } from "../api";

export const lightweightAgentBuilderKeys = {
  all: (workspaceId: string) => ["lightweight", workspaceId, "agent-builder"] as const,
  sessions: (workspaceId: string) => [...lightweightAgentBuilderKeys.all(workspaceId), "sessions"] as const,
};

export function lightweightAgentBuilderSessionsOptions(workspaceId: string) {
  return queryOptions({
    queryKey: lightweightAgentBuilderKeys.sessions(workspaceId),
    queryFn: lightweightApi.listAgentBuilderSessions,
    enabled: !!workspaceId,
  });
}
