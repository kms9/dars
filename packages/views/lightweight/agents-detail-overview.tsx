"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ACTIVE_TASK_STATUSES,
  accessScopeLabel,
  deriveAgentPresenceDetail,
  effectiveAccessScope,
  lightweightApi,
  lightweightKeys,
  runtimeDisplayLabel,
  type AgentDetailViewState,
  type LightweightAgent,
  type LightweightAgentTask,
  type LightweightRuntime,
  type LightweightAgentSnapshotTask,
} from "@dars/core/lightweight";
import { useWorkspacePaths } from "@dars/core/paths";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { AppLink } from "../navigation";
import { EmptyState, ErrorState, Panel } from "./components";

function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return "—";
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  return `${(ms / 60_000).toFixed(1)} min`;
}

function TaskRow({
  task,
  paths,
  onSelect,
  selected,
}: {
  task: {
    id: string;
    status: string;
    created_at: string;
    failure_reason?: string | null;
    trigger_summary?: string | null;
    issue_id?: string | null;
    chat_session_id?: string | null;
  };
  paths: ReturnType<typeof useWorkspacePaths>;
  onSelect: (taskId: string) => void;
  selected: boolean;
}) {
  const active = ACTIVE_TASK_STATUSES.has(task.status);
  return (
    <button
      type="button"
      className={`w-full rounded-lg border px-3 py-2 text-left text-body transition-colors hover:bg-muted/40 ${selected ? "border-primary" : ""}`}
      onClick={() => onSelect(task.id)}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-medium">{task.status}</span>
        <span className="text-caption text-muted-foreground">{new Date(task.created_at).toLocaleString()}</span>
      </div>
      {task.trigger_summary ? (
        <p className="mt-1 text-caption text-muted-foreground">Initiator: {task.trigger_summary}</p>
      ) : null}
      {task.failure_reason ? (
        <p className="mt-1 text-caption text-destructive">{task.failure_reason}</p>
      ) : null}
      <div className="mt-2 flex flex-wrap gap-2">
        {task.issue_id ? (
          <AppLink href={paths.issueDetail(task.issue_id)} className="text-caption text-primary underline">
            Issue
          </AppLink>
        ) : null}
        {task.chat_session_id ? (
          <AppLink href={`${paths.chat()}?session=${task.chat_session_id}`} className="text-caption text-primary underline">
            Chat
          </AppLink>
        ) : null}
        {active ? <Badge variant="secondary">Active</Badge> : null}
      </div>
    </button>
  );
}

export function AgentDetailOverviewPanel({
  agent,
  runtime,
  ownerName,
  tasksPage,
  snapshotTasks,
  canManage,
  onCancelWork,
  onDirectMessage,
  onAssignWork,
  actionError,
}: {
  agent: LightweightAgent;
  runtime: LightweightRuntime | null;
  ownerName: string;
  tasksPage: { items: LightweightAgentTask[]; summary_30d: { run_count: number; success_rate: number; avg_duration_ms: number; fail_count: number } } | undefined;
  snapshotTasks: Array<{
    id: string;
    status: string;
    created_at: string;
    failure_reason?: string | null;
    trigger_summary?: string | null;
    issue_id?: string | null;
    chat_session_id?: string | null;
  }>;
  canManage: boolean;
  onCancelWork: () => void;
  onDirectMessage: () => void;
  onAssignWork: () => void;
  actionError: unknown;
}) {
  const paths = useWorkspacePaths();
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const presence = deriveAgentPresenceDetail({
    runtime,
    tasks: snapshotTasks as LightweightAgentSnapshotTask[],
    archived: !!agent.archived_at,
    maxConcurrentTasks: agent.max_concurrent_tasks,
  });
  const currentWork = snapshotTasks.filter((task) => ACTIVE_TASK_STATUSES.has(task.status));
  const recentWork = tasksPage?.items ?? [];
  const summary = tasksPage?.summary_30d;
  const transcript = useQuery({
    queryKey: lightweightKeys.taskMessages(agent.workspace_id, selectedTaskId ?? ""),
    queryFn: () => lightweightApi.listTaskMessages(selectedTaskId!),
    enabled: !!selectedTaskId,
  });
  const access = effectiveAccessScope(agent.permission_mode, agent.invocation_targets);

  return (
    <div className="grid gap-5">
      <Panel>
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div className="flex items-start gap-4">
            {agent.avatar_url ? (
              <img src={agent.avatar_url} alt="" className="size-14 rounded-full object-cover" />
            ) : (
              <div className="flex size-14 items-center justify-center rounded-full bg-muted text-title-sm font-semibold">
                {agent.name.slice(0, 1).toUpperCase()}
              </div>
            )}
            <div>
              <h2 className="text-title-sm font-semibold">{agent.name}</h2>
              <p className="text-body text-muted-foreground">{agent.description || "No description"}</p>
              <div className="mt-2 flex flex-wrap gap-2 text-caption text-muted-foreground">
                <Badge variant="outline">{presence.availability}</Badge>
                <Badge variant="outline">{presence.workload}</Badge>
                <span>{runtime ? runtimeDisplayLabel(runtime) : "No runtime"}</span>
                <span>{agent.model || "No model"}</span>
                <span>{accessScopeLabel(access)}</span>
              </div>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={onDirectMessage}>Direct message</Button>
            <Button variant="outline" onClick={onAssignWork}>Assign work</Button>
            {canManage ? (
              <Button variant="outline" onClick={onCancelWork}>Cancel work</Button>
            ) : null}
          </div>
        </div>
        <ErrorState error={actionError} />
      </Panel>

      <div className="grid gap-5 lg:grid-cols-2">
        <Panel>
          <h3 className="font-semibold">Current work</h3>
          {currentWork.length === 0 ? (
            <p className="mt-2 text-body text-muted-foreground">No active tasks.</p>
          ) : (
            <div className="mt-3 grid gap-2">
              {currentWork.map((task) => (
                <TaskRow key={task.id} task={task} paths={paths} onSelect={setSelectedTaskId} selected={selectedTaskId === task.id} />
              ))}
            </div>
          )}
        </Panel>

        <Panel>
          <h3 className="font-semibold">30-day summary</h3>
          <dl className="mt-3 grid grid-cols-2 gap-3 text-body">
            <div><dt className="text-caption text-muted-foreground">Runs</dt><dd className="font-medium">{summary?.run_count ?? 0}</dd></div>
            <div><dt className="text-caption text-muted-foreground">Success rate</dt><dd className="font-medium">{summary ? `${Math.round(summary.success_rate * 100)}%` : "—"}</dd></div>
            <div><dt className="text-caption text-muted-foreground">Avg duration</dt><dd className="font-medium">{formatDuration(summary?.avg_duration_ms ?? 0)}</dd></div>
            <div><dt className="text-caption text-muted-foreground">Failures</dt><dd className="font-medium">{summary?.fail_count ?? 0}</dd></div>
          </dl>
          <div className="mt-4 grid gap-2 text-body text-muted-foreground">
            <div>Owner: {ownerName}</div>
            <div>Concurrency: {agent.max_concurrent_tasks}</div>
            <div>Skills: {agent.skills.filter((skill) => skill.enabled).length} enabled</div>
            <div>Updated: {new Date(agent.updated_at).toLocaleString()}</div>
          </div>
        </Panel>
      </div>

      <Panel>
        <h3 className="font-semibold">Recent work</h3>
        {recentWork.length === 0 ? (
          <EmptyState>No recent tasks in the last 30 days.</EmptyState>
        ) : (
          <div className="mt-3 grid gap-2">
            {recentWork.map((task) => (
              <TaskRow key={task.id} task={task} paths={paths} onSelect={setSelectedTaskId} selected={selectedTaskId === task.id} />
            ))}
          </div>
        )}
      </Panel>

      {selectedTaskId ? (
        <Panel>
          <div className="flex items-center justify-between gap-3">
            <h3 className="font-semibold">Transcript</h3>
            <Button size="sm" variant="ghost" onClick={() => setSelectedTaskId(null)}>Close</Button>
          </div>
          <ErrorState error={transcript.error} />
          {transcript.isPending ? <p className="mt-2 text-body text-muted-foreground">Loading messages…</p> : null}
          <div className="mt-3 grid gap-2">
            {(transcript.data?.items ?? []).map((message) => (
              <div key={message.id} className="rounded-md border px-3 py-2 text-body">
                <div className="text-caption text-muted-foreground">{message.type}{message.tool ? ` · ${message.tool}` : ""}</div>
                <p className="mt-1 whitespace-pre-wrap">{message.content || message.output || "—"}</p>
              </div>
            ))}
          </div>
        </Panel>
      ) : null}
    </div>
  );
}

export function AgentDetailWorkPanel({
  agentId,
  workspaceId,
  state,
  onStateChange,
}: {
  agentId: string;
  workspaceId: string;
  state: AgentDetailViewState;
  onStateChange: (patch: Partial<AgentDetailViewState>) => void;
}) {
  const paths = useWorkspacePaths();
  const runs = useQuery({
    queryKey: [
      ...lightweightKeys.runs(workspaceId),
      "agent-work",
      agentId,
      state.workScope,
      state.workStatus,
      state.workCursor,
    ],
    queryFn: () =>
      state.workScope === "created"
        ? lightweightApi.listRuns({
            cursor: state.workCursor ?? undefined,
            statuses: state.workStatus ? [state.workStatus] : undefined,
            creatorType: "agent",
            creatorId: agentId,
          })
        : lightweightApi.listRuns({
            cursor: state.workCursor ?? undefined,
            statuses: state.workStatus ? [state.workStatus] : undefined,
            assigneeType: "agent",
            assigneeId: agentId,
          }),
    enabled: !!agentId,
  });

  const filtered = useMemo(() => {
    const needle = state.workQuery.trim().toLowerCase();
    const items = runs.data?.items ?? [];
    if (!needle) return items;
    return items.filter(
      (run) =>
        run.title.toLowerCase().includes(needle) ||
        run.description.toLowerCase().includes(needle) ||
        run.identifier.toLowerCase().includes(needle),
    );
  }, [runs.data?.items, state.workQuery]);

  return (
    <Panel>
      <div className="flex flex-wrap items-end gap-3">
        <label className="grid gap-1 text-body">
          <span className="font-medium">Scope</span>
          <select
            className="h-9 rounded-md border bg-background px-3"
            value={state.workScope}
            onChange={(event) =>
              onStateChange({
                workScope: event.target.value as AgentDetailViewState["workScope"],
                workCursor: null,
              })
            }
          >
            <option value="assigned">Assigned</option>
            <option value="created">Created</option>
          </select>
        </label>
        <label className="grid gap-1 text-body">
          <span className="font-medium">Status</span>
          <select
            className="h-9 rounded-md border bg-background px-3"
            value={state.workStatus}
            onChange={(event) => onStateChange({ workStatus: event.target.value, workCursor: null })}
          >
            <option value="">All</option>
            <option value="backlog">backlog</option>
            <option value="todo">todo</option>
            <option value="in_progress">in_progress</option>
            <option value="in_review">in_review</option>
            <option value="done">done</option>
            <option value="blocked">blocked</option>
            <option value="cancelled">cancelled</option>
          </select>
        </label>
        <label className="grid min-w-[12rem] flex-1 gap-1 text-body">
          <span className="font-medium">Search</span>
          <input
            className="h-9 rounded-md border bg-background px-3"
            value={state.workQuery}
            onChange={(event) => onStateChange({ workQuery: event.target.value, workCursor: null })}
            placeholder="Filter loaded results"
          />
        </label>
      </div>

      <ErrorState error={runs.error} />
      {runs.isPending ? <p className="mt-4 text-body text-muted-foreground">Loading work…</p> : null}
      {filtered.length === 0 && !runs.isPending ? (
        <div className="mt-4"><EmptyState>No matching issues.</EmptyState></div>
      ) : null}
      <div className="mt-4 grid gap-2">
        {filtered.map((run) => (
          <AppLink key={run.id} href={paths.issueDetail(run.id)} className="rounded-lg border px-3 py-2 hover:bg-muted/40">
            <div className="flex items-center justify-between gap-2">
              <div>
                <div className="text-caption text-muted-foreground">{run.identifier}</div>
                <div className="font-medium">{run.title}</div>
              </div>
              <Badge variant="secondary">{run.status}</Badge>
            </div>
          </AppLink>
        ))}
      </div>
      {runs.data?.next_cursor ? (
        <Button
          className="mt-4"
          variant="outline"
          onClick={() => onStateChange({ workCursor: runs.data?.next_cursor ?? null })}
        >
          Next page
        </Button>
      ) : null}
    </Panel>
  );
}
