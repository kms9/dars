"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  agentToolBundleOptions,
  lightweightApi,
  toolDefinitionsOptions,
  toolKeys,
  toolSourcesOptions,
} from "@dars/core/lightweight";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import { EmptyState, ErrorState, Panel } from "./components";

const exportedToolNamePattern = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/;

export function AgentDetailToolsPanel({ agentId, workspaceId }: { agentId: string; workspaceId: string }) {
  const queryClient = useQueryClient();
  const sources = useQuery(toolSourcesOptions(workspaceId));
  const definitionQueries = useQueries({
    queries: (sources.data ?? []).map((source) => ({
      ...toolDefinitionsOptions(workspaceId, source.id),
      enabled: !!source.currentRevision,
    })),
  });
  const current = useQuery(agentToolBundleOptions(workspaceId, agentId));
  const [selected, setSelected] = useState<Map<string, string>>(new Map());
  const [feedback, setFeedback] = useState<string | null>(null);

  useEffect(() => {
    setSelected(new Map(current.data?.bundle?.items.map((item) => [item.toolDefinitionId, item.exportedName]) ?? []));
  }, [current.data?.bundle?.id, current.data?.bundle?.items]);

  const groups = useMemo(() => (sources.data ?? []).map((source, index) => ({
    source,
    tools: definitionQueries[index]?.data ?? [],
    error: definitionQueries[index]?.error,
    pending: definitionQueries[index]?.isPending === true,
  })), [definitionQueries, sources.data]);

  const aliasErrors = useMemo(() => {
    const errors = new Map<string, string>();
    const owners = new Map<string, string>();
    for (const [definitionId, rawName] of selected) {
      const name = rawName.trim();
      if (!exportedToolNamePattern.test(name)) {
        errors.set(definitionId, "Use 1-128 letters, numbers, dots, underscores, or hyphens; start with a letter or number.");
        continue;
      }
      const existing = owners.get(name);
      if (existing) {
        errors.set(existing, "Agent call names must be unique in this Bundle.");
        errors.set(definitionId, "Agent call names must be unique in this Bundle.");
      } else {
        owners.set(name, definitionId);
      }
    }
    return errors;
  }, [selected]);

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: toolKeys.agentBundle(workspaceId, agentId) });
    void queryClient.invalidateQueries({ queryKey: toolKeys.sources(workspaceId) });
  };
  const publish = useMutation({
    mutationFn: () => lightweightApi.publishAgentToolBundle(agentId, [...selected.entries()]
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([toolDefinitionId, exportedName]) => ({ toolDefinitionId, exportedName: exportedName.trim() }))),
    onSuccess: (bundle) => {
      const previousId = current.data?.bundle?.id;
      setFeedback(bundle?.id === previousId ? `No changes. Bundle ${bundle?.id ?? ""} remains current.` : `Published ${bundle?.id ?? "a new Bundle"}. New tasks use this snapshot.`);
      refresh();
    },
  });
  const clear = useMutation({
    mutationFn: () => lightweightApi.clearAgentToolBundle(agentId),
    onSuccess: () => {
      setSelected(new Map());
      setFeedback("Current tools cleared. Existing tasks keep their pinned Bundle; new tasks receive no Gateway Bundle.");
      refresh();
    },
  });
  const revoke = useMutation({
    mutationFn: () => {
      const bundleId = current.data?.bundle?.id;
      if (!bundleId) throw new Error("No current Bundle to revoke");
      return lightweightApi.revokeToolBundle(bundleId);
    },
    onSuccess: () => {
      setFeedback("Bundle revoked. Active tasks can no longer discover or call its tools.");
      refresh();
    },
  });

  const bundle = current.data?.bundle ?? null;
  const catalogIds = new Set(groups.flatMap((group) => group.tools.map((tool) => tool.id)));
  const staleItems = bundle?.items.filter((item) => {
    const source = sources.data?.find((candidate) => candidate.id === item.sourceId);
    return source?.currentRevision !== item.sourceRevisionId || !catalogIds.has(item.toolDefinitionId);
  }) ?? [];

  return (
    <div className="grid gap-5">
      <Panel>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h3 className="font-semibold">Current ToolBundle</h3>
            <p className="mt-1 text-body text-muted-foreground">
              {bundle ? `${bundle.id} · ${bundle.items.length} tools · ${bundle.status}` : "No Server ToolBundle is assigned."}
            </p>
          </div>
          {bundle ? <Badge variant={bundle.status === "active" ? "secondary" : "destructive"}>{bundle.status}</Badge> : null}
        </div>
        <p className="mt-3 text-caption text-muted-foreground">
          Publishing changes creates a new immutable ID. Existing tasks keep their pinned Bundle; only new tasks use the new head.
        </p>
        {staleItems.length > 0 ? (
          <p className="mt-3 text-body text-amber-700">
            {staleItems.length} selected tool(s) have a newer Source revision or are no longer in the current catalog. Review and publish explicitly to update.
          </p>
        ) : null}
        {feedback ? <p className="mt-3 rounded-md bg-muted px-3 py-2 text-body">{feedback}</p> : null}
        <ErrorState error={current.error ?? publish.error ?? clear.error ?? revoke.error} />
      </Panel>

      <Panel>
        <h3 className="font-semibold">Select tools</h3>
        <p className="mt-1 text-body text-muted-foreground">Choose complete Sources or individual tools. Different Agents publish independent snapshots.</p>
        <ErrorState error={sources.error} />
        <div className="mt-4 grid gap-4">
          {groups.map(({ source, tools, error, pending }) => {
            const currentIds = tools.map((tool) => tool.id);
            const allSelected = currentIds.length > 0 && currentIds.every((id) => selected.has(id));
            return (
              <section key={source.id} className="rounded-lg border p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <label className="flex items-start gap-2">
                    <Checkbox
                      checked={allSelected}
                      disabled={!source.enabled || currentIds.length === 0}
                      onChange={(event) => {
                        setSelected((previous) => {
                          const next = new Map(previous);
                          for (const tool of tools) {
                            if (event.target.checked) {
                              if (!next.has(tool.id)) next.set(tool.id, tool.publicName);
                            } else {
                              next.delete(tool.id);
                            }
                          }
                          return next;
                        });
                      }}
                    />
                    <span>
                      <span className="font-medium">{source.name}</span>
                      <span className="ml-2 text-caption text-muted-foreground">revision {source.revision?.revision ?? "—"}</span>
                    </span>
                  </label>
                  <div className="flex gap-2">
                    <Badge variant="outline">{source.kind}</Badge>
                    <Badge variant={source.enabled ? "secondary" : "destructive"}>{source.enabled ? "enabled" : "disabled"}</Badge>
                  </div>
                </div>
                {pending ? <p className="mt-3 text-body text-muted-foreground">Loading tools…</p> : null}
                <ErrorState error={error} />
                <div className="mt-3 grid gap-2">
                  {tools.map((tool) => {
                    const isSelected = selected.has(tool.id);
                    const aliasError = aliasErrors.get(tool.id);
                    return (
                    <div key={tool.id} className="flex items-start gap-3 rounded-md bg-muted/30 px-3 py-2">
                      <Checkbox
                        aria-label={`Select ${tool.publicName}`}
                        checked={isSelected}
                        disabled={!source.enabled || !tool.enabled}
                        onChange={(event) => {
                          setSelected((previous) => {
                            const next = new Map(previous);
                            if (event.target.checked) next.set(tool.id, previous.get(tool.id) ?? tool.publicName);
                            else next.delete(tool.id);
                            return next;
                          });
                        }}
                      />
                      <div className="min-w-0 flex-1">
                        <p className="font-medium">{tool.description || tool.upstreamName}</p>
                        <p className="text-caption text-muted-foreground">Canonical MCP name: {tool.publicName}</p>
                        <p className="text-caption text-muted-foreground">Upstream operation: {tool.upstreamName}</p>
                        {isSelected ? (
                          <div className="mt-2 grid gap-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <Input
                                aria-label={`Agent call name for ${tool.publicName}`}
                                value={selected.get(tool.id) ?? ""}
                                onChange={(event) => {
                                  const value = event.target.value;
                                  setSelected((previous) => new Map(previous).set(tool.id, value));
                                }}
                              />
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={() => setSelected((previous) => new Map(previous).set(tool.id, tool.publicName))}
                              >Reset to canonical</Button>
                            </div>
                            <p className="text-caption text-muted-foreground">This is the name the Agent uses in MCP tools/list and tools/call.</p>
                            {aliasError ? <p className="text-caption text-destructive">{aliasError}</p> : null}
                          </div>
                        ) : null}
                      </div>
                    </div>
                    );
                  })}
                </div>
              </section>
            );
          })}
          {!sources.isPending && groups.length === 0 ? <EmptyState>No Tool Sources are available. Import one from Workspace Tools.</EmptyState> : null}
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button disabled={selected.size === 0 || aliasErrors.size > 0 || publish.isPending} onClick={() => publish.mutate()}>
            {publish.isPending ? "Publishing…" : `Publish ${selected.size} tools`}
          </Button>
          <Button variant="outline" disabled={!bundle || clear.isPending} onClick={() => clear.mutate()}>Clear current tools</Button>
          <Button
            variant="destructive"
            disabled={!bundle || bundle.status !== "active" || revoke.isPending}
            onClick={() => {
              if (globalThis.confirm("Revoke this Bundle for active tasks? This is different from clearing the Agent head.")) revoke.mutate();
            }}
          >Revoke Bundle</Button>
        </div>
      </Panel>
    </div>
  );
}
