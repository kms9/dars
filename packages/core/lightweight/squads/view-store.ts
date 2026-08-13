import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { createWorkspaceAwareStorage, registerForWorkspaceRehydration } from "../../platform/workspace-storage";
import { defaultStorage } from "../../platform/storage";
import type {
  SquadColumnKey,
  SquadListFilters,
  SquadSortDirection,
  SquadSortField,
  SquadsScope,
} from "./types";

export const SQUAD_SCOPES: SquadsScope[] = ["mine", "all"];

export const SQUAD_SORT_DEFAULT_DIRECTION: Record<SquadSortField, SquadSortDirection> = {
  updated: "desc",
  name: "asc",
  members: "desc",
  created: "desc",
};

export const EMPTY_SQUAD_FILTERS: SquadListFilters = {
  leaders: [],
  creators: [],
  members: [],
};

export const SQUAD_DEFAULT_HIDDEN_COLUMNS: SquadColumnKey[] = ["updated"];

export type SquadsViewState = {
  scope: SquadsScope;
  sortField: SquadSortField;
  sortDirection: SquadSortDirection;
  hiddenColumns: SquadColumnKey[];
  filters: SquadListFilters;
  pageSize: number;
  setScope: (scope: SquadsScope) => void;
  toggleSort: (field: SquadSortField) => void;
  setSortField: (field: SquadSortField) => void;
  setSortDirection: (direction: SquadSortDirection) => void;
  toggleColumn: (key: SquadColumnKey) => void;
  toggleFilter: (key: keyof SquadListFilters, value: string) => void;
  clearFilters: () => void;
  setPageSize: (pageSize: number) => void;
};

const DEFAULTS = {
  scope: "mine" as SquadsScope,
  sortField: "updated" as SquadSortField,
  sortDirection: SQUAD_SORT_DEFAULT_DIRECTION.updated,
  hiddenColumns: SQUAD_DEFAULT_HIDDEN_COLUMNS,
  filters: EMPTY_SQUAD_FILTERS,
  pageSize: 25,
};

export const useSquadsViewStore = create<SquadsViewState>()(
  persist(
    (set) => ({
      ...DEFAULTS,
      setScope: (scope) =>
        set(scope === "mine" ? { scope, filters: EMPTY_SQUAD_FILTERS } : { scope }),
      toggleSort: (field) =>
        set((state) =>
          state.sortField === field
            ? { sortDirection: state.sortDirection === "asc" ? "desc" : "asc" }
            : { sortField: field, sortDirection: SQUAD_SORT_DEFAULT_DIRECTION[field] },
        ),
      setSortField: (field) => set({ sortField: field }),
      setSortDirection: (direction) => set({ sortDirection: direction }),
      toggleColumn: (key) =>
        set((state) => ({
          hiddenColumns: state.hiddenColumns.includes(key)
            ? state.hiddenColumns.filter((item) => item !== key)
            : [...state.hiddenColumns, key],
        })),
      toggleFilter: (key, value) =>
        set((state) => {
          const current = state.filters[key];
          const next = current.includes(value)
            ? current.filter((item) => item !== value)
            : [...current, value];
          return { filters: { ...state.filters, [key]: next } };
        }),
      clearFilters: () => set({ filters: EMPTY_SQUAD_FILTERS }),
      setPageSize: (pageSize) => set({ pageSize }),
    }),
    {
      name: "lightweight-squads-view",
      storage: createJSONStorage(() => createWorkspaceAwareStorage(defaultStorage)),
      partialize: (state) => ({
        scope: state.scope,
        sortField: state.sortField,
        sortDirection: state.sortDirection,
        hiddenColumns: state.hiddenColumns,
        filters: state.filters,
        pageSize: state.pageSize,
      }),
    },
  ),
);

registerForWorkspaceRehydration(() => useSquadsViewStore.persist.rehydrate());
