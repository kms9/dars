"use client";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@dars/core/auth";
import {
  accessScopeLabel,
  ALL_ACCESS_SCOPES,
  buildAgentListRows,
  effectiveAccessScope,
  lightweightAgentSnapshotOptions,
  lightweightAgentSnapshotKeys,
  lightweightApi,
  lightweightKeys,
  paginateRows,
  rowMatchesFilters,
  runBatchOperation,
  runtimeDisplayLabel,
  scopeMatches,
  sortAgentRows,
  useAgentsViewStore,
  type AgentListRow,
  type AgentsScope,
} from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button, buttonVariants } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import { Skeleton } from "@dars/ui/components/ui/skeleton";
import { AppLink, useNavigation } from "../navigation";
import { EmptyState, ErrorState, PageFrame, Panel } from "./components";

function scopeLabel(scope: AgentsScope, counts: { mine: number; all: number; archived: number }): string {
  if (scope === "mine") return `My (${counts.mine})`;
  if (scope === "archived") return `Archived (${counts.archived})`;
  return `All (${counts.all})`;
}

function presenceLabel(row: AgentListRow): string {
  if (!row.presence) return "Unknown";
  if (row.presence.availability === "archived") return "Archived";
  if (row.presence.workload === "working") {
    return `Working · ${row.presence.runningCount}`;
  }
  if (row.presence.workload === "queued") {
    return `Queued · ${row.presence.queuedCount}`;
  }
  return row.presence.availability;
}

function AgentBatchBar({
  rows,
  onClear,
  onDone,
}: {
  rows: AgentListRow[];
  onClear: () => void;
  onDone: () => void;
}) {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const owned = rows.filter((row) => row.isOwnedByMe);
  const anyArchived = rows.some((row) => row.agent.archived_at);
  const anyActive = rows.some((row) => !row.agent.archived_at);

  const invalidate = () =>
    void queryClient.invalidateQueries({
      queryKey: lightweightAgentSnapshotKeys.detail(workspace?.id ?? ""),
    });

  const run = async (label: string, fn: (row: AgentListRow) => Promise<unknown>, targets: AgentListRow[]) => {
    setBusy(true);
    setMessage(null);
    const summary = await runBatchOperation(
      targets.map((row) => ({ id: row.agent.id, row })),
      async (item) => fn(item.row),
    );
    invalidate();
    onDone();
    setBusy(false);
    if (summary.failed > 0) {
      setMessage(`${label}: ${summary.succeeded} succeeded, ${summary.failed} failed`);
    } else {
      setMessage(`${label}: ${summary.succeeded} succeeded`);
    }
  };

  return (
    <Panel className="sticky bottom-4 z-10 flex flex-wrap items-center gap-3 bg-background/95 backdrop-blur">
      <span className="text-body font-medium">{rows.length} selected</span>
      {anyActive ? (
        <Button
          size="sm"
          variant="outline"
          disabled={busy || owned.length === 0}
          onClick={() =>
            void run(
              "Access update",
              async (row) => {
                await lightweightApi.updateAgent(row.agent.id, {
                  permission_mode: "public_to",
                  invocation_targets: [{ target_type: "workspace" }],
                });
              },
              owned,
            )
          }
        >
          Set workspace access
        </Button>
      ) : null}
      {anyActive ? (
        <Button
          size="sm"
          variant="outline"
          disabled={busy || rows.filter((row) => row.canManage).length === 0}
          onClick={() =>
            void run("Archive", (row) => lightweightApi.archiveAgent(row.agent.id), rows.filter((row) => row.canManage))
          }
        >
          Archive
        </Button>
      ) : null}
      {anyArchived ? (
        <Button
          size="sm"
          variant="outline"
          disabled={busy || rows.filter((row) => row.canManage).length === 0}
          onClick={() =>
            void run("Restore", (row) => lightweightApi.restoreAgent(row.agent.id), rows.filter((row) => row.canManage))
          }
        >
          Restore
        </Button>
      ) : null}
      <Button size="sm" variant="ghost" onClick={onClear} disabled={busy}>
        Clear
      </Button>
      {message ? <span className="text-caption text-muted-foreground">{message}</span> : null}
    </Panel>
  );
}

function AgentRowMenu({
  row,
  onDuplicate,
}: {
  row: AgentListRow;
  onDuplicate: (agentId: string) => void;
}) {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);
  const archived = !!row.agent.archived_at;
  const hasWork = (row.presence?.runningCount ?? 0) + (row.presence?.queuedCount ?? 0) > 0;

  const refresh = () =>
    void queryClient.invalidateQueries({
      queryKey: lightweightAgentSnapshotKeys.detail(workspace?.id ?? ""),
    });

  const run = async (fn: () => Promise<unknown>) => {
    setError(null);
    try {
      await fn();
      refresh();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Request failed");
    }
  };

  return (
    <div className="flex flex-col items-end gap-1">
      <div className="flex flex-wrap justify-end gap-1">
        {!archived ? (
          <Button size="sm" variant="ghost" onClick={() => onDuplicate(row.agent.id)}>
            Duplicate
          </Button>
        ) : null}
        {row.canManage && hasWork && !archived ? (
          <Button size="sm" variant="ghost" onClick={() => void run(() => lightweightApi.cancelAgentTasks(row.agent.id))}>
            Cancel work
          </Button>
        ) : null}
        {row.canManage && archived ? (
          <Button size="sm" variant="ghost" onClick={() => void run(() => lightweightApi.restoreAgent(row.agent.id))}>
            Restore
          </Button>
        ) : null}
        {row.canManage && !archived ? (
          <Button size="sm" variant="ghost" onClick={() => void run(() => lightweightApi.archiveAgent(row.agent.id))}>
            Archive
          </Button>
        ) : null}
      </div>
      {error ? <span className="text-caption text-destructive">{error}</span> : null}
    </div>
  );
}

export function AgentsPage() {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const user = useAuthStore((state) => state.user);
  const members = useQuery({
    queryKey: lightweightKeys.members(workspace?.id ?? ""),
    queryFn: () => lightweightApi.listMembers(workspace!.id),
    enabled: !!workspace,
  });
  const runtimes = useQuery({
    queryKey: lightweightKeys.runtimes(workspace?.id ?? ""),
    queryFn: lightweightApi.listRuntimes,
    enabled: !!workspace,
  });
  const snapshot = useQuery(lightweightAgentSnapshotOptions(workspace?.id ?? ""));
  const view = useAgentsViewStore();
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const memberRole =
    members.data?.find((member) => member.user_id === user?.id)?.role ?? null;

  const rows = useMemo(() => {
    if (!snapshot.data) return [];
    const built = buildAgentListRows({
      snapshot: snapshot.data,
      runtimes: runtimes.data ?? [],
      currentUserId: user?.id ?? null,
      memberRole,
    });
    return sortAgentRows(
      built.filter(
        (row) =>
          scopeMatches(row.agent, view.scope, user?.id ?? null) &&
          rowMatchesFilters(row, view.filters, query),
      ),
      view.sortField,
      view.sortDirection,
    );
  }, [
    memberRole,
    query,
    runtimes.data,
    snapshot.data,
    user?.id,
    view.filters,
    view.scope,
    view.sortDirection,
    view.sortField,
  ]);

  const pageInfo = paginateRows(rows, page, view.pageSize);
  const selectedRows = rows.filter((row) => selected.has(row.agent.id));
  const counts = snapshot.data?.scope_counts ?? { mine: 0, all: 0, archived: 0 };
  const isColumnVisible = (key: string) => !view.hiddenColumns.includes(key as never);

  return (
    <PageFrame
      title="Agents"
      description="Runtime-bound agents with invocation permissions, workload, and lifecycle controls."
      action={
        <AppLink href={paths.newAgent()} className={buttonVariants()}>
          Create agent
        </AppLink>
      }
    >
      <Panel>
        <div className="flex flex-wrap items-center gap-3">
          {(["mine", "all", "archived"] as AgentsScope[]).map((scope) => (
            <Button
              key={scope}
              size="sm"
              variant={view.scope === scope ? "default" : "outline"}
              onClick={() => {
                view.setScope(scope);
                setPage(1);
              }}
            >
              {scopeLabel(scope, counts)}
            </Button>
          ))}
          <Input
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setPage(1);
            }}
            placeholder="Search name or description"
            aria-label="Search agents"
            className="max-w-sm"
          />
          <span className="text-caption text-muted-foreground">
            {rows.length} result{rows.length === 1 ? "" : "s"}
          </span>
        </div>
        <div className="mt-4 flex flex-wrap gap-3">
          <label className="grid gap-1 text-caption">
            Availability
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.filters.availability[0] ?? ""}
              onChange={(event) => {
                view.clearFilters();
                if (event.target.value) view.toggleFilter("availability", event.target.value);
                setPage(1);
              }}
            >
              <option value="">Any</option>
              <option value="online">Online</option>
              <option value="unstable">Unstable</option>
              <option value="offline">Offline</option>
              <option value="archived">Archived</option>
            </select>
          </label>
          <label className="grid gap-1 text-caption">
            Runtime
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.filters.runtimes[0] ?? ""}
              onChange={(event) => {
                view.clearFilters();
                if (event.target.value) view.toggleFilter("runtimes", event.target.value);
                setPage(1);
              }}
            >
              <option value="">Any</option>
              {(snapshot.data?.filter_metadata.runtimes ?? []).map((runtime) => (
                <option key={runtime.id} value={runtime.id}>
                  {runtime.name} · {runtime.status}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-caption">
            Owner
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.filters.owners[0] ?? ""}
              onChange={(event) => {
                view.clearFilters();
                if (event.target.value) view.toggleFilter("owners", event.target.value);
                setPage(1);
              }}
            >
              <option value="">Any</option>
              {(snapshot.data?.filter_metadata.owners ?? []).map((owner) => (
                <option key={owner.id} value={owner.id}>
                  {owner.name}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-caption">
            Access
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.filters.access[0] ?? ""}
              onChange={(event) => {
                view.clearFilters();
                if (event.target.value) view.toggleFilter("access", event.target.value);
                setPage(1);
              }}
            >
              <option value="">Any</option>
              {ALL_ACCESS_SCOPES.map((scope) => (
                <option key={scope} value={scope}>
                  {accessScopeLabel(scope)}
                </option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-caption">
            Sort
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.sortField}
              onChange={(event) => view.setSortField(event.target.value as typeof view.sortField)}
            >
              <option value="lastActive">Last active</option>
              <option value="name">Name</option>
              <option value="runs">Runs</option>
              <option value="created">Updated</option>
            </select>
          </label>
          <Button size="sm" variant="ghost" onClick={() => view.clearFilters()}>
            Clear filters
          </Button>
        </div>
        <div className="mt-3 flex flex-wrap gap-3 text-caption text-muted-foreground">
          {(["status", "owner", "access", "runtime", "lastActive", "runs", "model", "created"] as const).map(
            (column) => (
              <label key={column} className="flex items-center gap-2">
                <Checkbox
                  checked={isColumnVisible(column)}
                  onChange={() => view.toggleColumn(column)}
                />
                {column}
              </label>
            ),
          )}
        </div>
      </Panel>

      <ErrorState error={snapshot.error ?? runtimes.error ?? members.error} />
      {snapshot.isPending ? (
        <div className="grid gap-2">
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
        </div>
      ) : null}
      {!snapshot.isPending && rows.length === 0 ? <EmptyState>No agents match this view.</EmptyState> : null}

      <div className="grid gap-2">
        {pageInfo.pageRows.map((row) => (
          <div key={row.agent.id} className="rounded-xl border bg-card p-4">
            <div className="flex flex-wrap items-start gap-4">
              <label className="flex items-start gap-2 pt-1">
                <Checkbox
                  checked={selected.has(row.agent.id)}
                  onChange={() => {
                    setSelected((prev) => {
                      const next = new Set(prev);
                      if (next.has(row.agent.id)) next.delete(row.agent.id);
                      else next.add(row.agent.id);
                      return next;
                    });
                  }}
                />
                <AppLink href={paths.agentDetail(row.agent.id)} className="min-w-0">
                  <div className="flex items-center gap-3">
                    {row.agent.avatar_url ? (
                      <img src={row.agent.avatar_url} alt="" className="size-10 rounded-full object-cover" />
                    ) : (
                      <div className="flex size-10 items-center justify-center rounded-full bg-muted text-caption font-semibold">
                        {row.agent.name.slice(0, 1).toUpperCase()}
                      </div>
                    )}
                    <div>
                      <div className="font-semibold">{row.agent.name}</div>
                      <div className="line-clamp-1 text-body text-muted-foreground">
                        {row.agent.description || "No description"}
                      </div>
                    </div>
                  </div>
                </AppLink>
              </label>
              <div className="ml-auto grid gap-2 text-caption text-muted-foreground md:grid-cols-2 lg:grid-cols-4">
                {isColumnVisible("status") ? <Badge variant="secondary">{presenceLabel(row)}</Badge> : null}
                {isColumnVisible("owner") ? <span>Owner: {row.ownerName}</span> : null}
                {isColumnVisible("access") ? (
                  <span>
                    Access:{" "}
                    {accessScopeLabel(
                      effectiveAccessScope(row.agent.permission_mode, row.detail?.invocation_targets),
                    )}
                  </span>
                ) : null}
                {isColumnVisible("runtime") ? (
                  <span>Runtime: {row.runtime ? runtimeDisplayLabel(row.runtime) : "Unbound"}</span>
                ) : null}
                {isColumnVisible("lastActive") ? (
                  <span>Last active: {row.lastActiveAt ? new Date(row.lastActiveAt).toLocaleDateString() : "—"}</span>
                ) : null}
                {isColumnVisible("runs") ? <span>Runs: {row.runCount}</span> : null}
                {isColumnVisible("model") ? <span>Model: {row.agent.model || "—"}</span> : null}
                {isColumnVisible("created") ? (
                  <span>Updated: {new Date(row.agent.updated_at).toLocaleDateString()}</span>
                ) : null}
              </div>
              <AgentRowMenu
                row={row}
                onDuplicate={(agentId) =>
                  navigation.push(`${paths.newAgentBlank()}?duplicate=${encodeURIComponent(agentId)}`)
                }
              />
            </div>
          </div>
        ))}
      </div>

      {rows.length > view.pageSize ? (
        <div className="flex items-center justify-between gap-3">
          <span className="text-caption text-muted-foreground">
            Page {pageInfo.page} of {pageInfo.totalPages}
          </span>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" disabled={pageInfo.page <= 1} onClick={() => setPage(pageInfo.page - 1)}>
              Previous
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={pageInfo.page >= pageInfo.totalPages}
              onClick={() => setPage(pageInfo.page + 1)}
            >
              Next
            </Button>
          </div>
        </div>
      ) : null}

      {selectedRows.length > 0 ? (
        <AgentBatchBar rows={selectedRows} onClear={() => setSelected(new Set())} onDone={() => setSelected(new Set())} />
      ) : null}
    </PageFrame>
  );
}
