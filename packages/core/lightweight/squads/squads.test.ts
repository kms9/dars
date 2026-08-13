import { describe, expect, it } from "vitest";
import {
  buildSquadListRows,
  computeSquadScopeCounts,
  matchesSquadSearch,
  rowMatchesSquadFilters,
  sortSquadRows,
  squadScopeMatches,
} from "./list";
import type { SquadListRow } from "./types";

const squadFixture = {
  id: "s1",
  workspace_id: "w1",
  name: "Alpha Squad",
  description: "coordinates work",
  instructions: "",
  avatar_url: null,
  leader_id: "leader-1",
  creator_id: "user-1",
  archived_at: null,
  archived_by: null,
  member_count: 2,
  member_preview: [
    { agent_id: "leader-1", name: "Leader", role: "leader" as const },
    { agent_id: "member-1", name: "Member", role: "member" as const },
  ],
  created_at: "2026-08-01T00:00:00Z",
  updated_at: "2026-08-02T00:00:00Z",
};

describe("squad list helpers", () => {
  it("matches search on name and description", () => {
    expect(matchesSquadSearch(squadFixture, "alpha")).toBe(true);
    expect(matchesSquadSearch(squadFixture, "coordinates")).toBe(true);
    expect(matchesSquadSearch(squadFixture, "missing")).toBe(false);
  });

  it("scopes mine vs all and excludes archived", () => {
    expect(squadScopeMatches(squadFixture, "mine", "user-1", "member")).toBe(true);
    expect(squadScopeMatches(squadFixture, "mine", "other", "member")).toBe(false);
    expect(squadScopeMatches(squadFixture, "mine", "other", "admin")).toBe(true);
    expect(squadScopeMatches({ ...squadFixture, archived_at: "2026-08-03T00:00:00Z" }, "all", "user-1", "admin")).toBe(
      false,
    );
  });

  it("filters by leader, creator, and member preview", () => {
    const row: SquadListRow = {
      squad: squadFixture,
      leaderName: "Leader",
      creatorName: "Creator",
      canManage: true,
      isMine: true,
    };
    expect(
      rowMatchesSquadFilters(row, { leaders: ["leader-1"], creators: [], members: [] }, ""),
    ).toBe(true);
    expect(
      rowMatchesSquadFilters(row, { leaders: ["other"], creators: [], members: [] }, ""),
    ).toBe(false);
    expect(
      rowMatchesSquadFilters(row, { leaders: [], creators: [], members: ["member-1"] }, ""),
    ).toBe(true);
  });

  it("builds rows with manage flags and counts scopes", () => {
    const rows = buildSquadListRows({
      squads: [squadFixture],
      members: [{ id: "m1", workspace_id: "w1", user_id: "user-1", role: "member", name: "Creator", email: "", created_at: "" }],
      currentUserId: "user-1",
      memberRole: "member",
    });
    expect(rows[0]?.canManage).toBe(true);
    expect(rows[0]?.creatorName).toBe("Creator");
    const counts = computeSquadScopeCounts(rows, "user-1", "member");
    expect(counts).toEqual({ mine: 1, all: 1 });
  });

  it("sorts by member count and name", () => {
    const small: SquadListRow = {
      squad: { ...squadFixture, id: "s2", name: "Beta", member_count: 1 },
      leaderName: "L",
      creatorName: "C",
      canManage: false,
      isMine: false,
    };
    const large: SquadListRow = {
      squad: squadFixture,
      leaderName: "L",
      creatorName: "C",
      canManage: true,
      isMine: true,
    };
    const sorted = sortSquadRows([small, large], "members", "desc");
    expect(sorted[0]?.squad.id).toBe("s1");
  });
});
