import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { createWorkspaceAwareStorage, registerForWorkspaceRehydration } from "../../platform/workspace-storage";
import { defaultStorage } from "../../platform/storage";
import type { ManualAgentDrafts } from "./types";

const EMPTY_MANUAL_AGENT_DRAFTS: ManualAgentDrafts = { byOwner: {} };

type ManualAgentDraftStore = {
  draft: ManualAgentDrafts;
  setDraft: (draft: ManualAgentDrafts) => void;
  clearDraft: () => void;
};

export const useManualAgentDraftStore = create<ManualAgentDraftStore>()(
  persist(
    (set) => ({
      draft: EMPTY_MANUAL_AGENT_DRAFTS,
      setDraft: (draft) => set({ draft }),
      clearDraft: () => set({ draft: EMPTY_MANUAL_AGENT_DRAFTS }),
    }),
    {
      name: "dars_lightweight_agents_manual_draft",
      storage: createJSONStorage(() => createWorkspaceAwareStorage(defaultStorage)),
      merge: (persisted, current) => {
        if (!persisted) return current;
        const value = persisted as Partial<ManualAgentDraftStore>;
        return {
          ...current,
          ...value,
          draft: value.draft ?? EMPTY_MANUAL_AGENT_DRAFTS,
        };
      },
    },
  ),
);

registerForWorkspaceRehydration(() => useManualAgentDraftStore.persist.rehydrate());
