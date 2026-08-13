import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { createWorkspaceAwareStorage, registerForWorkspaceRehydration } from "../../platform/workspace-storage";
import { defaultStorage } from "../../platform/storage";
import type { AgentColumnKey, AgentListFilters, AgentSortDirection, AgentSortField, AgentsScope } from "./types";

export const AGENT_SCOPES: AgentsScope[] = ["mine", "all", "archived"];

export const AGENT_SORT_DEFAULT_DIRECTION: Record<AgentSortField, AgentSortDirection> = {
  lastActive: "desc",
  name: "asc",
  runs: "desc",
  created: "desc",
};

export const EMPTY_AGENT_FILTERS: AgentListFilters = {
  availability: [],
  runtimes: [],
  owners: [],
  models: [],
  access: [],
};

export const AGENT_DEFAULT_HIDDEN_COLUMNS: AgentColumnKey[] = ["model", "created"];

export type AgentsViewState = {
  scope: AgentsScope;
  sortField: AgentSortField;
  sortDirection: AgentSortDirection;
  hiddenColumns: AgentColumnKey[];
  filters: AgentListFilters;
  pageSize: number;
  setScope: (scope: AgentsScope) => void;
  toggleSort: (field: AgentSortField) => void;
  setSortField: (field: AgentSortField) => void;
  setSortDirection: (direction: AgentSortDirection) => void;
  toggleColumn: (key: AgentColumnKey) => void;
  toggleFilter: (key: keyof AgentListFilters, value: string) => void;
  clearFilters: () => void;
  setPageSize: (pageSize: number) => void;
};

const DEFAULTS = {
  scope: "mine" as AgentsScope,
  sortField: "lastActive" as AgentSortField,
  sortDirection: AGENT_SORT_DEFAULT_DIRECTION.lastActive,
  hiddenColumns: AGENT_DEFAULT_HIDDEN_COLUMNS,
  filters: EMPTY_AGENT_FILTERS,
  pageSize: 25,
};

export const useAgentsViewStore = create<AgentsViewState>()(
  persist(
    (set) => ({
      ...DEFAULTS,
      setScope: (scope) =>
        set(scope === "mine" ? { scope, filters: EMPTY_AGENT_FILTERS } : { scope }),
      toggleSort: (field) =>
        set((state) =>
          state.sortField === field
            ? { sortDirection: state.sortDirection === "asc" ? "desc" : "asc" }
            : { sortField: field, sortDirection: AGENT_SORT_DEFAULT_DIRECTION[field] },
        ),
      setSortField: (field) =>
        set((state) =>
          state.sortField === field
            ? {}
            : { sortField: field, sortDirection: AGENT_SORT_DEFAULT_DIRECTION[field] },
        ),
      setSortDirection: (direction) => set({ sortDirection: direction }),
      toggleColumn: (key) =>
        set((state) => ({
          hiddenColumns: state.hiddenColumns.includes(key)
            ? state.hiddenColumns.filter((item) => item !== key)
            : [...state.hiddenColumns, key],
        })),
      toggleFilter: (key, value) =>
        set((state) => {
          const list = state.filters[key] as string[];
          const next = list.includes(value) ? list.filter((item) => item !== value) : [...list, value];
          const scope = state.scope === "mine" ? "all" : state.scope;
          return { scope, filters: { ...state.filters, [key]: next } };
        }),
      clearFilters: () => set({ filters: EMPTY_AGENT_FILTERS }),
      setPageSize: (pageSize) => set({ pageSize }),
    }),
    {
      name: "dars_lightweight_agents_view",
      storage: createJSONStorage(() => createWorkspaceAwareStorage(defaultStorage)),
      partialize: (state) => ({
        scope: state.scope,
        sortField: state.sortField,
        sortDirection: state.sortDirection,
        hiddenColumns: state.hiddenColumns,
        filters: state.filters,
        pageSize: state.pageSize,
      }),
      merge: (persisted, current) => {
        if (!persisted) return { ...current, ...DEFAULTS };
        const value = persisted as Partial<AgentsViewState>;
        return {
          ...current,
          ...value,
          filters: { ...EMPTY_AGENT_FILTERS, ...(value.filters ?? {}) },
        };
      },
    },
  ),
);

registerForWorkspaceRehydration(() => useAgentsViewStore.persist.rehydrate());
