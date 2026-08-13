"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@dars/core/api";
import {
  agentDetailHref,
  lightweightAgentDetailKeys,
  lightweightAgentSnapshotKeys,
  lightweightAgentTasksKeys,
  parseAgentDetailViewState,
  type AgentDetailView,
  type AgentDetailViewState,
} from "@dars/core/lightweight";
import { useNavigation } from "../navigation";

export type DirtyPromptState = {
  message: string;
  onSave: () => Promise<void>;
  onDiscard: () => void;
  onCancel: () => void;
};

export function useAgentDetailViewState(): {
  state: AgentDetailViewState;
  setView: (view: AgentDetailView, extra?: Partial<AgentDetailViewState>) => void;
  setState: (patch: Partial<AgentDetailViewState>) => void;
} {
  const navigation = useNavigation();
  const state = parseAgentDetailViewState(navigation.searchParams);

  const pushState = useCallback(
    (patch: Partial<AgentDetailViewState> & Pick<AgentDetailViewState, "view">) => {
      const next = { ...state, ...patch };
      navigation.push(agentDetailHref(navigation.pathname, next));
    },
    [navigation, state],
  );

  return {
    state,
    setView: (view, extra) => pushState({ ...extra, view }),
    setState: (patch) => pushState({ ...state, ...patch, view: patch.view ?? state.view }),
  };
}

export function useDirtyNavigationGuard(options: {
  dirty: boolean;
  onSave: () => Promise<void>;
  onDiscard: () => void;
}): {
  prompt: DirtyPromptState | null;
  requestNavigation: (action: () => void, message?: string) => void;
  clearPrompt: () => void;
} {
  const [prompt, setPrompt] = useState<DirtyPromptState | null>(null);
  const pendingAction = useRef<(() => void) | null>(null);

  useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!options.dirty) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [options.dirty]);

  const requestNavigation = useCallback(
    (action: () => void, message = "You have unsaved changes.") => {
      if (!options.dirty) {
        action();
        return;
      }
      pendingAction.current = action;
      setPrompt({
        message,
        onSave: async () => {
          await options.onSave();
          pendingAction.current?.();
          pendingAction.current = null;
          setPrompt(null);
        },
        onDiscard: () => {
          options.onDiscard();
          pendingAction.current?.();
          pendingAction.current = null;
          setPrompt(null);
        },
        onCancel: () => {
          pendingAction.current = null;
          setPrompt(null);
        },
      });
    },
    [options],
  );

  return {
    prompt,
    requestNavigation,
    clearPrompt: () => setPrompt(null),
  };
}

export function useAgentDetailReconnectRefetch(workspaceId: string, agentId: string): void {
  const queryClient = useQueryClient();
  useEffect(() => {
    const refetch = () => {
      void queryClient.invalidateQueries({ queryKey: lightweightAgentDetailKeys.one(workspaceId, agentId) });
      void queryClient.invalidateQueries({ queryKey: lightweightAgentTasksKeys.all(workspaceId, agentId) });
      void queryClient.invalidateQueries({ queryKey: lightweightAgentSnapshotKeys.detail(workspaceId) });
      void queryClient.invalidateQueries({ queryKey: ["lightweight", workspaceId, "skills"] });
    };
    const handleVisibility = () => {
      if (document.visibilityState === "visible") refetch();
    };
    window.addEventListener("online", refetch);
    document.addEventListener("visibilitychange", handleVisibility);
    return () => {
      window.removeEventListener("online", refetch);
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [agentId, queryClient, workspaceId]);
}

export function classifyAgentUpdateError(error: unknown): {
  conflict: boolean;
  message: string;
} {
  const message = error instanceof Error ? error.message : "Save failed";
  return {
    conflict: error instanceof ApiError && error.status === 409,
    message,
  };
}
