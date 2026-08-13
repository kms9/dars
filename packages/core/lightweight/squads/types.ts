import type { LightweightSquad } from "../types";

export type SquadsScope = "mine" | "all";

export type SquadSortField = "name" | "members" | "updated" | "created";
export type SquadSortDirection = "asc" | "desc";

export type SquadListFilters = {
  leaders: string[];
  creators: string[];
  members: string[];
};

export type SquadListRow = {
  squad: LightweightSquad;
  leaderName: string;
  creatorName: string;
  canManage: boolean;
  isMine: boolean;
};

export type SquadColumnKey = "squad" | "leader" | "members" | "creator" | "updated";
