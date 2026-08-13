/** @vitest-environment jsdom */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AgentDetailPage } from "./agents-detail";

const api = vi.hoisted(() => ({
  getAgent: vi.fn(),
  getAgentSnapshot: vi.fn(),
  getAgentTasks: vi.fn(),
  listRuntimes: vi.fn(),
  listMembers: vi.fn(),
  listChatSessions: vi.fn(),
  listSkills: vi.fn(),
  listRuns: vi.fn(),
  listToolSources: vi.fn(),
  listToolSourceDefinitions: vi.fn(),
  getAgentToolBundle: vi.fn(),
  publishAgentToolBundle: vi.fn(),
  clearAgentToolBundle: vi.fn(),
  revokeToolBundle: vi.fn(),
}));

vi.mock("@dars/core/lightweight", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dars/core/lightweight")>();
  return {
    ...actual,
    lightweightApi: {
      ...actual.lightweightApi,
      getAgent: api.getAgent,
      getAgentSnapshot: api.getAgentSnapshot,
      getAgentTasks: api.getAgentTasks,
      listRuntimes: api.listRuntimes,
      listMembers: api.listMembers,
      listChatSessions: api.listChatSessions,
      listSkills: api.listSkills,
      listRuns: api.listRuns,
      listToolSources: api.listToolSources,
      listToolSourceDefinitions: api.listToolSourceDefinitions,
      getAgentToolBundle: api.getAgentToolBundle,
      publishAgentToolBundle: api.publishAgentToolBundle,
      clearAgentToolBundle: api.clearAgentToolBundle,
      revokeToolBundle: api.revokeToolBundle,
      listTaskMessages: vi.fn().mockResolvedValue({ items: [], next_cursor: null }),
    },
    lightweightAgentDetailOptions: (workspaceId: string, agentId: string) => ({
      queryKey: ["lightweight", workspaceId, "agent-detail", agentId],
      queryFn: () => api.getAgent(agentId),
      enabled: !!workspaceId && !!agentId,
    }),
    lightweightAgentSnapshotOptions: (workspaceId: string) => ({
      queryKey: ["lightweight", workspaceId, "agent-snapshot", "detail"],
      queryFn: api.getAgentSnapshot,
      enabled: !!workspaceId,
    }),
    lightweightAgentTasksOptions: (workspaceId: string, agentId: string, cursor?: string | null) => ({
      queryKey: ["lightweight", workspaceId, "agent-detail", agentId, "tasks", { cursor: cursor ?? null }],
      queryFn: () => api.getAgentTasks(agentId, cursor ?? undefined),
      enabled: !!workspaceId && !!agentId,
    }),
    toolSourcesOptions: (workspaceId: string) => ({
      queryKey: ["lightweight", workspaceId, "tools", "sources"],
      queryFn: api.listToolSources,
    }),
    toolDefinitionsOptions: (workspaceId: string, sourceId: string) => ({
      queryKey: ["lightweight", workspaceId, "tools", "sources", sourceId, "definitions"],
      queryFn: () => api.listToolSourceDefinitions(sourceId),
    }),
    agentToolBundleOptions: (workspaceId: string, agentId: string) => ({
      queryKey: ["lightweight", workspaceId, "tools", "agent-bundle", agentId],
      queryFn: () => api.getAgentToolBundle(agentId),
    }),
    toolKeys: {
      all: (workspaceId: string) => ["lightweight", workspaceId, "tools"],
      sources: (workspaceId: string) => ["lightweight", workspaceId, "tools", "sources"],
      agentBundle: (workspaceId: string, agentId: string) => ["lightweight", workspaceId, "tools", "agent-bundle", agentId],
    },
  };
});

const navigation = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
  pathname: "/test/agents/agent-1",
  searchParams: new URLSearchParams("view=overview"),
}));

vi.mock("../navigation", () => ({
  useNavigation: () => navigation,
  AppLink: ({ href, children }: { href: string; children: React.ReactNode }) => <a href={href}>{children}</a>,
}));

vi.mock("@dars/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "workspace-1", slug: "test" }),
  useWorkspacePaths: () => ({
    agents: () => "/test/agents",
    agentDetail: (id: string) => `/test/agents/${id}`,
    issueDetail: (id: string) => `/test/issues/${id}`,
    chat: () => "/test/chat",
    skillDetail: (id: string) => `/test/skills/${id}`,
  }),
}));

vi.mock("@dars/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) => selector({ user: { id: "user-1" } }),
}));

const agent = {
  id: "agent-1",
  workspace_id: "workspace-1",
  runtime_id: "runtime-1",
  owner_id: "user-1",
  name: "Reviewer",
  description: "Reviews code",
  instructions: "Be strict",
  runtime_config: {},
  status: "idle",
  max_concurrent_tasks: 1,
  custom_env_keys: [],
  custom_args: [],
  mcp_configured: false,
  model: "gpt-test",
  thinking_level: null,
  service_tier: null,
  permission_mode: "private",
  disabled_runtime_skills: [],
  archived_at: null,
  skills: [],
  invocation_targets: [],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
};

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <AgentDetailPage id="agent-1" />
    </QueryClientProvider>,
  );
}

describe("AgentDetailPage", () => {
  beforeEach(() => {
    cleanup();
    navigation.push.mockReset();
    navigation.searchParams = new URLSearchParams("view=overview");
    api.getAgent.mockResolvedValue(agent);
    api.getAgentSnapshot.mockResolvedValue({
      scope_counts: { mine: 1, all: 1, archived: 0 },
      agents: [],
      tasks: [],
      run_counts: [],
      activity: [],
      filter_metadata: { owners: [{ id: "user-1", name: "You" }], runtimes: [] },
    });
    api.getAgentTasks.mockResolvedValue({
      items: [],
      summary_30d: { run_count: 0, success_count: 0, fail_count: 0, success_rate: 0, avg_duration_ms: 0 },
    });
    api.listRuntimes.mockResolvedValue([
      {
        id: "runtime-1",
        workspace_id: "workspace-1",
        daemon_id: "daemon-1",
        name: "Local",
        provider: "codex",
        status: "online",
        device_info: "",
        metadata: {},
        owner_id: "user-1",
        profile_id: null,
        last_seen_at: null,
        created_at: "",
        updated_at: "",
      },
    ]);
    api.listMembers.mockResolvedValue([
      {
        id: "member-1",
        workspace_id: "workspace-1",
        user_id: "user-1",
        role: "owner",
        name: "You",
        email: "you@example.com",
        created_at: "",
      },
    ]);
    api.listChatSessions.mockResolvedValue({ items: [], next_cursor: null });
    api.listSkills.mockResolvedValue([]);
    api.listRuns.mockResolvedValue({ items: [], next_cursor: null });
    api.listToolSources.mockReset().mockResolvedValue([{
      id: "source-1", workspaceId: "workspace-1", name: "pet-api", kind: "openapi", enabled: true,
      currentRevision: "revision-1", revision: { id: "revision-1", revision: 1, status: "ready" },
      createdAt: "", updatedAt: "",
    }]);
    api.listToolSourceDefinitions.mockReset().mockResolvedValue([{
      id: "tool-1", publicName: "pet-api.listPets", upstreamName: "listPets", description: "List pets",
      inputSchema: {}, outputSchema: null, operationMetadata: {}, enabled: true,
    }]);
    api.getAgentToolBundle.mockReset().mockResolvedValue({
      bundle: {
        id: "tb_current", workspaceId: "workspace-1", manifestHash: "hash", status: "active",
        items: [{
          ordinal: 0, exportedName: "skill.pet.list", canonicalPublicName: "pet-api.listPets", sourceId: "source-1", sourceRevisionId: "revision-1",
          toolDefinitionId: "tool-1", definition: {},
        }],
        createdAt: "", revokedAt: null,
      },
    });
    api.publishAgentToolBundle.mockReset();
    api.clearAgentToolBundle.mockReset();
    api.revokeToolBundle.mockReset();
  });

  it("deep-links to the work tab", async () => {
    navigation.searchParams = new URLSearchParams("view=work&workScope=created");
    renderPage();
    expect(await screen.findByText("Scope")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Work" }).className).toContain("bg-secondary");
    expect(api.listRuns).toHaveBeenCalledWith(
      expect.objectContaining({ creatorType: "agent", creatorId: "agent-1" }),
    );
  });

  it("redirects retired MCP view state to overview content", async () => {
    navigation.searchParams = new URLSearchParams("view=mcp");
    renderPage();
    expect(await screen.findByText("30-day summary")).toBeTruthy();
    expect(screen.queryByText("MCP")).toBeNull();
  });

  it("navigates between tabs through the URL", async () => {
    renderPage();
    await screen.findByText("Recent work");
    fireEvent.click(screen.getByRole("button", { name: "Work" }));
    await waitFor(() => {
      expect(navigation.push).toHaveBeenCalled();
    });
    const pushed = navigation.push.mock.calls.at(-1)?.[0] as string;
    expect(pushed).toContain("view=work");
  });

  it("refetches agent detail after reconnect", async () => {
    renderPage();
    await screen.findByText("30-day summary");
    api.getAgent.mockClear();
    window.dispatchEvent(new Event("online"));
    await waitFor(() => {
      expect(api.getAgent).toHaveBeenCalled();
    });
  });

  it("restores the current immutable Bundle in Capabilities > Tools", async () => {
    navigation.searchParams = new URLSearchParams("view=capabilities&cap=tools");
    renderPage();
    expect(await screen.findByText(/tb_current/)).toBeTruthy();
    expect(await screen.findByText(/Canonical MCP name: pet-api\.listPets/)).toBeTruthy();
    expect((await screen.findByLabelText("Agent call name for pet-api.listPets") as HTMLInputElement).value).toBe("skill.pet.list");
    const checked = screen.getAllByRole("checkbox").filter((element) => (element as HTMLInputElement).checked);
    expect(checked.length).toBeGreaterThanOrEqual(2);
  });

  it("publishes a custom Agent call name and blocks invalid aliases", async () => {
    navigation.searchParams = new URLSearchParams("view=capabilities&cap=tools");
    api.publishAgentToolBundle.mockResolvedValue({
      id: "tb_alias", workspaceId: "workspace-1", manifestHash: "alias-hash", status: "active",
      items: [{
        ordinal: 0, exportedName: "skill.pet.get_pets", canonicalPublicName: "pet-api.listPets",
        sourceId: "source-1", sourceRevisionId: "revision-1", toolDefinitionId: "tool-1", definition: {},
      }],
      createdAt: "", revokedAt: null,
    });
    renderPage();
    const input = await screen.findByLabelText("Agent call name for pet-api.listPets");
    fireEvent.change(input, { target: { value: "invalid alias" } });
    expect((screen.getByRole("button", { name: "Publish 1 tools" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(input, { target: { value: "skill.pet.get_pets" } });
    fireEvent.click(screen.getByRole("button", { name: "Publish 1 tools" }));
    await waitFor(() => {
      expect(api.publishAgentToolBundle).toHaveBeenCalledWith("agent-1", [{
        toolDefinitionId: "tool-1",
        exportedName: "skill.pet.get_pets",
      }]);
    });
  });

  it("blocks duplicate Agent call names before publication", async () => {
    navigation.searchParams = new URLSearchParams("view=capabilities&cap=tools");
    api.listToolSourceDefinitions.mockResolvedValueOnce([
      {
        id: "tool-1", publicName: "pet-api.listPets", upstreamName: "listPets", description: "List pets",
        inputSchema: {}, outputSchema: null, operationMetadata: {}, enabled: true,
      },
      {
        id: "tool-2", publicName: "pet-api.createPet", upstreamName: "createPet", description: "Create pet",
        inputSchema: {}, outputSchema: null, operationMetadata: {}, enabled: true,
      },
    ]);
    renderPage();
    const secondTool = await screen.findByRole("checkbox", { name: "Select pet-api.createPet" });
    fireEvent.click(secondTool);
    fireEvent.change(screen.getByLabelText("Agent call name for pet-api.createPet"), {
      target: { value: "skill.pet.list" },
    });
    expect((screen.getByRole("button", { name: "Publish 2 tools" }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getAllByText("Agent call names must be unique in this Bundle.")).toHaveLength(2);
    expect(api.publishAgentToolBundle).not.toHaveBeenCalled();
  });

  it("warns when the current Bundle pins an older Source revision", async () => {
    navigation.searchParams = new URLSearchParams("view=capabilities&cap=tools");
    api.listToolSources.mockResolvedValueOnce([{
      id: "source-1", workspaceId: "workspace-1", name: "pet-api", kind: "openapi", enabled: true,
      currentRevision: "revision-2", revision: { id: "revision-2", revision: 2, status: "ready" },
      createdAt: "", updatedAt: "",
    }]);
    renderPage();
    expect(await screen.findByText(/newer Source revision/)).toBeTruthy();
    expect(await screen.findByText(/publish explicitly to update/)).toBeTruthy();
  });

  it("does not expose Agent Tools to ordinary members", async () => {
    navigation.searchParams = new URLSearchParams("view=capabilities&cap=tools");
    api.listMembers.mockResolvedValueOnce([{
      id: "member-1", workspace_id: "workspace-1", user_id: "user-1", role: "member",
      name: "You", email: "you@example.com", created_at: "",
    }]);
    renderPage();
    expect((await screen.findByRole("alert")).textContent).toContain("Workspace owner or admin permission is required");
    expect(api.listToolSources).not.toHaveBeenCalled();
  });
});
