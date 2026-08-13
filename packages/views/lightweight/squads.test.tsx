import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { buildSquadListRows, computeSquadScopeCounts } from "@dars/core/lightweight";

vi.mock("@dars/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "u1" } }),
}));
vi.mock("@dars/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "w1", slug: "demo" }),
  useWorkspacePaths: () => ({
    squadDetail: (id: string) => `/demo/squads/${id}`,
    squads: () => "/demo/squads",
    newAgentBlank: () => "/demo/agents/new/blank",
    issueDetail: (id: string) => `/demo/issues/${id}`,
  }),
}));
vi.mock("@dars/core/lightweight", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dars/core/lightweight")>();
  return {
    ...actual,
    lightweightApi: {
      ...actual.lightweightApi,
      listSquads: async () => [
        {
          id: "s1",
          workspace_id: "w1",
          name: "Alpha",
          description: "",
          instructions: "",
          avatar_url: null,
          leader_id: "a1",
          creator_id: "u1",
          archived_at: null,
          archived_by: null,
          member_count: 1,
          member_preview: [{ agent_id: "a1", name: "Leader", role: "leader" }],
          created_at: "2026-08-01T00:00:00Z",
          updated_at: "2026-08-02T00:00:00Z",
        },
      ],
      listAgents: async () => [],
      listMembers: async () => [
        { id: "m1", workspace_id: "w1", user_id: "u1", role: "member", name: "Me", email: "", created_at: "" },
      ],
      archiveSquad: async () => undefined,
    },
    lightweightKeys: actual.lightweightKeys,
  };
});
vi.mock("../navigation", () => ({
  AppLink: ({ children, href }: { children: React.ReactNode; href: string }) => <a href={href}>{children}</a>,
  useNavigation: () => ({ push: vi.fn(), searchParams: new URLSearchParams() }),
}));

describe("squad list data", () => {
  it("computes mine and all counts for active squads", () => {
    const rows = buildSquadListRows({
      squads: [
        {
          id: "s1",
          workspace_id: "w1",
          name: "One",
          description: "",
          instructions: "",
          avatar_url: null,
          leader_id: "a1",
          creator_id: "u1",
          archived_at: null,
          archived_by: null,
          member_count: 1,
          member_preview: [{ agent_id: "a1", name: "Leader", role: "leader" }],
          created_at: "2026-08-01T00:00:00Z",
          updated_at: "2026-08-02T00:00:00Z",
        },
        {
          id: "s2",
          workspace_id: "w1",
          name: "Two",
          description: "",
          instructions: "",
          avatar_url: null,
          leader_id: "a2",
          creator_id: "u2",
          archived_at: "2026-08-03T00:00:00Z",
          archived_by: "u2",
          member_count: 1,
          member_preview: [{ agent_id: "a2", name: "Other", role: "leader" }],
          created_at: "2026-08-01T00:00:00Z",
          updated_at: "2026-08-02T00:00:00Z",
        },
      ],
      members: [],
      currentUserId: "u1",
      memberRole: "member",
    });
    expect(computeSquadScopeCounts(rows, "u1", "member")).toEqual({ mine: 1, all: 1 });
  });
});

describe("SquadsPage", () => {
  it("renders scope controls when data is provided", async () => {
    const { SquadsPage } = await import("./squads");
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={client}>
        <SquadsPage />
      </QueryClientProvider>,
    );
    expect(await screen.findByText("My (1)")).toBeTruthy();
    expect(screen.getByText("Alpha")).toBeTruthy();
  });
});
