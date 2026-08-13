import type { LightweightMember } from "../types";
import type { SquadListFilters, SquadListRow, SquadsScope } from "./types";

export function matchesSquadSearch(
  squad: { name: string; description: string },
  query: string,
): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  return [squad.name, squad.description].some((value) => value.toLowerCase().includes(needle));
}

export function squadScopeMatches(
  squad: { creator_id: string; archived_at: string | null },
  scope: SquadsScope,
  currentUserId: string | null,
  memberRole: "owner" | "admin" | "member" | null,
): boolean {
  if (squad.archived_at) return false;
  if (scope === "mine") {
    return (
      !!currentUserId &&
      (squad.creator_id === currentUserId || memberRole === "owner" || memberRole === "admin")
    );
  }
  return true;
}

export function rowMatchesSquadFilters(
  row: SquadListRow,
  filters: SquadListFilters,
  query: string,
): boolean {
  if (!matchesSquadSearch(row.squad, query)) return false;
  if (filters.leaders.length > 0 && !filters.leaders.includes(row.squad.leader_id)) return false;
  if (filters.creators.length > 0 && !filters.creators.includes(row.squad.creator_id)) return false;
  if (
    filters.members.length > 0 &&
    !row.squad.member_preview.some((member) => filters.members.includes(member.agent_id))
  ) {
    return false;
  }
  return true;
}

export function buildSquadListRows(input: {
  squads: readonly import("../types").LightweightSquad[];
  members: readonly LightweightMember[];
  currentUserId: string | null;
  memberRole: "owner" | "admin" | "member" | null;
}): SquadListRow[] {
  const memberNames = new Map(input.members.map((member) => [member.user_id, member.name]));
  return input.squads.map((squad) => {
    const leaderPreview = squad.member_preview.find((member) => member.role === "leader");
    const canManage =
      input.memberRole === "owner" ||
      input.memberRole === "admin" ||
      (!!input.currentUserId && squad.creator_id === input.currentUserId);
    return {
      squad,
      leaderName: leaderPreview?.name ?? squad.leader_id,
      creatorName: memberNames.get(squad.creator_id) ?? squad.creator_id,
      canManage,
      isMine: !!input.currentUserId && squad.creator_id === input.currentUserId,
    };
  });
}

export function computeSquadScopeCounts(
  rows: readonly SquadListRow[],
  currentUserId: string | null,
  memberRole: "owner" | "admin" | "member" | null,
): { mine: number; all: number } {
  const active = rows.filter((row) => !row.squad.archived_at);
  const mine = active.filter(
    (row) =>
      !!currentUserId &&
      (row.squad.creator_id === currentUserId || memberRole === "owner" || memberRole === "admin"),
  ).length;
  return { mine, all: active.length };
}

function compareStrings(a: string | null | undefined, b: string | null | undefined): number {
  return (a ?? "").localeCompare(b ?? "");
}

export function sortSquadRows(
  rows: SquadListRow[],
  field: import("./types").SquadSortField,
  direction: import("./types").SquadSortDirection,
): SquadListRow[] {
  const factor = direction === "asc" ? 1 : -1;
  return [...rows].sort((left, right) => {
    let result = 0;
    switch (field) {
      case "name":
        result = compareStrings(left.squad.name, right.squad.name);
        break;
      case "members":
        result = left.squad.member_count - right.squad.member_count;
        break;
      case "created":
        result = compareStrings(left.squad.created_at, right.squad.created_at);
        break;
      case "updated":
      default:
        result = compareStrings(left.squad.updated_at, right.squad.updated_at);
        break;
    }
    if (result === 0) result = compareStrings(left.squad.id, right.squad.id);
    return result * factor;
  });
}
