import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { NavigationProvider } from "../navigation";
import { ChatPage } from "./chat";

const api = vi.hoisted(() => ({
  cancelTask: vi.fn(),
  consumeDraftRestore: vi.fn(),
  createChatSession: vi.fn(),
  getPendingChatTask: vi.fn(),
  listAgents: vi.fn(),
  listChatMessages: vi.fn(),
  listChatSessions: vi.fn(),
  listDraftRestores: vi.fn(),
  listTaskMessages: vi.fn(),
  sendChatMessage: vi.fn(),
  updateChatSession: vi.fn(),
}));

vi.mock("@dars/core/lightweight", () => ({
  lightweightApi: api,
  lightweightKeys: {
    agents: (workspaceId: string) => ["lightweight", workspaceId, "agents"],
    chats: (workspaceId: string) => ["lightweight", workspaceId, "chats"],
    taskMessages: (workspaceId: string, taskId: string) => ["lightweight", workspaceId, "task-messages", taskId],
  },
}));

vi.mock("@dars/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "workspace-1", slug: "test" }),
}));

describe("ChatPage streaming transcript", () => {
  beforeEach(() => {
    api.listChatSessions.mockResolvedValue({
      items: [{
        id: "session-1",
        workspace_id: "workspace-1",
        agent_id: "agent-1",
        creator_id: "user-1",
        runtime_id: "runtime-1",
        title: "Streaming test",
        session_id: null,
        work_dir: null,
        status: "active",
        created_at: "2026-08-06T00:00:00Z",
        updated_at: "2026-08-06T00:00:00Z",
      }],
      next_cursor: null,
    });
    api.listAgents.mockResolvedValue([]);
    api.listChatMessages.mockResolvedValue({ items: [], next_cursor: null });
    api.getPendingChatTask.mockResolvedValue({ task_id: "task-1", status: "running" });
    api.listDraftRestores.mockResolvedValue([]);
    api.listTaskMessages.mockResolvedValue({
      items: [
        { id: "m4", task_id: "task-1", seq: 4, type: "text", tool: null, content: "SECOND", input: null, output: null, created_at: "2026-08-06T00:00:04Z" },
        { id: "m2", task_id: "task-1", seq: 2, type: "thinking", tool: null, content: "PRIVATE_REASONING", input: null, output: null, created_at: "2026-08-06T00:00:02Z" },
        { id: "m3", task_id: "task-1", seq: 3, type: "tool_result", tool: "shell", content: null, input: null, output: "PRIVATE_TOOL_OUTPUT", created_at: "2026-08-06T00:00:03Z" },
        { id: "m1", task_id: "task-1", seq: 1, type: "text", tool: null, content: "FIRST", input: null, output: null, created_at: "2026-08-06T00:00:01Z" },
      ],
      next_cursor: null,
    });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("renders text events in sequence while hiding thinking and tool payloads", async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });

    render(
      <QueryClientProvider client={queryClient}>
        <NavigationProvider value={{
          push: vi.fn(),
          replace: vi.fn(),
          back: vi.fn(),
          pathname: "/test/chat",
          searchParams: new URLSearchParams("session=session-1"),
          getShareableUrl: (path) => path,
        }}>
          <ChatPage />
        </NavigationProvider>
      </QueryClientProvider>,
    );

    const stream = await screen.findByLabelText("Assistant response streaming");
    expect(stream.textContent).toContain("FIRST\n\nSECOND");
    expect(document.body.textContent).not.toContain("PRIVATE_REASONING");
    expect(document.body.textContent).not.toContain("PRIVATE_TOOL_OUTPUT");
    expect(api.listTaskMessages).toHaveBeenCalledWith("task-1");
    queryClient.clear();
  });
});
