import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { Workspace } from "../types";

export const workspaceKeys = {
  list: () => ["workspaces", "list"] as const,
};

export function workspaceListOptions() {
  return queryOptions({ queryKey: workspaceKeys.list(), queryFn: () => api.listWorkspaces() });
}

export function workspaceBySlugOptions(slug: string) {
  return queryOptions({
    ...workspaceListOptions(),
    select: (workspaces: Workspace[]) => workspaces.find((workspace) => workspace.slug === slug) ?? null,
  });
}
