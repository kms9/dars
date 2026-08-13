const encode = encodeURIComponent;

function workspaceScoped(slug: string) {
  const root = `/${encode(slug)}`;
  return {
    root: () => `${root}/issues`,
    issues: () => `${root}/issues`,
    runs: () => `${root}/issues`,
    issueDetail: (id: string) => `${root}/issues/${encode(id)}`,
    agents: () => `${root}/agents`,
    newAgent: () => `${root}/agents/new`,
    newAgentBlank: () => `${root}/agents/new/blank`,
    newAgentAi: () => `${root}/agents/new/ai`,
    newAgentAiSession: (sessionId: string) => `${root}/agents/new/ai/${encode(sessionId)}`,
    agentDetail: (id: string) => `${root}/agents/${encode(id)}`,
    squads: () => `${root}/squads`,
    squadDetail: (id: string) => `${root}/squads/${encode(id)}`,
    chat: () => `${root}/chat`,
    runtimes: () => `${root}/runtimes`,
    runtimeProfiles: () => `${root}/runtimes`,
    runtimeDetail: (id: string) => `${root}/runtimes/${encode(id)}`,
    skills: () => `${root}/skills`,
    skillDetail: (id: string) => `${root}/skills/${encode(id)}`,
    tools: () => `${root}/tools`,
    toolDetail: (id: string) => `${root}/tools/${encode(id)}`,
    settings: () => `${root}/settings`,
  };
}

export const paths = {
  workspace: workspaceScoped,
  login: () => "/login",
  newWorkspace: () => "/workspaces/new",
  root: () => "/",
};

export type WorkspacePaths = ReturnType<typeof workspaceScoped>;

export function isGlobalPath(path: string): boolean {
  return path === "/" || path === "/login" || path.startsWith("/workspaces/");
}
