import { describe, expect, it } from "vitest";
import {
  agentDetailHref,
  buildAgentDetailSearchParams,
  parseAgentDetailViewState,
} from "./detail-view";
import { canInvokeAgent } from "./permissions";
import { isRuntimeSkillDisabled, setRuntimeSkillEnabled } from "./runtime-skills";

describe("agent detail view state", () => {
  it("defaults to overview", () => {
    expect(parseAgentDetailViewState(new URLSearchParams())).toEqual({
      view: "overview",
      cap: "instructions",
      workScope: "assigned",
      workStatus: "",
      workQuery: "",
      workCursor: null,
    });
  });

  it("maps retired MCP and integrations views to overview", () => {
    expect(parseAgentDetailViewState(new URLSearchParams("view=mcp")).view).toBe("overview");
    expect(parseAgentDetailViewState(new URLSearchParams("view=integrations")).view).toBe("overview");
  });

  it("parses work tab deep links", () => {
    expect(parseAgentDetailViewState(new URLSearchParams("view=work&workScope=created"))).toMatchObject({
      view: "work",
      workScope: "created",
    });
  });

  it("maps legacy instructions/skills deep links into capabilities", () => {
    expect(parseAgentDetailViewState(new URLSearchParams("view=skills"))).toMatchObject({
      view: "capabilities",
      cap: "skills",
    });
  });

  it("supports the approved Tools capability without restoring the retired MCP view", () => {
    expect(parseAgentDetailViewState(new URLSearchParams("view=capabilities&cap=tools"))).toMatchObject({
      view: "capabilities",
      cap: "tools",
    });
    expect(parseAgentDetailViewState(new URLSearchParams("view=mcp")).view).toBe("overview");
  });

  it("round-trips work filters in the URL", () => {
    const href = agentDetailHref("/ws/agents/agent-1", {
      view: "work",
      workScope: "created",
      workStatus: "todo",
      workQuery: "bug",
      workCursor: "cursor-1",
    });
    const params = new URL(href, "http://localhost").searchParams;
    expect(parseAgentDetailViewState(params)).toMatchObject({
      view: "work",
      workScope: "created",
      workStatus: "todo",
      workQuery: "bug",
      workCursor: "cursor-1",
    });
    expect(buildAgentDetailSearchParams(parseAgentDetailViewState(params)).toString()).toContain("workScope=created");
  });
});

describe("agent permissions", () => {
  const agent = {
    owner_id: "owner-1",
    permission_mode: "public_to",
    archived_at: null,
  };

  it("allows workspace access targets to invoke", () => {
    expect(
      canInvokeAgent(
        agent,
        [{ id: "t1", target_type: "workspace", target_id: "ws-1" }],
        "member-user",
        "member-1",
      ),
    ).toBe(true);
  });

  it("blocks archived agents", () => {
    expect(
      canInvokeAgent(
        { ...agent, archived_at: "2026-01-01T00:00:00Z" },
        [{ id: "t1", target_type: "workspace", target_id: "ws-1" }],
        "member-user",
        "member-1",
      ),
    ).toBe(false);
  });
});

describe("runtime skill toggles", () => {
  const skill = {
    key: "lint",
    name: "Lint",
    provider: "codex",
    source_path: "/tmp",
    file_count: 1,
    root: "provider" as const,
  };

  it("tracks disabled runtime skills by runtime and key", () => {
    const disabled = setRuntimeSkillEnabled([], "runtime-1", skill, false);
    expect(isRuntimeSkillDisabled(disabled, "runtime-1", skill)).toBe(true);
    expect(setRuntimeSkillEnabled(disabled, "runtime-1", skill, true)).toEqual([]);
  });
});
