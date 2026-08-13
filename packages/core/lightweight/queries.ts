export const lightweightKeys = {
  all: (workspaceId: string) => ["lightweight", workspaceId] as const,
  runtimes: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "runtimes"] as const,
  profiles: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "runtime-profiles"] as const,
  agents: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "agents"] as const,
  skills: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "skills"] as const,
  squads: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "squads"] as const,
  runs: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "runs"] as const,
  chats: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "chats"] as const,
  taskMessages: (workspaceId: string, taskId: string) => [...lightweightKeys.all(workspaceId), "task-messages", taskId] as const,
  members: (workspaceId: string) => [...lightweightKeys.all(workspaceId), "members"] as const,
};
