"use client";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button } from "@dars/ui/components/ui/button";
import { Badge } from "@dars/ui/components/ui/badge";
import { Textarea } from "@dars/ui/components/ui/textarea";
import { lightweightApi, lightweightKeys, LIGHTWEIGHT_RUN_STATUSES, type LightweightRunStatus } from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { AppLink } from "../navigation";
import { EmptyState, ErrorState, Field, PageFrame, Panel, SubmitButton, TextField, formValue, onForm } from "./components";

function AssigneeOptions() {
  const workspace = useCurrentWorkspace();
  const agents = useQuery({ queryKey: lightweightKeys.agents(workspace?.id ?? ""), queryFn: lightweightApi.listAgents, enabled: !!workspace });
  const squads = useQuery({ queryKey: lightweightKeys.squads(workspace?.id ?? ""), queryFn: lightweightApi.listSquads, enabled: !!workspace });
  return (
    <>
      <optgroup label="Agents">
        {(agents.data ?? []).filter((agent) => !agent.archived_at).map((agent) => <option key={agent.id} value={`agent:${agent.id}`}>{agent.name}</option>)}
      </optgroup>
      <optgroup label="Squads">
        {(squads.data ?? []).filter((squad) => !squad.archived_at).map((squad) => <option key={squad.id} value={`squad:${squad.id}`}>{squad.name}</option>)}
      </optgroup>
    </>
  );
}

export function RunsPage() {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<string>("");
  const [cursor, setCursor] = useState<string | undefined>();
  const runs = useQuery({
    queryKey: [...lightweightKeys.runs(workspace?.id ?? ""), { status, cursor }],
    queryFn: () => lightweightApi.listRuns({ statuses: status ? [status] : undefined, cursor }),
    enabled: !!workspace,
  });
  const create = useMutation({
    mutationFn: (body: Record<string, unknown>) => lightweightApi.createRun(body),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: lightweightKeys.runs(workspace?.id ?? "") }),
  });

  return (
    <PageFrame title="Runs" description="A stable, cursor-paginated execution queue. Runs are backed by the lightweight Issue core.">
      <Panel>
        <form className="grid gap-4 md:grid-cols-2" onSubmit={onForm(async (form) => {
          const [assignee_type, assignee_id] = formValue(form, "assignee").split(":");
          await create.mutateAsync({
            title: formValue(form, "title"),
            description: formValue(form, "description"),
            status: formValue(form, "status"),
            assignee_type,
            assignee_id,
            acceptance_criteria: [],
            context_refs: [],
          });
        })}>
          <Field label="Title" name="title" required />
          <div className="grid gap-2">
            <label className="text-body font-medium" htmlFor="assignee">Assignee</label>
            <select id="assignee" name="assignee" required className="h-9 rounded-md border bg-background px-3 text-body">
              <option value="">Select an agent or squad</option>
              <AssigneeOptions />
            </select>
          </div>
          <TextField label="Description" name="description" rows={3} />
          <div className="grid content-start gap-2">
            <label className="text-body font-medium" htmlFor="status">Initial status</label>
            <select id="status" name="status" defaultValue="backlog" className="h-9 rounded-md border bg-background px-3 text-body">
              {LIGHTWEIGHT_RUN_STATUSES.map((value) => <option key={value} value={value}>{value}</option>)}
            </select>
          </div>
          <div className="md:col-span-2"><ErrorState error={create.error} /><SubmitButton pending={create.isPending}>Create run</SubmitButton></div>
        </form>
      </Panel>

      <div className="flex items-center gap-3">
        <label className="text-body font-medium" htmlFor="run-status-filter">Status</label>
        <select
          id="run-status-filter"
          value={status}
          onChange={(event) => { setStatus(event.target.value); setCursor(undefined); }}
          className="h-9 rounded-md border bg-background px-3 text-body"
        >
          <option value="">All statuses</option>
          {LIGHTWEIGHT_RUN_STATUSES.map((value) => <option key={value} value={value}>{value}</option>)}
        </select>
      </div>
      <ErrorState error={runs.error} />
      {runs.isPending ? <p className="text-body text-muted-foreground">Loading runs…</p> : null}
      {runs.data?.items.length === 0 ? <EmptyState>No runs yet.</EmptyState> : null}
      <div className="grid gap-3">
        {runs.data?.items.map((run) => (
          <AppLink key={run.id} href={paths.issueDetail(run.id)} className="rounded-xl border bg-card p-4 transition-colors hover:bg-muted/40">
            <div className="flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="text-caption text-muted-foreground">{run.identifier}</div>
                <div className="truncate font-medium">{run.title}</div>
              </div>
              <Badge variant={run.status === "blocked" ? "destructive" : "secondary"}>{run.status}</Badge>
            </div>
            <p className="mt-2 line-clamp-2 text-body text-muted-foreground">{run.description || "No description"}</p>
          </AppLink>
        ))}
      </div>
      {runs.data?.next_cursor ? <Button variant="outline" onClick={() => setCursor(runs.data?.next_cursor ?? undefined)}>Next page</Button> : null}
    </PageFrame>
  );
}

export function RunDetailPage({ id }: { id: string }) {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const key = lightweightKeys.runs(workspace?.id ?? "");
  const run = useQuery({ queryKey: [...key, id], queryFn: () => lightweightApi.getRun(id), enabled: !!workspace && !!id });
  const comments = useQuery({ queryKey: [...key, id, "comments"], queryFn: () => lightweightApi.listComments(id), enabled: !!workspace && !!id });
  const tasks = useQuery({ queryKey: [...key, id, "tasks"], queryFn: () => lightweightApi.listTaskRuns(id), enabled: !!workspace && !!id });
  const update = useMutation({ mutationFn: (status: LightweightRunStatus) => lightweightApi.updateRun(id, { status }), onSuccess: () => void queryClient.invalidateQueries({ queryKey: key }) });
  const comment = useMutation({ mutationFn: (content: string) => lightweightApi.createComment(id, content), onSuccess: () => void queryClient.invalidateQueries({ queryKey: [...key, id, "comments"] }) });

  if (run.isPending) return <PageFrame title="Run"><p>Loading…</p></PageFrame>;
  if (!run.data) return <PageFrame title="Run"><ErrorState error={run.error} /></PageFrame>;

  return (
    <PageFrame
      title={`${run.data.identifier} · ${run.data.title}`}
      description={run.data.description || "No description"}
      action={
        <select value={run.data.status} onChange={(event) => update.mutate(event.target.value as LightweightRunStatus)} className="h-9 rounded-md border bg-background px-3 text-body">
          {LIGHTWEIGHT_RUN_STATUSES.map((status) => <option key={status}>{status}</option>)}
        </select>
      }
    >
      <ErrorState error={update.error} />
      <div className="grid gap-6 lg:grid-cols-2">
        <Panel>
          <h2 className="mb-4 font-semibold">Flat comments</h2>
          <div className="grid gap-3">
            {comments.data?.items.map((item) => (
              <article key={item.id} className="rounded-lg bg-muted/40 p-3">
                <div className="text-caption text-muted-foreground">{item.author_display} · {new Date(item.created_at).toLocaleString()}</div>
                <p className="mt-2 whitespace-pre-wrap text-body">{item.content}</p>
              </article>
            ))}
            {comments.data?.items.length === 0 ? <EmptyState>No comments.</EmptyState> : null}
          </div>
          <form className="mt-4 grid gap-2" onSubmit={onForm(async (form) => comment.mutateAsync(formValue(form, "content")))}>
            <Textarea name="content" required placeholder="Add an append-only comment" />
            <ErrorState error={comment.error} />
            <SubmitButton pending={comment.isPending}>Add comment</SubmitButton>
          </form>
        </Panel>

        <Panel>
          <h2 className="mb-4 font-semibold">Task runs</h2>
          <ErrorState error={tasks.error} />
          <div className="grid gap-3">
            {tasks.data?.items.map((task) => (
              <article key={task.id} className="rounded-lg border p-3 text-body">
                <div className="flex items-center justify-between gap-2"><strong>{task.agent.name}</strong><Badge variant="secondary">{task.status}</Badge></div>
                <div className="mt-2 text-muted-foreground">Runtime: {task.runtime.name} · attempt {task.attempt}/{task.max_attempts}{task.is_leader_task ? " · leader" : ""}</div>
                {task.failure_reason || task.error ? <p className="mt-2 text-destructive">{task.failure_reason ?? task.error}</p> : null}
                {task.status === "queued" || task.status === "running" ? <Button className="mt-3" size="sm" variant="outline" onClick={() => lightweightApi.cancelTask(task.id)}>Cancel</Button> : null}
              </article>
            ))}
            {tasks.data?.items.length === 0 ? <EmptyState>No task runs.</EmptyState> : null}
          </div>
        </Panel>
      </div>
    </PageFrame>
  );
}
