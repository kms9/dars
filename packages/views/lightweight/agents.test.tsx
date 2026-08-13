// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import {
  EMPTY_AGENT_DRAFT,
  toStoredAgentDraft,
  useManualAgentDraftStore,
} from "@dars/core/lightweight";
import { useManualDraftSync } from "./agents-create-hooks";

function seedStoredDraft(owner: string, name: string) {
  useManualAgentDraftStore.getState().setDraft({
    byOwner: {
      [owner]: {
        runtimeId: "runtime-1",
        draft: toStoredAgentDraft({ ...EMPTY_AGENT_DRAFT, name }),
      },
    },
  });
}

afterEach(() => {
  cleanup();
  useManualAgentDraftStore.getState().clearDraft();
});

describe("useManualDraftSync", () => {
  it("restores only the current flow draft", () => {
    seedStoredDraft("duplicate:agent-A", "Half-finished copy");
    const setDraft = vi.fn();
    renderHook(() =>
      useManualDraftSync({
        duplicateId: "agent-A",
        draft: EMPTY_AGENT_DRAFT,
        setDraft,
        ready: true,
      }),
    );
    expect(setDraft).toHaveBeenCalledTimes(1);
    expect(setDraft.mock.calls[0]?.[0]).toMatchObject({ name: "Half-finished copy" });
  });

  it("does not restore another flow draft", () => {
    seedStoredDraft("duplicate:agent-A", "Half-finished copy");
    const setDraft = vi.fn();
    renderHook(() =>
      useManualDraftSync({
        duplicateId: null,
        draft: EMPTY_AGENT_DRAFT,
        setDraft,
        ready: true,
      }),
    );
    expect(setDraft).not.toHaveBeenCalled();
  });
});

describe("classifyAgentCreateError", () => {
  it("maps conflict errors to name errors", async () => {
    const { classifyAgentCreateError } = await import("./agents-create-hooks");
    const { ApiError } = await import("@dars/core/api");
    expect(classifyAgentCreateError(new ApiError("conflict", 409, "Conflict"), "failed", "name taken")).toEqual({
      nameError: "name taken",
      formError: null,
    });
  });
});
