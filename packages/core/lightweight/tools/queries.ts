import { queryOptions } from "@tanstack/react-query";
import { lightweightApi } from "../api";

export const toolKeys = {
  all: (workspaceId: string) => ["lightweight", workspaceId, "tools"] as const,
  sources: (workspaceId: string) => [...toolKeys.all(workspaceId), "sources"] as const,
  source: (workspaceId: string, sourceId: string) => [...toolKeys.sources(workspaceId), sourceId] as const,
  definitions: (workspaceId: string, sourceId: string) => [...toolKeys.source(workspaceId, sourceId), "definitions"] as const,
  agentBundle: (workspaceId: string, agentId: string) => [...toolKeys.all(workspaceId), "agent-bundle", agentId] as const,
};

export function toolSourcesOptions(workspaceId: string) {
  return queryOptions({
    queryKey: toolKeys.sources(workspaceId),
    queryFn: lightweightApi.listToolSources,
  });
}

export function toolSourceOptions(workspaceId: string, sourceId: string) {
  return queryOptions({
    queryKey: toolKeys.source(workspaceId, sourceId),
    queryFn: () => lightweightApi.getToolSource(sourceId),
  });
}

export function toolDefinitionsOptions(workspaceId: string, sourceId: string) {
  return queryOptions({
    queryKey: toolKeys.definitions(workspaceId, sourceId),
    queryFn: () => lightweightApi.listToolSourceDefinitions(sourceId),
  });
}

export function agentToolBundleOptions(workspaceId: string, agentId: string) {
  return queryOptions({
    queryKey: toolKeys.agentBundle(workspaceId, agentId),
    queryFn: () => lightweightApi.getAgentToolBundle(agentId),
  });
}
