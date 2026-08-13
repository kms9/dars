import type { ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { NavigationProvider } from "../navigation";
import { SkillsCreatePanel } from "./skills-create";
import { SkillsPage, SkillDetailPage } from "./skills";

const api = vi.hoisted(() => ({
  listSkills: vi.fn(),
  createSkill: vi.fn(),
  importSkill: vi.fn(),
  importSkillArchive: vi.fn(),
  searchSkills: vi.fn(),
  listRuntimes: vi.fn(),
  getSkill: vi.fn(),
  listSkillFiles: vi.fn(),
  updateSkill: vi.fn(),
  replaceSkillFiles: vi.fn(),
  deleteSkill: vi.fn(),
  listAgents: vi.fn(),
  setAgentSkills: vi.fn(),
  initiateListLocalSkills: vi.fn(),
  getListLocalSkillsResult: vi.fn(),
  initiateImportLocalSkill: vi.fn(),
  getImportLocalSkillResult: vi.fn(),
}));

const resolveRuntimeLocalSkills = vi.hoisted(() => vi.fn());
const resolveRuntimeLocalSkillImport = vi.hoisted(() => vi.fn());
const unwrapSkillImportResponse = vi.hoisted(() =>
  vi.fn((result: unknown) => {
    if (!result || typeof result !== "object") return {};
    const value = result as Record<string, unknown>;
    if (typeof value.id === "string") return { skill: value, status: "created" };
    return {
      status: typeof value.status === "string" ? value.status : undefined,
      reason: typeof value.reason === "string" ? value.reason : undefined,
      skill: value.skill as { id: string; name: string } | undefined,
    };
  }),
);

vi.mock("@dars/core/lightweight", () => ({
  lightweightApi: api,
  lightweightKeys: {
    skills: (workspaceId: string) => ["lightweight", workspaceId, "skills"],
    agents: (workspaceId: string) => ["lightweight", workspaceId, "agents"],
    runtimes: (workspaceId: string) => ["lightweight", workspaceId, "runtimes"],
  },
  resolveRuntimeLocalSkills,
  resolveRuntimeLocalSkillImport,
  unwrapSkillImportResponse,
}));

vi.mock("@dars/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "workspace-1", slug: "test" }),
  useWorkspacePaths: () => ({
    skills: () => "/test/skills",
    skillDetail: (id: string) => `/test/skills/${id}`,
  }),
}));

vi.mock("@dars/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } | null }) => unknown) =>
    selector({ user: { id: "user-1" } }),
}));

vi.mock("@dars/core/api", () => ({
  ApiError: class ApiError extends Error {
    constructor(
      message: string,
      readonly status: number,
    ) {
      super(message);
      this.name = "ApiError";
    }
  },
}));

const navigation = {
  push: vi.fn(),
  replace: vi.fn(),
  back: vi.fn(),
  pathname: "/test/skills",
  searchParams: new URLSearchParams(),
  getShareableUrl: (path: string) => path,
};

function renderWithProviders(ui: ReactElement) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <NavigationProvider value={navigation}>{ui}</NavigationProvider>
    </QueryClientProvider>,
  );
  return { ...view, queryClient };
}

describe("Skills create/import UX", () => {
  beforeEach(() => {
    api.listSkills.mockResolvedValue([
      {
        id: "skill-1",
        workspace_id: "workspace-1",
        name: "Alpha",
        description: "First skill",
        content: "# Alpha",
        config: { origin: { type: "github" } },
        created_by: "user-1",
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      },
      {
        id: "skill-2",
        workspace_id: "workspace-1",
        name: "Beta",
        description: "Second skill",
        content: "# Beta",
        config: {},
        created_by: "user-1",
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      },
    ]);
    api.createSkill.mockResolvedValue({
      id: "skill-new",
      workspace_id: "workspace-1",
      name: "New",
      description: "",
      content: "# New",
      config: {},
      created_by: "user-1",
      created_at: "2026-08-06T00:00:00Z",
      updated_at: "2026-08-06T00:00:00Z",
    });
    api.importSkill.mockResolvedValue({
      id: "skill-imported",
      workspace_id: "workspace-1",
      name: "Imported",
      description: "",
      content: "# Imported",
      config: { origin: { type: "clawhub" } },
      created_by: "user-1",
      created_at: "2026-08-06T00:00:00Z",
      updated_at: "2026-08-06T00:00:00Z",
    });
    api.listRuntimes.mockResolvedValue([
      {
        id: "runtime-1",
        workspace_id: "workspace-1",
        daemon_id: "daemon-1",
        name: "Mine",
        provider: "claude",
        status: "online",
        device_info: "",
        metadata: {},
        owner_id: "user-1",
        profile_id: null,
        last_seen_at: null,
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      },
      {
        id: "runtime-2",
        workspace_id: "workspace-1",
        daemon_id: "daemon-2",
        name: "Theirs",
        provider: "codex",
        status: "online",
        device_info: "",
        metadata: {},
        owner_id: "user-2",
        profile_id: null,
        last_seen_at: null,
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      },
    ]);
    resolveRuntimeLocalSkills.mockResolvedValue({
      skills: [
        {
          key: "local-1",
          name: "Local Skill",
          description: "From disk",
          source_path: "/skills/local",
          provider: "claude",
          file_count: 1,
        },
      ],
      supported: true,
    });
    resolveRuntimeLocalSkillImport.mockResolvedValue({
      status: "created",
      skill: {
        id: "skill-local",
        workspace_id: "workspace-1",
        name: "Local Skill",
        description: "From disk",
        content: "# Local",
        config: {},
        created_by: "user-1",
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      },
    });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("shows Manual / URL / Runtime create chooser", () => {
    const { queryClient } = renderWithProviders(<SkillsCreatePanel />);
    expect(screen.getByTestId("create-method-manual")).toBeTruthy();
    expect(screen.getByTestId("create-method-url")).toBeTruthy();
    expect(screen.getByTestId("create-method-runtime")).toBeTruthy();
    queryClient.clear();
  });

  it("imports a skill from URL with on_conflict", async () => {
    const { queryClient } = renderWithProviders(<SkillsCreatePanel />);
    fireEvent.click(screen.getByTestId("create-method-url"));
    fireEvent.change(screen.getByLabelText("Skill URL"), {
      target: { value: "https://clawhub.ai/owner/skill" },
    });
    fireEvent.change(screen.getByLabelText("On conflict"), { target: { value: "rename" } });
    fireEvent.click(screen.getByRole("button", { name: "Import from URL" }));

    await waitFor(() => {
      expect(api.importSkill).toHaveBeenCalledWith({
        url: "https://clawhub.ai/owner/skill",
        on_conflict: "rename",
      });
    });
    await waitFor(() => {
      expect(navigation.push).toHaveBeenCalledWith("/test/skills/skill-imported");
    });
    queryClient.clear();
  });

  it("creates a skill manually", async () => {
    const { queryClient } = renderWithProviders(<SkillsCreatePanel />);
    fireEvent.click(screen.getByTestId("create-method-manual"));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Manual Skill" } });
    fireEvent.change(screen.getByLabelText("SKILL.md content"), { target: { value: "# Manual" } });
    fireEvent.click(screen.getByRole("button", { name: "Create skill" }));

    await waitFor(() => {
      expect(api.createSkill).toHaveBeenCalledWith({
        name: "Manual Skill",
        description: "",
        content: "# Manual",
        config: {},
      });
    });
    queryClient.clear();
  });

  it("discovers and imports selected runtime local skills for the owner", async () => {
    const { queryClient } = renderWithProviders(<SkillsCreatePanel />);
    fireEvent.click(screen.getByTestId("create-method-runtime"));

    await screen.findByText("Local Skill");
    fireEvent.click(screen.getByLabelText(/Local Skill/));
    fireEvent.click(screen.getByRole("button", { name: /Import selected/ }));

    await waitFor(() => {
      expect(resolveRuntimeLocalSkillImport).toHaveBeenCalledWith(
        "runtime-1",
        expect.objectContaining({ skill_key: "local-1", supports_conflict: true }),
      );
    });
    queryClient.clear();
  });

  it("shows owner-only message when no owned online runtimes exist", async () => {
    api.listRuntimes.mockResolvedValue([
      {
        id: "runtime-2",
        workspace_id: "workspace-1",
        daemon_id: "daemon-2",
        name: "Theirs",
        provider: "codex",
        status: "online",
        device_info: "",
        metadata: {},
        owner_id: "user-2",
        profile_id: null,
        last_seen_at: null,
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      },
    ]);
    const { queryClient } = renderWithProviders(<SkillsCreatePanel />);
    fireEvent.click(screen.getByTestId("create-method-runtime"));
    expect(await screen.findByTestId("runtime-import-owner-only")).toBeTruthy();
    queryClient.clear();
  });

  it("filters the skills list by name/description/origin", async () => {
    const { queryClient } = renderWithProviders(<SkillsPage />);
    await screen.findByText("Alpha");
    expect(screen.getByText("Beta")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Search skills"), { target: { value: "github" } });
    expect(screen.getByText("Alpha")).toBeTruthy();
    expect(screen.queryByText("Beta")).toBeNull();
    queryClient.clear();
  });
});

describe("Skill detail management", () => {
  beforeEach(() => {
    api.getSkill.mockResolvedValue({
      id: "skill-1",
      workspace_id: "workspace-1",
      name: "Alpha",
      description: "First skill",
      content: "# Alpha",
      config: {},
      created_by: "user-1",
      created_at: "2026-08-06T00:00:00Z",
      updated_at: "2026-08-06T00:00:00Z",
    });
    api.listSkillFiles.mockResolvedValue([
      { id: "file-1", skill_id: "skill-1", path: "SKILL.md", content: "# Alpha", created_at: "", updated_at: "" },
      { id: "file-2", skill_id: "skill-1", path: "notes.md", content: "notes", created_at: "", updated_at: "" },
    ]);
    api.listAgents.mockResolvedValue([
      {
        id: "agent-1",
        workspace_id: "workspace-1",
        runtime_id: "runtime-1",
        owner_id: "user-1",
        name: "Agent One",
        description: "",
        instructions: "",
        runtime_config: {},
        status: "idle",
        max_concurrent_tasks: 1,
        custom_env_keys: [],
        custom_args: [],
        mcp_configured: false,
        model: null,
        thinking_level: null,
        service_tier: null,
        permission_mode: "private",
        disabled_runtime_skills: [],
        archived_at: null,
        skills: [{ id: "skill-1", name: "Alpha", enabled: true }],
        invocation_targets: [],
        created_at: "",
        updated_at: "",
      },
    ]);
    api.replaceSkillFiles.mockResolvedValue([]);
    api.updateSkill.mockResolvedValue({});
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("saves the full file set from the file tree editor", async () => {
    const { queryClient } = renderWithProviders(<SkillDetailPage id="skill-1" />);
    await screen.findByText("notes.md");
    fireEvent.click(screen.getByRole("button", { name: "notes.md" }));
    fireEvent.change(screen.getByDisplayValue("notes"), { target: { value: "updated notes" } });
    fireEvent.click(screen.getByRole("button", { name: "Save files" }));

    await waitFor(() => {
      expect(api.replaceSkillFiles).toHaveBeenCalledWith(
        "skill-1",
        expect.arrayContaining([
          { path: "SKILL.md", content: "# Alpha" },
          { path: "notes.md", content: "updated notes" },
        ]),
      );
    });
    queryClient.clear();
  });

  it("shows a conflict message when deleting a bound skill", async () => {
    const { ApiError } = await import("@dars/core/api");
    api.deleteSkill.mockRejectedValue(new ApiError("conflict", 409, "Conflict"));
    const { queryClient } = renderWithProviders(<SkillDetailPage id="skill-1" />);
    await screen.findByRole("button", { name: "Delete" });
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect((await screen.findByTestId("skill-delete-conflict")).textContent).toContain("Agent One");
    queryClient.clear();
  });
});
