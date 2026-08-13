"use client";
import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@dars/ui/components/ui/button";
import { Textarea } from "@dars/ui/components/ui/textarea";
import { lightweightApi, lightweightKeys } from "@dars/core/lightweight";
import { useCurrentWorkspace } from "@dars/core/paths";
import { useNavigation } from "../navigation";
import { EmptyState, ErrorState, Field, PageFrame, Panel, SubmitButton, formValue, onForm } from "./components";

export function ChatPage() {
  const workspace = useCurrentWorkspace();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const workspaceId = workspace?.id ?? "";
  const selectedId = navigation.searchParams.get("session") ?? "";
  const sessionsKey = lightweightKeys.chats(workspaceId);
  const sessions = useQuery({ queryKey: sessionsKey, queryFn: () => lightweightApi.listChatSessions(), enabled: !!workspace });
  const agents = useQuery({ queryKey: lightweightKeys.agents(workspaceId), queryFn: lightweightApi.listAgents, enabled: !!workspace });
  const messages = useQuery({
    queryKey: [...sessionsKey, selectedId, "messages"],
    queryFn: () => lightweightApi.listChatMessages(selectedId),
    enabled: !!selectedId,
    refetchInterval: selectedId ? 3000 : false,
  });
  const pending = useQuery({
    queryKey: [...sessionsKey, selectedId, "pending"],
    queryFn: () => lightweightApi.getPendingChatTask(selectedId),
    enabled: !!selectedId,
    refetchInterval: selectedId ? 2000 : false,
  });
  const pendingTaskId = pending.data?.task_id ?? "";
  const taskMessages = useQuery({
    queryKey: lightweightKeys.taskMessages(workspaceId, pendingTaskId),
    queryFn: () => lightweightApi.listTaskMessages(pendingTaskId),
    enabled: !!selectedId && !!pendingTaskId,
    refetchInterval: pendingTaskId ? 750 : false,
  });
  const restores = useQuery({ queryKey: [...sessionsKey, selectedId, "restores"], queryFn: () => lightweightApi.listDraftRestores(selectedId), enabled: !!selectedId });
  const selected = useMemo(() => sessions.data?.items.find((item) => item.id === selectedId), [selectedId, sessions.data]);
  const streamingText = useMemo(() => taskMessages.data?.items
    .filter((message) => message.type === "text" && message.content)
    .sort((left, right) => left.seq - right.seq)
    .map((message) => message.content)
    .join("\n\n") ?? "", [taskMessages.data]);
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: sessionsKey });
  };
  const create = useMutation({ mutationFn: (body: { agentId: string; title: string }) => lightweightApi.createChatSession(body.agentId, body.title), onSuccess: (value) => { refresh(); navigation.push(`${navigation.pathname}?session=${value.id}`); } });
  const send = useMutation({ mutationFn: (content: string) => lightweightApi.sendChatMessage(selectedId, content), onSuccess: () => refresh() });
  const cancel = useMutation({ mutationFn: (taskId: string) => lightweightApi.cancelTask(taskId), onSuccess: refresh });
  const archive = useMutation({ mutationFn: () => lightweightApi.updateChatSession(selectedId, { status: selected?.status === "archived" ? "active" : "archived" }), onSuccess: refresh });

  return (
    <PageFrame title="Direct chat" description="One agent, one runtime-backed session, plain text messages, and resumable pending tasks.">
      <div className="grid min-h-[36rem] gap-5 lg:grid-cols-[19rem_1fr]">
        <Panel className="flex flex-col gap-4">
          <form className="grid gap-3" onSubmit={onForm(async (form) => create.mutateAsync({ agentId: formValue(form, "agent_id"), title: formValue(form, "title") }))}>
            <label className="text-body font-medium" htmlFor="agent_id">Agent</label>
            <select id="agent_id" name="agent_id" required className="h-9 rounded-md border bg-background px-3 text-body">
              <option value="">Select an online agent</option>
              {(agents.data ?? []).filter((agent) => !agent.archived_at && agent.runtime_id).map((agent) => <option key={agent.id} value={agent.id}>{agent.name}</option>)}
            </select>
            <Field label="Title" name="title" placeholder="Optional conversation title" />
            <ErrorState error={create.error} />
            <SubmitButton pending={create.isPending}>New session</SubmitButton>
          </form>
          <div className="grid gap-1 border-t pt-4">
            {sessions.data?.items.map((session) => (
              <Button
                key={session.id}
                variant={selectedId === session.id ? "secondary" : "ghost"}
                className="justify-start overflow-hidden"
                onClick={() => navigation.push(`${navigation.pathname}?session=${session.id}`)}
              >
                <span className="truncate">{session.title}</span>
              </Button>
            ))}
            {sessions.data?.items.length === 0 ? <p className="text-body text-muted-foreground">No sessions.</p> : null}
          </div>
        </Panel>

        <Panel className="flex min-h-[36rem] flex-col">
          {!selected ? <EmptyState>Select or create a direct chat session.</EmptyState> : (
            <>
              <div className="flex items-start justify-between gap-3 border-b pb-4">
                <div><h2 className="font-semibold">{selected.title}</h2><p className="text-caption text-muted-foreground">{selected.status}</p></div>
                <Button variant="outline" size="sm" onClick={() => archive.mutate()}>{selected.status === "archived" ? "Restore" : "Archive"}</Button>
              </div>
              <div className="flex-1 space-y-3 overflow-y-auto py-4">
                {restores.data?.map((restore) => (
                  <div key={restore.id} className="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-body">
                    <div className="text-caption font-medium text-amber-700">Recovered draft</div>
                    <p className="mt-1 whitespace-pre-wrap">{restore.content}</p>
                    <Button size="sm" variant="ghost" onClick={() => lightweightApi.consumeDraftRestore(selectedId, restore.id).then(refresh)}>Dismiss</Button>
                  </div>
                ))}
                {messages.data?.items.map((message) => (
                  <article key={message.id} className={message.role === "user" ? "ml-12 rounded-lg bg-primary p-3 text-body text-primary-foreground" : "mr-12 rounded-lg bg-muted p-3 text-body"}>
                    <div className="mb-1 text-micro opacity-70">{message.role}</div>
                    <p className="whitespace-pre-wrap">{message.content || message.failure_reason}</p>
                  </article>
                ))}
                {pendingTaskId && streamingText ? (
                  <article aria-live="polite" aria-label="Assistant response streaming" className="mr-12 rounded-lg bg-muted p-3 text-body">
                    <div className="mb-1 text-micro opacity-70">assistant · streaming</div>
                    <p className="whitespace-pre-wrap">{streamingText}</p>
                  </article>
                ) : null}
              </div>
              {pendingTaskId ? (
                <div className="mb-3 flex items-center justify-between rounded-lg border px-3 py-2 text-body">
                  <span>Task {pending.data?.status}</span>
                  <Button variant="outline" size="sm" onClick={() => cancel.mutate(pendingTaskId)}>Cancel</Button>
                </div>
              ) : null}
              <form className="grid gap-2 border-t pt-4" onSubmit={onForm(async (form) => send.mutateAsync(formValue(form, "content")))}>
                <Textarea name="content" required disabled={selected.status !== "active"} placeholder="Message this agent" />
                <ErrorState error={send.error ?? cancel.error ?? archive.error} />
                <SubmitButton pending={send.isPending}>Send</SubmitButton>
              </form>
            </>
          )}
        </Panel>
      </div>
    </PageFrame>
  );
}
