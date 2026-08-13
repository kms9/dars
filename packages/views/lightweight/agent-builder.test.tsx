/** @vitest-environment jsdom */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AiBuilderSessionPage } from "./agent-builder";

const api = vi.hoisted(() => ({
  listAgentBuilderSessions: vi.fn(),
  listChatMessages: vi.fn(),
  getPendingChatTask: vi.fn(),
  listSkills: vi.fn(),
  listRuntimes: vi.fn(),
  saveAgentBuilderDraft: vi.fn(),
  sendChatMessage: vi.fn(),
  switchAgentBuilderRuntime: vi.fn(),
}));

vi.mock("@dars/core/lightweight", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@dars/core/lightweight")>();
  return {
    ...actual,
    lightweightApi: api,
    lightweightKeys: {
      runtimes: (workspaceId: string) => ["lightweight", workspaceId, "runtimes"],
      skills: (workspaceId: string) => ["lightweight", workspaceId, "skills"],
    },
    lightweightAgentBuilderKeys: {
      all: (workspaceId: string) => ["lightweight", workspaceId, "agent-builder"],
      sessions: (workspaceId: string) => ["lightweight", workspaceId, "agent-builder", "sessions"],
    },
    lightweightAgentBuilderSessionsOptions: (workspaceId: string) => ({
      queryKey: ["lightweight", workspaceId, "agent-builder", "sessions"],
      queryFn: api.listAgentBuilderSessions,
      enabled: !!workspaceId,
    }),
  };
});

vi.mock("@dars/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "workspace-1", slug: "test" }),
  useWorkspacePaths: () => ({
    newAgentAi: () => "/test/agents/new/ai",
    agents: () => "/test/agents",
    agentDetail: (id: string) => `/test/agents/${id}`,
  }),
}));

const navigation = vi.hoisted(() => ({
  push: vi.fn(),
  pathname: "/test/agents/new/ai/session-1",
  searchParams: new URLSearchParams(),
}));

vi.mock("../navigation", () => ({
  useNavigation: () => navigation,
}));

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <AiBuilderSessionPage sessionId="session-1" />
    </QueryClientProvider>,
  );
}

describe("AiBuilderSessionPage", () => {
  beforeEach(() => {
    cleanup();
    if (!HTMLFormElement.prototype.requestSubmit) {
      HTMLFormElement.prototype.requestSubmit = function requestSubmit(this: HTMLFormElement) {
        this.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
      };
    }
    api.listAgentBuilderSessions.mockResolvedValue([{
      session_id: "session-1",
      title: "Create an agent",
      runtime_id: "runtime-1",
      created_at: "2026-08-06T00:00:00Z",
      updated_at: "2026-08-06T00:00:00Z",
      last_message_content: "",
      last_message_role: "",
      last_message_at: "",
      draft: {
        name: "Draft agent",
        description: "",
        instructions: "Do things",
        avatar_url: null,
        model: "",
        thinking_level: "",
        service_tier: "",
        skill_ids: [],
        permission_scope: "private",
        member_ids: [],
        max_concurrent_tasks: 1,
      },
    }]);
    api.listChatMessages.mockResolvedValue({ items: [], next_cursor: null });
    api.getPendingChatTask.mockResolvedValue({});
    api.listSkills.mockResolvedValue([]);
    api.listRuntimes.mockResolvedValue([{ id: "runtime-1", name: "Local", status: "online" }]);
    api.saveAgentBuilderDraft.mockResolvedValue(undefined);
    api.sendChatMessage.mockResolvedValue({ message_id: "m1", task_id: "t1", created_at: "2026-08-06T00:00:00Z" });
    navigation.push.mockReset();
    vi.spyOn(window, "confirm").mockReturnValue(true);
  });

  it("submits composer on Enter without Shift", async () => {
    renderPage();
    const textarea = await screen.findByPlaceholderText("Describe the agent you want to build");
    fireEvent.change(textarea, { target: { value: "Build a reviewer" } });
    fireEvent.keyDown(textarea, { key: "Enter", code: "Enter", shiftKey: false });
    await vi.waitFor(() => expect(api.sendChatMessage).toHaveBeenCalled());
  });

  it("asks before leaving when draft is dirty", async () => {
    renderPage();
    const nameInput = await screen.findByDisplayValue("Draft agent");
    fireEvent.change(nameInput, { target: { value: "Changed" } });
    await waitFor(() => expect(screen.getByText("Unsaved edits")).toBeTruthy());
    const back = screen.getAllByRole("button", { name: "Back" })[0];
    expect(back).toBeTruthy();
    fireEvent.click(back!);
    expect(window.confirm).toHaveBeenCalled();
    expect(navigation.push).toHaveBeenCalledWith("/test/agents/new/ai");
  });
});
