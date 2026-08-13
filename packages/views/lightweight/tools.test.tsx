/** @vitest-environment jsdom */

import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ToolsPage } from "./tools";

const state = vi.hoisted(() => ({ role: "owner" as "owner" | "admin" | "member" }));
const api = vi.hoisted(() => ({
  listMembers: vi.fn(),
  listToolSources: vi.fn(),
  importToolSource: vi.fn(),
  createToolSource: vi.fn(),
  validateToolSource: vi.fn(),
}));
const navigation = vi.hoisted(() => ({
  push: vi.fn(), replace: vi.fn(), back: vi.fn(), pathname: "/test/tools", searchParams: new URLSearchParams(),
}));

vi.mock("@dars/core/lightweight", () => ({
  lightweightApi: api,
  lightweightKeys: { members: (workspaceId: string) => ["lightweight", workspaceId, "members"] },
  toolKeys: {
    sources: (workspaceId: string) => ["lightweight", workspaceId, "tools", "sources"],
    source: (workspaceId: string, sourceId: string) => ["lightweight", workspaceId, "tools", "sources", sourceId],
    definitions: (workspaceId: string, sourceId: string) => ["lightweight", workspaceId, "tools", "sources", sourceId, "definitions"],
  },
  toolSourcesOptions: (workspaceId: string) => ({
    queryKey: ["lightweight", workspaceId, "tools", "sources"], queryFn: api.listToolSources,
  }),
  toolSourceOptions: vi.fn(),
  toolDefinitionsOptions: vi.fn(),
}));

vi.mock("@dars/core/auth", () => ({
  useAuthStore: (selector: (value: { user: { id: string } }) => unknown) => selector({ user: { id: "user-1" } }),
}));

vi.mock("@dars/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "workspace-1", slug: "test" }),
  useWorkspacePaths: () => ({
    tools: () => "/test/tools",
    toolDetail: (id: string) => `/test/tools/${id}`,
  }),
}));

vi.mock("../navigation", () => ({
  useNavigation: () => navigation,
  AppLink: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => <a href={href} {...props}>{children}</a>,
}));

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><ToolsPage /></QueryClientProvider>);
}

describe("Workspace Tools control plane", () => {
  beforeEach(() => {
    cleanup();
    state.role = "owner";
    navigation.push.mockReset();
    api.listMembers.mockReset().mockImplementation(async () => [{
      id: "member-1", workspace_id: "workspace-1", user_id: "user-1", role: state.role,
      name: "Owner", email: "owner@example.com", created_at: "",
    }]);
    api.listToolSources.mockReset().mockResolvedValue([]);
    api.importToolSource.mockReset().mockResolvedValue({
      source: {
        id: "source-1", workspaceId: "workspace-1", name: "pet-api", kind: "openapi", enabled: false,
        currentRevision: "revision-1", revision: null, createdAt: "", updatedAt: "",
      },
      tools: [],
      artifact: { id: "artifact-1", sha256: "hash", mediaType: "application/json", sizeBytes: 10 },
    });
  });

  it("uploads an OpenAPI file through the Server import API", async () => {
    renderPage();
    await screen.findByText("Import Tool Source");
    fireEvent.change(screen.getByLabelText("Canonical namespace"), { target: { value: "pet-api" } });
    fireEvent.change(screen.getByLabelText("API base endpoint"), { target: { value: "https://api.example.com" } });
    const file = new File(["{}"], "openapi.json", { type: "application/json" });
    fireEvent.change(screen.getByLabelText("Artifact"), { target: { files: [file] } });
    const submit = screen.getByRole("button", { name: "Import and validate" });
    fireEvent.submit(submit.closest("form")!);
    await waitFor(() => expect(api.importToolSource).toHaveBeenCalledWith(expect.objectContaining({
      name: "pet-api", kind: "openapi", endpoint: "https://api.example.com", filename: "openapi.json",
    })));
    expect(navigation.push).toHaveBeenCalledWith("/test/tools/source-1");
  });

  it("hides data and actions from ordinary members", async () => {
    state.role = "member";
    renderPage();
    expect((await screen.findByRole("alert")).textContent).toContain("Workspace owner or admin permission is required");
    expect(screen.queryByText("Import Tool Source")).toBeNull();
    expect(api.listToolSources).not.toHaveBeenCalled();
  });
});
