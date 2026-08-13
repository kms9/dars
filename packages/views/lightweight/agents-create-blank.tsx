"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@dars/core/auth";
import {
  applyDraftModelChange,
  applyDraftRuntimeChange,
  EMPTY_AGENT_DRAFT,
  isRuntimeUsableForUser,
  lightweightApi,
  lightweightKeys,
  runtimeDisplayLabel,
  type AgentDraft,
} from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import { Textarea } from "@dars/ui/components/ui/textarea";
import { useNavigation } from "../navigation";
import {
  clearManualDraftForOwner,
  useCreateAgentSubmit,
  useDuplicateDraftSeed,
  useManualDraftSync,
  useStableSubmitGuard,
} from "./agents-create-hooks";
import { ErrorState, PageFrame, Panel } from "./components";

function AvatarPreview({ draft, onPick }: { draft: AgentDraft; onPick: (file: File | null) => void }) {
  const [previewUrl, setPreviewUrl] = useState<string | null>(draft.avatarUrl);

  useEffect(() => {
    if (draft.avatarFile) {
      const url = URL.createObjectURL(draft.avatarFile);
      setPreviewUrl(url);
      return () => URL.revokeObjectURL(url);
    }
    setPreviewUrl(draft.avatarUrl);
    return undefined;
  }, [draft.avatarFile, draft.avatarUrl]);

  return (
    <div className="flex items-center gap-4">
      {previewUrl ? (
        <img src={previewUrl} alt="" className="size-16 rounded-full object-cover" />
      ) : (
        <div className="flex size-16 items-center justify-center rounded-full bg-muted text-title-sm font-semibold">
          {draft.name.slice(0, 1).toUpperCase() || "A"}
        </div>
      )}
      <div className="grid gap-2">
        <Input
          type="file"
          accept="image/png,image/jpeg"
          onChange={(event) => onPick(event.target.files?.[0] ?? null)}
        />
        <p className="text-caption text-muted-foreground">Avatar uploads after the agent is created.</p>
      </div>
    </div>
  );
}

export function ManualCreateAgentPage() {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const user = useAuthStore((state) => state.user);
  const queryClient = useQueryClient();
  const duplicateId = navigation.searchParams.get("duplicate");
  const fromSquadId = navigation.searchParams.get("from_squad");
  const [draft, setDraft] = useState<AgentDraft>(EMPTY_AGENT_DRAFT);
  const [duplicateRuntimeReset, setDuplicateRuntimeReset] = useState(false);
  const [duplicateSeeded, setDuplicateSeeded] = useState(!duplicateId);
  const [joinPrompt, setJoinPrompt] = useState<{ agentId: string } | null>(null);
  const [joinError, setJoinError] = useState<string | null>(null);
  const [joinPending, setJoinPending] = useState(false);

  const returnSquad = useQuery({
    queryKey: [...lightweightKeys.squads(workspace?.id ?? ""), fromSquadId ?? ""],
    queryFn: () => lightweightApi.getSquad(fromSquadId!),
    enabled: !!workspace && !!fromSquadId,
  });

  const agents = useQuery({
    queryKey: lightweightKeys.agents(workspace?.id ?? ""),
    queryFn: lightweightApi.listAgents,
    enabled: !!workspace,
  });
  const runtimes = useQuery({
    queryKey: lightweightKeys.runtimes(workspace?.id ?? ""),
    queryFn: lightweightApi.listRuntimes,
    enabled: !!workspace,
  });
  const skills = useQuery({
    queryKey: lightweightKeys.skills(workspace?.id ?? ""),
    queryFn: lightweightApi.listSkills,
    enabled: !!workspace,
  });
  const members = useQuery({
    queryKey: lightweightKeys.members(workspace?.id ?? ""),
    queryFn: () => lightweightApi.listMembers(workspace!.id),
    enabled: !!workspace,
  });

  const duplicateAgent = useMemo(
    () => (duplicateId ? agents.data?.find((agent) => agent.id === duplicateId) ?? null : null),
    [agents.data, duplicateId],
  );

  const usableRuntimes = useMemo(
    () => (runtimes.data ?? []).filter((runtime) => isRuntimeUsableForUser(runtime, user?.id ?? null)),
    [runtimes.data, user?.id],
  );

  useDuplicateDraftSeed({
    source: duplicateAgent,
    runtimesSettled: !runtimes.isPending,
    runtimes: runtimes.data ?? [],
    currentUserId: user?.id ?? null,
    fallbackRuntimeId: usableRuntimes[0]?.id ?? "",
    nameSuffix: " copy",
    onSeed: (seeded, runtimeReset) => {
      setDuplicateRuntimeReset(runtimeReset);
      setDraft(seeded);
      setDuplicateSeeded(true);
    },
  });

  useManualDraftSync({
    duplicateId,
    draft,
    setDraft,
    ready: !duplicateId || duplicateSeeded,
  });

  const submit = useCreateAgentSubmit({
    draft,
    runtimeId: draft.runtimeId || null,
    duplicateSource: duplicateAgent,
    onCreated: () => clearManualDraftForOwner(duplicateId),
    createAgent: lightweightApi.createAgent,
    setAgentSkills: lightweightApi.setAgentSkills,
    uploadAvatar: (agentId, file) => lightweightApi.uploadAgentAvatar(agentId, file, file.name),
    onSuccess: (agentId) => {
      void queryClient.invalidateQueries({ queryKey: lightweightKeys.agents(workspace?.id ?? "") });
      if (fromSquadId && returnSquad.data && !returnSquad.data.archived_at) {
        setJoinPrompt({ agentId });
        return;
      }
      navigation.push(fromSquadId ? paths.squadDetail(fromSquadId) : paths.agentDetail(agentId));
    },
  });

  const { guard } = useStableSubmitGuard();
  const canCreate = draft.name.trim().length > 0 && !!draft.runtimeId && !submit.creating;

  return (
    <PageFrame
      title={duplicateAgent ? `Duplicate ${duplicateAgent.name}` : "Create agent"}
      description="Name and a bindable local runtime are required. Draft restores within this browser session."
      action={
        <Button
          variant="outline"
          onClick={() =>
            navigation.push(
              fromSquadId ? paths.squadDetail(fromSquadId) : duplicateId ? paths.agents() : paths.newAgent(),
            )
          }
        >
          Back
        </Button>
      }
    >
      {fromSquadId && returnSquad.data ? (
        <Panel>
          <p className="text-body text-muted-foreground">
            Creating an agent for squad <strong>{returnSquad.data.name}</strong>. After creation you can confirm adding
            it to the roster before returning to the squad.
          </p>
        </Panel>
      ) : null}
      {duplicateAgent ? (
        <Panel>
          <p className="text-body text-muted-foreground">
            Instructions, skills, and non-secret configuration are copied. Identity, owner, history, and environment
            secrets are not.
          </p>
        </Panel>
      ) : null}
      {duplicateRuntimeReset ? (
        <Panel>
          <p className="text-body text-warning">The source runtime is unavailable. Pick a runtime and model again.</p>
        </Panel>
      ) : null}

      <Panel>
        <div className="grid gap-5">
          <AvatarPreview
            draft={draft}
            onPick={(file) => setDraft((current) => ({ ...current, avatarFile: file, avatarUrl: null }))}
          />
          <label className="grid gap-2 text-body">
            <span className="font-medium">Name</span>
            <Input
              value={draft.name}
              required
              onChange={(event) => {
                submit.clearNameError();
                setDraft((current) => ({ ...current, name: event.target.value }));
              }}
            />
          </label>
          {submit.nameError ? <p className="text-body text-destructive">{submit.nameError}</p> : null}
          <label className="grid gap-2 text-body">
            <span className="font-medium">Description</span>
            <Textarea
              value={draft.description}
              onChange={(event) => setDraft((current) => ({ ...current, description: event.target.value }))}
              rows={3}
            />
          </label>
          <label className="grid gap-2 text-body">
            <span className="font-medium">Instructions</span>
            <Textarea
              value={draft.instructions}
              onChange={(event) => setDraft((current) => ({ ...current, instructions: event.target.value }))}
              rows={8}
            />
          </label>

          <label className="grid gap-2 text-body">
            <span className="font-medium">Runtime</span>
            <select
              className="h-9 rounded-md border bg-background px-3"
              value={draft.runtimeId}
              onChange={(event) => setDraft((current) => applyDraftRuntimeChange(current, event.target.value))}
              required
            >
              <option value="">Select runtime</option>
              {usableRuntimes.map((runtime) => (
                <option key={runtime.id} value={runtime.id}>
                  {runtimeDisplayLabel(runtime)} · {runtime.status}
                </option>
              ))}
            </select>
          </label>

          <div className="grid gap-4 md:grid-cols-3">
            <label className="grid gap-2 text-body">
              <span className="font-medium">Model</span>
              <Input
                value={draft.model}
                onChange={(event) => setDraft((current) => applyDraftModelChange(current, event.target.value))}
                placeholder="Optional"
              />
            </label>
            <label className="grid gap-2 text-body">
              <span className="font-medium">Thinking</span>
              <Input
                value={draft.thinkingLevel}
                onChange={(event) => setDraft((current) => ({ ...current, thinkingLevel: event.target.value }))}
                placeholder="Optional"
              />
            </label>
            <label className="grid gap-2 text-body">
              <span className="font-medium">Service tier</span>
              <Input
                value={draft.serviceTier}
                onChange={(event) => setDraft((current) => ({ ...current, serviceTier: event.target.value }))}
                placeholder="Optional"
              />
            </label>
          </div>

          <label className="grid gap-2 text-body">
            <span className="font-medium">Max concurrent tasks</span>
            <Input
              type="number"
              min={1}
              max={32}
              value={draft.maxConcurrentTasks}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  maxConcurrentTasks: Number(event.target.value || "1"),
                }))
              }
            />
          </label>

          <label className="grid gap-2 text-body">
            <span className="font-medium">Access</span>
            <select
              className="h-9 rounded-md border bg-background px-3"
              value={draft.permissionScope}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  permissionScope: event.target.value as AgentDraft["permissionScope"],
                  memberIds: event.target.value === "members" ? current.memberIds : new Set(),
                }))
              }
            >
              <option value="private">Private</option>
              <option value="workspace">Workspace</option>
              <option value="members">Specified members</option>
            </select>
          </label>

          {draft.permissionScope === "members" ? (
            <div className="grid gap-2">
              {(members.data ?? []).map((member) => (
                <label key={member.id} className="flex items-center gap-2 text-body">
                  <Checkbox
                    checked={draft.memberIds.has(member.user_id)}
                    onChange={() =>
                      setDraft((current) => {
                        const next = new Set(current.memberIds);
                        if (next.has(member.user_id)) next.delete(member.user_id);
                        else next.add(member.user_id);
                        return { ...current, memberIds: next };
                      })
                    }
                  />
                  <span>{member.name}</span>
                  <Badge variant="outline">{member.role}</Badge>
                </label>
              ))}
            </div>
          ) : null}

          <div className="grid gap-2">
            <span className="font-medium">Workspace skills</span>
            {(skills.data ?? []).map((skill) => (
              <label key={skill.id} className="flex items-center gap-2 text-body">
                <Checkbox
                  checked={draft.skillIds.has(skill.id)}
                  onChange={() =>
                    setDraft((current) => {
                      const next = new Set(current.skillIds);
                      if (next.has(skill.id)) next.delete(skill.id);
                      else next.add(skill.id);
                      return { ...current, skillIds: next };
                    })
                  }
                />
                <span>{skill.name}</span>
              </label>
            ))}
            {(skills.data?.length ?? 0) === 0 ? (
              <p className="text-body text-muted-foreground">No workspace skills yet.</p>
            ) : null}
          </div>

          <ErrorState error={submit.formError} />
          {submit.avatarError ? (
            <Panel>
              <p className="text-body text-destructive">Agent created, but avatar upload failed: {submit.avatarError}</p>
              <Button className="mt-3" variant="outline" onClick={() => void submit.retryAvatar()}>
                Retry avatar upload
              </Button>
            </Panel>
          ) : null}

          <div className="flex flex-wrap gap-3">
            <Button
              disabled={!canCreate || submit.creating}
              onClick={() => void guard(() => submit.create(true))}
            >
              {submit.creating ? "Creating…" : "Create and open"}
            </Button>
            <Button
              variant="outline"
              disabled={!canCreate || submit.creating}
              onClick={() => void guard(() => submit.create(false))}
            >
              Create
            </Button>
          </div>
        </div>
      </Panel>

      {joinPrompt !== null ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/70 p-4 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-xl border bg-card p-5 shadow-lg">
            <h2 className="text-title-sm font-semibold">Add agent to squad?</h2>
            <p className="mt-2 text-body text-muted-foreground">
              {returnSquad.data
                ? `Add the new agent as a member of ${returnSquad.data.name}. It will not become the leader.`
                : "Add the new agent to the squad roster."}
            </p>
            {joinError ? <p className="mt-3 text-body text-destructive">{joinError}</p> : null}
            <div className="mt-4 flex justify-end gap-2">
              <Button
                variant="outline"
                disabled={joinPending}
                onClick={() => {
                  setJoinPrompt(null);
                  if (fromSquadId) navigation.push(paths.squadDetail(fromSquadId));
                }}
              >
                Skip
              </Button>
              <Button
                disabled={joinPending || !fromSquadId || !joinPrompt}
                onClick={async () => {
                  if (!fromSquadId || !joinPrompt) return;
                  setJoinPending(true);
                  setJoinError(null);
                  try {
                    await lightweightApi.addSquadMember(fromSquadId, joinPrompt.agentId, "member");
                    void queryClient.invalidateQueries({ queryKey: lightweightKeys.squads(workspace?.id ?? "") });
                    setJoinPrompt(null);
                    navigation.push(paths.squadDetail(fromSquadId));
                  } catch (error) {
                    setJoinError(error instanceof Error ? error.message : "Failed to add to squad");
                  } finally {
                    setJoinPending(false);
                  }
                }}
              >
                {joinPending ? "Adding…" : "Add to squad"}
              </Button>
            </div>
          </div>
        </div>
      ) : null}
    </PageFrame>
  );
}
