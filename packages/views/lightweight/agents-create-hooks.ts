"use client";

import { useEffect, useRef, useState, type Dispatch, type SetStateAction } from "react";
import {
  buildCreateAgentBody,
  buildDuplicateDraft,
  fromStoredAgentDraft,
  manualDraftOwner,
  setManualDraftEntry,
  toStoredAgentDraft,
  useManualAgentDraftStore,
  type AgentDraft,
  type LightweightAgent,
} from "@dars/core/lightweight";
import { ApiError } from "@dars/core/api";
import type { LightweightRuntime } from "@dars/core/lightweight";

export function useDuplicateDraftSeed(options: {
  source: LightweightAgent | null;
  runtimesSettled: boolean;
  runtimes: LightweightRuntime[];
  currentUserId: string | null;
  fallbackRuntimeId: string;
  nameSuffix: string;
  onSeed: (draft: AgentDraft, runtimeReset: boolean) => void;
}): void {
  const seededRef = useRef(false);
  const onSeedRef = useRef(options.onSeed);
  onSeedRef.current = options.onSeed;

  useEffect(() => {
    if (seededRef.current || !options.source || !options.runtimesSettled) return;
    seededRef.current = true;
    const draft = buildDuplicateDraft(options.source, {
      runtimes: options.runtimes,
      currentUserId: options.currentUserId,
      fallbackRuntimeId: options.fallbackRuntimeId,
      nameSuffix: options.nameSuffix,
    });
    onSeedRef.current(draft, draft.runtimeId !== (options.source.runtime_id ?? ""));
  }, [
    options.currentUserId,
    options.fallbackRuntimeId,
    options.nameSuffix,
    options.runtimes,
    options.runtimesSettled,
    options.source,
  ]);
}

export function useManualDraftSync(options: {
  duplicateId: string | null;
  draft: AgentDraft;
  setDraft: Dispatch<SetStateAction<AgentDraft>>;
  ready: boolean;
}): void {
  const owner = manualDraftOwner(options.duplicateId);
  const setStored = useManualAgentDraftStore((state) => state.setDraft);
  const [restored, setRestored] = useState(false);

  useEffect(() => {
    if (restored || !options.ready) return;
    const persisted = useManualAgentDraftStore.getState().draft.byOwner[owner];
    if (persisted) options.setDraft(fromStoredAgentDraft(persisted.draft, persisted.runtimeId));
    setRestored(true);
  }, [owner, options.ready, options.setDraft, restored]);

  useEffect(() => {
    if (!restored) return;
    const current = useManualAgentDraftStore.getState().draft;
    setStored(
      setManualDraftEntry(current, owner, {
        runtimeId: options.draft.runtimeId,
        draft: toStoredAgentDraft(options.draft),
      }),
    );
  }, [options.draft, owner, restored, setStored]);
}

export function clearManualDraftForOwner(duplicateId: string | null): void {
  const store = useManualAgentDraftStore.getState();
  store.setDraft(setManualDraftEntry(store.draft, manualDraftOwner(duplicateId), null));
}

export function classifyAgentCreateError(
  error: unknown,
  fallbackMessage: string,
  conflictMessage: string,
): { nameError: string | null; formError: string | null } {
  const message = error instanceof Error && error.message ? error.message : fallbackMessage;
  return error instanceof ApiError && error.status === 409
    ? { nameError: conflictMessage, formError: null }
    : { nameError: null, formError: message };
}

export function useCreateAgentSubmit(options: {
  draft: AgentDraft;
  runtimeId: string | null;
  duplicateSource?: LightweightAgent | null;
  onCreated?: () => void;
  createAgent: (body: Record<string, unknown>) => Promise<LightweightAgent>;
  setAgentSkills: (agentId: string, skills: Array<{ skill_id: string; enabled: boolean }>) => Promise<unknown>;
  uploadAvatar: (agentId: string, file: File) => Promise<unknown>;
  onSuccess: (agentId: string) => void;
}) {
  const [creating, setCreating] = useState(false);
  const [nameError, setNameError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const [avatarRetryAgentId, setAvatarRetryAgentId] = useState<string | null>(null);
  const [avatarError, setAvatarError] = useState<string | null>(null);

  const create = async (openAfterCreate: boolean) => {
    if (!options.runtimeId || creating) return;
    setCreating(true);
    setNameError(null);
    setFormError(null);
    setAvatarError(null);
    try {
      const agent = await options.createAgent(
        buildCreateAgentBody({
          draft: options.draft,
          runtimeId: options.runtimeId,
          duplicateSource: options.duplicateSource,
        }),
      );
      if (options.draft.skillIds.size > 0) {
        await options.setAgentSkills(
          agent.id,
          [...options.draft.skillIds].map((skillId) => ({ skill_id: skillId, enabled: true })),
        );
      }
      options.onCreated?.();
      if (options.draft.avatarFile) {
        try {
          await options.uploadAvatar(agent.id, options.draft.avatarFile);
          setAvatarRetryAgentId(null);
        } catch (error) {
          setAvatarRetryAgentId(agent.id);
          setAvatarError(error instanceof Error ? error.message : "Avatar upload failed");
        }
      }
      if (openAfterCreate) options.onSuccess(agent.id);
      else setCreating(false);
    } catch (error) {
      const next = classifyAgentCreateError(error, "Create failed", "An agent with this name already exists");
      setNameError(next.nameError);
      setFormError(next.formError);
      setCreating(false);
    }
  };

  const retryAvatar = async () => {
    if (!avatarRetryAgentId || !options.draft.avatarFile) return;
    setAvatarError(null);
    try {
      await options.uploadAvatar(avatarRetryAgentId, options.draft.avatarFile);
      setAvatarRetryAgentId(null);
    } catch (error) {
      setAvatarError(error instanceof Error ? error.message : "Avatar upload failed");
    }
  };

  return {
    create,
    creating,
    nameError,
    formError,
    avatarError,
    avatarRetryAgentId,
    retryAvatar,
    clearNameError: () => setNameError(null),
  };
}

export function useStableSubmitGuard() {
  const submittingRef = useRef(false);
  return {
    guard: async (fn: () => Promise<void>) => {
      if (submittingRef.current) return;
      submittingRef.current = true;
      try {
        await fn();
      } finally {
        submittingRef.current = false;
      }
    },
  };
}
