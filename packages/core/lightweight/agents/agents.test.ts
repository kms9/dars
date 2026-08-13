import { describe, expect, it } from "vitest";
import {
  buildDuplicateDraft,
  EMPTY_AGENT_DRAFT,
  manualDraftOwner,
  setManualDraftEntry,
  storedAgentDraftsEqual,
  toStoredAgentDraft,
} from "./draft";
import { effectiveAccessScope } from "./effective-access";
import { rowMatchesFilters, scopeMatches, sortAgentRows } from "./list";
import type { AgentListRow } from "./types";
import type { LightweightAgent, LightweightAgentSnapshot } from "../types";

const runtime = {
  id: "runtime-1",
  workspace_id: "ws-1",
  daemon_id: "daemon-1",
  name: "Local",
  provider: "codex",
  status: "online",
  device_info: "",
  metadata: {},
  owner_id: "user-1",
  profile_id: null,
  last_seen_at: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const sourceAgent: LightweightAgent = {
  id: "agent-1",
  workspace_id: "ws-1",
  runtime_id: "runtime-1",
  owner_id: "user-1",
  name: "Reviewer",
  description: "Reviews code",
  instructions: "Be strict",
  runtime_config: { mode: "fast" },
  status: "idle",
  max_concurrent_tasks: 2,
  custom_env_keys: ["API_KEY"],
  custom_args: ["--flag"],
  mcp_configured: false,
  model: "gpt-test",
  thinking_level: "high",
  service_tier: "priority",
  permission_mode: "public_to",
  disabled_runtime_skills: [],
  archived_at: null,
  skills: [{ id: "skill-1", name: "Lint", enabled: true }],
  invocation_targets: [{ id: "target-1", target_type: "workspace", target_id: "ws-1" }],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-02T00:00:00Z",
};

describe("agent draft helpers", () => {
  it("scopes manual drafts by owner", () => {
    expect(manualDraftOwner(null)).toBe("blank");
    expect(manualDraftOwner("agent-1")).toBe("duplicate:agent-1");
  });

  it("drops empty manual draft slots", () => {
    const next = setManualDraftEntry({ byOwner: {} }, "blank", {
      runtimeId: "runtime-1",
      draft: toStoredAgentDraft(EMPTY_AGENT_DRAFT),
    });
    expect(next.byOwner.blank).toBeUndefined();
  });

  it("duplicate clears identity and unavailable runtime config", () => {
    const draft = buildDuplicateDraft(sourceAgent, {
      runtimes: [{ ...runtime, status: "offline" }],
      currentUserId: "user-1",
      fallbackRuntimeId: "runtime-2",
      nameSuffix: " copy",
    });
    expect(draft.name).toBe("Reviewer copy");
    expect(draft.instructions).toBe("Be strict");
    expect(draft.skillIds.has("skill-1")).toBe(true);
    expect(draft.avatarUrl).toBeNull();
    expect(draft.runtimeId).toBe("runtime-2");
    expect(draft.model).toBe("");
  });

  it("duplicate keeps model when runtime stays usable", () => {
    const draft = buildDuplicateDraft(sourceAgent, {
      runtimes: [runtime],
      currentUserId: "user-1",
      fallbackRuntimeId: "runtime-2",
      nameSuffix: " copy",
    });
    expect(draft.runtimeId).toBe("runtime-1");
    expect(draft.model).toBe("gpt-test");
  });
});

describe("agent list helpers", () => {
  const snapshot: LightweightAgentSnapshot = {
    scope_counts: { mine: 1, all: 1, archived: 0 },
    agents: [
      {
        id: "agent-1",
        workspace_id: "ws-1",
        owner_id: "user-1",
        name: "Alpha",
        description: "First",
        avatar_url: null,
        runtime_id: "runtime-1",
        status: "idle",
        permission_mode: "private",
        model: "gpt-test",
        thinking_level: null,
        service_tier: null,
        max_concurrent_tasks: 1,
        archived_at: null,
        updated_at: "2026-01-02T00:00:00Z",
      },
    ],
    tasks: [],
    run_counts: [{ agent_id: "agent-1", run_count: 3 }],
    activity: [],
    filter_metadata: { owners: [{ id: "user-1", name: "Owner" }], runtimes: [] },
  };

  const row: AgentListRow = {
    agent: snapshot.agents[0]!,
    detail: null,
    runtime,
    presence: {
      availability: "online",
      workload: "idle",
      runningCount: 0,
      queuedCount: 0,
      capacity: 1,
    },
    runCount: 3,
    lastActiveAt: null,
    ownerName: "Owner",
    isOwnedByMe: true,
    canManage: true,
  };

  it("filters by scope and search", () => {
    expect(scopeMatches(row.agent, "mine", "user-1")).toBe(true);
    expect(scopeMatches(row.agent, "mine", "user-2")).toBe(false);
    expect(rowMatchesFilters(row, { availability: [], runtimes: [], owners: [], models: [], access: [] }, "alpha")).toBe(true);
    expect(rowMatchesFilters(row, { availability: [], runtimes: [], owners: [], models: [], access: [] }, "missing")).toBe(false);
  });

  it("derives access scope for workspace grants", () => {
    expect(effectiveAccessScope("public_to", [{ id: "t1", target_type: "workspace", target_id: "ws-1" }])).toBe("workspace");
    expect(effectiveAccessScope("private", [])).toBe("owner-only");
  });

  it("sorts by run count descending", () => {
    const other = {
      ...row,
      agent: { ...row.agent, id: "agent-2", name: "Beta" },
      runCount: 1,
    };
    const sorted = sortAgentRows([other, row], "runs", "desc");
    expect(sorted[0]?.agent.id).toBe("agent-1");
  });

  it("compares stored drafts for manual persistence", () => {
    const a = toStoredAgentDraft({ ...EMPTY_AGENT_DRAFT, name: "A" });
    const b = toStoredAgentDraft({ ...EMPTY_AGENT_DRAFT, name: "B" });
    expect(storedAgentDraftsEqual(a, b)).toBe(false);
  });
});
