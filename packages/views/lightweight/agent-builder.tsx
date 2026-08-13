"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import { Label } from "@dars/ui/components/ui/label";
import { Textarea } from "@dars/ui/components/ui/textarea";
import {
  decodeBuilderInput,
  encodeBuilderInput,
  EMPTY_AGENT_DRAFT,
  lightweightAgentBuilderSessionsOptions,
  lightweightAgentBuilderKeys,
  mergeBuilderDraft,
  parseBuilderDraft,
  stripBuilderDraft,
  toStoredAgentDraft,
  type StoredAgentDraft,
} from "@dars/core/lightweight";
import { lightweightApi, lightweightKeys, newIdempotencyKey } from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { useNavigation } from "../navigation";
import { EmptyState, ErrorState, Field, PageFrame, Panel, SubmitButton, formValue, onForm } from "./components";

function useOnlineRuntimes(workspaceId: string) {
  const runtimes = useQuery({ queryKey: lightweightKeys.runtimes(workspaceId), queryFn: lightweightApi.listRuntimes, enabled: !!workspaceId });
  return useMemo(() => (runtimes.data ?? []).filter((runtime) => runtime.status === "online"), [runtimes.data]);
}

function useBuilderDraftAutosave(sessionId: string, draft: StoredAgentDraft, dirty: boolean) {
  const lastSaved = useRef(JSON.stringify(draft));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    if (!sessionId || !dirty) return;
    const serialized = JSON.stringify(draft);
    if (serialized === lastSaved.current) return;
    const timer = window.setTimeout(() => {
      setSaving(true);
      void lightweightApi.saveAgentBuilderDraft(sessionId, draft)
        .then(() => {
          lastSaved.current = serialized;
          setError(null);
        })
        .catch((err: unknown) => setError(err instanceof Error ? err.message : "Autosave failed"))
        .finally(() => setSaving(false));
    }, 500);
    return () => window.clearTimeout(timer);
  }, [dirty, draft, sessionId]);
  return { saving, error };
}

export function AiCreateAgentEntryPage() {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const workspaceId = workspace?.id ?? "";
  const sessions = useQuery(lightweightAgentBuilderSessionsOptions(workspaceId));
  const onlineRuntimes = useOnlineRuntimes(workspaceId);
  const start = useMutation({
    mutationFn: (body: { runtime_id: string; model?: string }) => lightweightApi.createAgentBuilderSession(body),
    onSuccess: (session) => navigation.push(paths.newAgentAiSession(session.session_id)),
  });

  return (
    <PageFrame
      title="AI Builder"
      description="Start a guided conversation to draft an agent configuration, then finalize it into a real agent."
      action={<Button variant="outline" onClick={() => navigation.push(paths.newAgent())}>Back</Button>}
    >
      <div className="grid gap-5 lg:grid-cols-[20rem_1fr]">
        <Panel>
          <h2 className="text-body font-semibold">Resume a draft</h2>
          <div className="mt-3 grid gap-2">
            {(sessions.data ?? []).map((session) => (
              <button
                key={session.session_id}
                type="button"
                className="rounded-lg border px-3 py-2 text-left hover:bg-muted/40"
                onClick={() => navigation.push(paths.newAgentAiSession(session.session_id))}
              >
                <div className="font-medium">{session.draft?.name || session.title}</div>
                <div className="text-caption text-muted-foreground">{session.last_message_content || "Saved draft"}</div>
              </button>
            ))}
            {sessions.isSuccess && (sessions.data?.length ?? 0) === 0 ? (
              <p className="text-body text-muted-foreground">No unfinished sessions.</p>
            ) : null}
          </div>
        </Panel>
        <Panel>
          <form
            className="grid gap-4"
            onSubmit={onForm(async (form) => start.mutateAsync({
              runtime_id: formValue(form, "runtime_id"),
              model: formValue(form, "model") || undefined,
            }))}
          >
            <div className="grid gap-2">
              <label className="text-body font-medium" htmlFor="runtime_id">Online runtime</label>
              <select id="runtime_id" name="runtime_id" required className="h-9 rounded-md border bg-background px-3 text-body">
                <option value="">Select runtime</option>
                {onlineRuntimes.map((runtime) => (
                  <option key={runtime.id} value={runtime.id}>{runtime.name}</option>
                ))}
              </select>
            </div>
            <Field label="Model (optional)" name="model" placeholder="Leave empty for runtime default" />
            <ErrorState error={start.error} />
            <SubmitButton pending={start.isPending}>Start builder session</SubmitButton>
          </form>
        </Panel>
      </div>
    </PageFrame>
  );
}

export function AiBuilderSessionPage({ sessionId }: { sessionId: string }) {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const workspaceId = workspace?.id ?? "";
  const sessions = useQuery(lightweightAgentBuilderSessionsOptions(workspaceId));
  const session = sessions.data?.find((item) => item.session_id === sessionId);
  const onlineRuntimes = useOnlineRuntimes(workspaceId);
  const skills = useQuery({ queryKey: lightweightKeys.skills(workspaceId), queryFn: lightweightApi.listSkills, enabled: !!workspaceId });
  const messages = useQuery({
    queryKey: [...lightweightAgentBuilderKeys.all(workspaceId), "messages", sessionId],
    queryFn: () => lightweightApi.listChatMessages(sessionId),
    enabled: !!sessionId,
    refetchInterval: 2000,
  });
  const pending = useQuery({
    queryKey: [...lightweightAgentBuilderKeys.all(workspaceId), "pending", sessionId],
    queryFn: () => lightweightApi.getPendingChatTask(sessionId),
    enabled: !!sessionId,
    refetchInterval: 1500,
  });
  const [draft, setDraft] = useState<StoredAgentDraft>(() => toStoredAgentDraft(EMPTY_AGENT_DRAFT));
  const [composer, setComposer] = useState("");
  const [dirty, setDirty] = useState(false);
  const [switchingRuntime, setSwitchingRuntime] = useState(false);
  const autosave = useBuilderDraftAutosave(sessionId, draft, dirty);
  const skillIdSet = useMemo(() => new Set((skills.data ?? []).map((skill) => skill.id)), [skills.data]);
  const appliedMessage = useRef<string | null>(null);

  const hydratedSessionId = useRef<string | null>(null);
  useEffect(() => {
    if (!session?.draft) return;
    if (hydratedSessionId.current === sessionId) return;
    hydratedSessionId.current = sessionId;
    setDraft(session.draft);
    setDirty(false);
  }, [session?.draft, sessionId]);

  useEffect(() => {
    const latest = [...(messages.data?.items ?? [])].reverse().find((message) => message.role === "assistant" && parseBuilderDraft(message.content));
    if (!latest || appliedMessage.current === latest.id) return;
    const payload = parseBuilderDraft(latest.content);
    if (!payload) return;
    setDraft((current) => mergeBuilderDraft(current, payload, skillIdSet, new Set()));
    appliedMessage.current = latest.id;
    setDirty(true);
  }, [messages.data?.items, skillIdSet]);

  useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!dirty) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [dirty]);

  const leave = useCallback(() => {
    if (dirty && !window.confirm("Leave without saving your latest draft edits?")) return;
    navigation.push(paths.newAgentAi());
  }, [dirty, navigation, paths]);

  const send = useMutation({
    mutationFn: (content: string) => lightweightApi.sendChatMessage(sessionId, encodeBuilderInput(content, draft)),
    onSuccess: () => {
      setComposer("");
      void queryClient.invalidateQueries({ queryKey: lightweightAgentBuilderKeys.sessions(workspaceId) });
      void queryClient.invalidateQueries({ queryKey: [...lightweightAgentBuilderKeys.all(workspaceId), "messages", sessionId] });
    },
  });

  const finalize = useMutation({
    mutationFn: () => lightweightApi.saveAgentBuilderDraft(sessionId, draft, {
      finalize: true,
      idempotencyKey: newIdempotencyKey("builder-finalize"),
    }),
    onSuccess: (result) => {
      const agentId = result && typeof result === "object" && "agent_id" in result ? result.agent_id : null;
      if (agentId) navigation.push(paths.agentDetail(agentId));
      else navigation.push(paths.agents());
    },
  });

  const switchRuntime = async (runtimeId: string) => {
    if (!runtimeId || switchingRuntime) return;
    setSwitchingRuntime(true);
    try {
      const result = await lightweightApi.switchAgentBuilderRuntime(sessionId, runtimeId);
      setDraft((current) => ({ ...current, model: "" }));
      void queryClient.invalidateQueries({ queryKey: lightweightAgentBuilderKeys.sessions(workspaceId) });
      return result.runtime_id;
    } finally {
      setSwitchingRuntime(false);
    }
  };

  const pendingActive = !!pending.data?.task_id;

  return (
    <PageFrame
      title="AI Builder session"
      description={session?.title ?? "Design an agent through conversation and structured draft edits."}
      action={<Button variant="outline" onClick={leave}>Back</Button>}
    >
      <div className="grid gap-5 xl:grid-cols-[1.2fr_0.8fr]">
        <Panel className="flex min-h-[32rem] flex-col gap-4">
          <div className="flex-1 space-y-3 overflow-y-auto">
            {(messages.data?.items ?? []).map((message) => (
              <article
                key={message.id}
                className={message.role === "user" ? "ml-10 rounded-lg bg-primary p-3 text-primary-foreground" : "mr-10 rounded-lg bg-muted p-3"}
              >
                <div className="mb-1 text-micro opacity-70">{message.role}</div>
                <p className="whitespace-pre-wrap text-body">
                  {message.role === "user" ? decodeBuilderInput(message.content) : stripBuilderDraft(message.content)}
                </p>
              </article>
            ))}
            {!messages.data?.items?.length ? <EmptyState>Send the first message to start shaping the agent.</EmptyState> : null}
          </div>
          <form
            className="grid gap-2 border-t pt-4"
            onSubmit={(event) => {
              event.preventDefault();
              const text = composer.trim();
              if (!text || pendingActive || switchingRuntime) return;
              void send.mutateAsync(text);
            }}
          >
            <Textarea
              value={composer}
              onChange={(event) => setComposer(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault();
                  event.currentTarget.form?.requestSubmit();
                }
              }}
              placeholder="Describe the agent you want to build"
              rows={3}
            />
            <div className="flex items-center justify-between gap-3">
              <span className="text-caption text-muted-foreground">
                {pendingActive ? "Waiting for reply…" : autosave.saving ? "Saving draft…" : dirty ? "Unsaved edits" : "Draft saved"}
              </span>
              <SubmitButton pending={send.isPending || pendingActive || switchingRuntime}>Send</SubmitButton>
            </div>
            <ErrorState error={send.error ?? autosave.error} />
          </form>
        </Panel>

        <Panel className="grid gap-4">
          <div className="grid gap-2">
            <label className="text-body font-medium" htmlFor="builder-runtime">Runtime</label>
            <select
              id="builder-runtime"
              className="h-9 rounded-md border bg-background px-3 text-body"
              value={session?.runtime_id ?? ""}
              disabled={pendingActive || switchingRuntime}
              onChange={(event) => void switchRuntime(event.target.value)}
            >
              <option value="">Select runtime</option>
              {onlineRuntimes.map((runtime) => (
                <option key={runtime.id} value={runtime.id}>{runtime.name}</option>
              ))}
            </select>
            {pendingActive ? <p className="text-caption text-amber-700">Stop the active reply before switching runtime.</p> : null}
          </div>
          <div className="grid gap-2">
            <Label htmlFor="builder-name">Name</Label>
            <Input id="builder-name" value={draft.name} onChange={(event) => { setDraft((current) => ({ ...current, name: event.target.value })); setDirty(true); }} required />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="builder-description">Description</Label>
            <Textarea id="builder-description" value={draft.description} onChange={(event) => { setDraft((current) => ({ ...current, description: event.target.value })); setDirty(true); }} rows={2} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="builder-instructions">Instructions</Label>
            <Textarea id="builder-instructions" value={draft.instructions} onChange={(event) => { setDraft((current) => ({ ...current, instructions: event.target.value })); setDirty(true); }} rows={8} />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="builder-model">Model</Label>
            <Input id="builder-model" value={draft.model} onChange={(event) => { setDraft((current) => ({ ...current, model: event.target.value })); setDirty(true); }} />
          </div>
          <div className="grid gap-2">
            <div className="text-body font-medium">Workspace skills</div>
            {(skills.data ?? []).map((skill) => (
              <label key={skill.id} className="flex items-center gap-2 text-body">
                <Checkbox
                  checked={draft.skill_ids.includes(skill.id)}
                  onChange={() => {
                    setDraft((current) => ({
                      ...current,
                      skill_ids: current.skill_ids.includes(skill.id)
                        ? current.skill_ids.filter((id) => id !== skill.id)
                        : [...current.skill_ids, skill.id],
                    }));
                    setDirty(true);
                  }}
                />
                {skill.name}
              </label>
            ))}
          </div>
          <ErrorState error={finalize.error} />
          <Button type="button" disabled={finalize.isPending} onClick={() => finalize.mutate()}>
            {finalize.isPending ? "Creating…" : "Create agent"}
          </Button>
        </Panel>
      </div>
    </PageFrame>
  );
}
