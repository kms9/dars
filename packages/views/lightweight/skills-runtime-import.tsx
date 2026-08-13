"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@dars/core/auth";
import {
  lightweightApi,
  lightweightKeys,
  resolveRuntimeLocalSkillImport,
  resolveRuntimeLocalSkills,
  type LightweightRuntimeLocalSkillImportConflict,
  type LightweightRuntimeLocalSkillSummary,
  type LightweightSkill,
} from "@dars/core/lightweight";
import { useCurrentWorkspace } from "@dars/core/paths";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import { ErrorState, Panel } from "./components";

type ConflictAction = "overwrite" | "rename" | "skip";

type ImportRow = {
  key: string;
  name: string;
  description?: string;
  status: "created" | "updated" | "conflict" | "skipped" | "failed" | "pending";
  conflict?: LightweightRuntimeLocalSkillImportConflict;
  error?: string;
  skill?: LightweightSkill;
};

function defaultRename(name: string): string {
  return `${name} copy`;
}

export function RuntimeLocalSkillImportPanel({
  onImported,
  onBack,
}: {
  onImported?: (skill: LightweightSkill) => void;
  onBack: () => void;
}) {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const userId = useAuthStore((state) => state.user?.id ?? null);
  const runtimes = useQuery({
    queryKey: lightweightKeys.runtimes(workspace?.id ?? ""),
    queryFn: lightweightApi.listRuntimes,
    enabled: !!workspace,
  });

  const ownedOnline = useMemo(
    () =>
      (runtimes.data ?? []).filter(
        (runtime) =>
          runtime.status === "online" &&
          (userId == null || runtime.owner_id === userId),
      ),
    [runtimes.data, userId],
  );
  const hasAnyOnline = (runtimes.data ?? []).some((runtime) => runtime.status === "online");
  const ownerBlocked = hasAnyOnline && ownedOnline.length === 0;

  const [runtimeId, setRuntimeId] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [results, setResults] = useState<ImportRow[]>([]);
  const [resolutions, setResolutions] = useState<Record<string, { action: ConflictAction; renameName: string }>>({});

  useEffect(() => {
    setRuntimeId((prev) => prev || ownedOnline[0]?.id || "");
  }, [ownedOnline]);

  useEffect(() => {
    setSelected(new Set());
    setResults([]);
    setResolutions({});
    setError(null);
  }, [runtimeId]);

  const discovery = useQuery({
    queryKey: [...lightweightKeys.runtimes(workspace?.id ?? ""), runtimeId, "local-skills"],
    queryFn: () => resolveRuntimeLocalSkills(runtimeId),
    enabled: !!runtimeId && !ownerBlocked,
    retry: false,
  });

  const skills = discovery.data?.skills ?? [];

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const refreshSkills = async () => {
    await queryClient.invalidateQueries({ queryKey: lightweightKeys.skills(workspace?.id ?? "") });
  };

  const runImportBatch = async (
    items: LightweightRuntimeLocalSkillSummary[],
    options?: { action?: "overwrite"; targetSkillId?: string; name?: string },
  ) => {
    const rows: ImportRow[] = items.map((skill) => ({
      key: skill.key,
      name: options?.name ?? skill.name,
      description: skill.description,
      status: "pending",
    }));
    setResults(rows);
    setBusy(true);
    setError(null);

    const nextRows = [...rows];
    for (let index = 0; index < items.length; index += 1) {
      const skill = items[index]!;
      try {
        const result = await resolveRuntimeLocalSkillImport(runtimeId, {
          skill_key: skill.key,
          name: options?.name ?? skill.name,
          description: skill.description,
          action: options?.action,
          target_skill_id: options?.targetSkillId,
          supports_conflict: true,
        });
        if (result.status === "conflict") {
          nextRows[index] = {
            ...nextRows[index]!,
            status: "conflict",
            conflict: result.conflict,
          };
          setResolutions((prev) => ({
            ...prev,
            [skill.key]: prev[skill.key] ?? {
              action: result.conflict?.can_overwrite ? "overwrite" : "rename",
              renameName: defaultRename(skill.name),
            },
          }));
        } else {
          nextRows[index] = {
            ...nextRows[index]!,
            status: result.status,
            skill: result.skill,
          };
          if (result.skill) onImported?.(result.skill);
        }
      } catch (err) {
        nextRows[index] = {
          ...nextRows[index]!,
          status: "failed",
          error: err instanceof Error ? err.message : "Import failed",
        };
      }
      setResults([...nextRows]);
    }

    await refreshSkills();
    setBusy(false);
  };

  const importSelected = async () => {
    const items = skills.filter((skill) => selected.has(skill.key));
    if (items.length === 0) return;
    await runImportBatch(items);
  };

  const resolveConflicts = async () => {
    const conflicts = results.filter((row) => row.status === "conflict");
    if (conflicts.length === 0) return;
    setBusy(true);
    const next = [...results];
    for (const row of conflicts) {
      const resolution = resolutions[row.key] ?? { action: "skip" as const, renameName: defaultRename(row.name) };
      const index = next.findIndex((item) => item.key === row.key);
      if (index < 0) continue;
      if (resolution.action === "skip") {
        next[index] = { ...next[index]!, status: "skipped" };
        setResults([...next]);
        continue;
      }
      const summary = skills.find((skill) => skill.key === row.key);
      if (!summary) continue;
      try {
        const result = await resolveRuntimeLocalSkillImport(runtimeId, {
          skill_key: summary.key,
          name: resolution.action === "rename" ? resolution.renameName.trim() : summary.name,
          description: summary.description,
          action: resolution.action === "overwrite" ? "overwrite" : undefined,
          target_skill_id: resolution.action === "overwrite" ? row.conflict?.existing_skill_id : undefined,
          supports_conflict: true,
        });
        if (result.status === "conflict") {
          next[index] = { ...next[index]!, status: "conflict", conflict: result.conflict };
        } else {
          next[index] = { ...next[index]!, status: result.status, skill: result.skill, conflict: undefined };
          if (result.skill) onImported?.(result.skill);
        }
      } catch (err) {
        next[index] = {
          ...next[index]!,
          status: "failed",
          error: err instanceof Error ? err.message : "Conflict resolution failed",
        };
      }
      setResults([...next]);
    }
    await refreshSkills();
    setBusy(false);
  };

  if (ownerBlocked) {
    return (
      <Panel>
        <h2 className="font-semibold">Copy from runtime</h2>
        <p className="mt-2 text-body text-muted-foreground" data-testid="runtime-import-owner-only">
          Owner-only import. Online runtimes in this workspace belong to other members, so local-skill import is unavailable.
        </p>
        <Button type="button" variant="outline" className="mt-4" onClick={onBack}>Back</Button>
      </Panel>
    );
  }

  if (ownedOnline.length === 0) {
    return (
      <Panel>
        <h2 className="font-semibold">Copy from runtime</h2>
        <p className="mt-2 text-body text-muted-foreground">
          No online runtimes you own. Start a Daemon runtime first, then rediscover local skills.
        </p>
        <Button type="button" variant="outline" className="mt-4" onClick={onBack}>Back</Button>
      </Panel>
    );
  }

  const conflicts = results.filter((row) => row.status === "conflict");

  return (
    <Panel>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold">Copy from runtime</h2>
          <p className="mt-1 text-body text-muted-foreground">Discover local skills and import selected ones into this workspace.</p>
        </div>
        <Button type="button" variant="outline" onClick={onBack} disabled={busy}>Back</Button>
      </div>

      <div className="mt-4 grid gap-2">
        <label className="text-body font-medium" htmlFor="runtime_id">Online runtime</label>
        <select
          id="runtime_id"
          value={runtimeId}
          onChange={(event) => setRuntimeId(event.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-body"
          disabled={busy}
        >
          {ownedOnline.map((runtime) => (
            <option key={runtime.id} value={runtime.id}>{runtime.name} · {runtime.provider}</option>
          ))}
        </select>
      </div>

      <ErrorState error={error ?? discovery.error} />
      {discovery.isPending ? <p className="mt-4 text-body text-muted-foreground">Discovering local skills…</p> : null}
      {discovery.data && !discovery.data.supported ? (
        <p className="mt-4 text-body text-muted-foreground">This runtime does not support local skill discovery.</p>
      ) : null}

      {skills.length > 0 ? (
        <div className="mt-4 grid gap-2">
          {skills.map((skill) => (
            <label key={skill.key} className="flex items-start gap-3 rounded-lg border p-3 text-body">
              <Checkbox
                checked={selected.has(skill.key)}
                onChange={() => toggle(skill.key)}
                disabled={busy}
              />
              <span className="min-w-0 flex-1">
                <span className="flex flex-wrap items-center gap-2">
                  <strong>{skill.name}</strong>
                  <Badge variant="outline">{skill.provider}</Badge>
                </span>
                {skill.description ? <span className="mt-1 block text-caption text-muted-foreground">{skill.description}</span> : null}
              </span>
            </label>
          ))}
        </div>
      ) : null}

      {skills.length === 0 && discovery.isSuccess && discovery.data.supported ? (
        <p className="mt-4 text-body text-muted-foreground">No local skills found on this runtime.</p>
      ) : null}

      <div className="mt-4 flex flex-wrap gap-2">
        <Button type="button" onClick={() => void importSelected()} disabled={busy || selected.size === 0}>
          {busy ? "Importing…" : `Import selected (${selected.size})`}
        </Button>
        <Button type="button" variant="outline" onClick={() => void discovery.refetch()} disabled={busy || !runtimeId}>
          Refresh discovery
        </Button>
      </div>

      {results.length > 0 ? (
        <div className="mt-5 grid gap-2">
          <h3 className="font-medium">Import results</h3>
          {results.map((row) => (
            <div key={row.key} className="rounded-lg border p-3 text-body">
              <div className="flex flex-wrap items-center gap-2">
                <strong>{row.name}</strong>
                <Badge variant="secondary">{row.status}</Badge>
              </div>
              {row.error ? <p className="mt-1 text-caption text-destructive">{row.error}</p> : null}
              {row.status === "conflict" ? (
                <div className="mt-3 grid gap-2 md:grid-cols-[160px_1fr]">
                  <select
                    className="h-9 rounded-md border bg-background px-3 text-body"
                    value={resolutions[row.key]?.action ?? "rename"}
                    onChange={(event) =>
                      setResolutions((prev) => ({
                        ...prev,
                        [row.key]: {
                          action: event.target.value as ConflictAction,
                          renameName: prev[row.key]?.renameName ?? defaultRename(row.name),
                        },
                      }))
                    }
                    disabled={busy}
                  >
                    {row.conflict?.can_overwrite ? <option value="overwrite">Overwrite</option> : null}
                    <option value="rename">Rename</option>
                    <option value="skip">Skip</option>
                  </select>
                  {(resolutions[row.key]?.action ?? "rename") === "rename" ? (
                    <Input
                      value={resolutions[row.key]?.renameName ?? defaultRename(row.name)}
                      onChange={(event) =>
                        setResolutions((prev) => ({
                          ...prev,
                          [row.key]: {
                            action: "rename",
                            renameName: event.target.value,
                          },
                        }))
                      }
                      disabled={busy}
                    />
                  ) : (
                    <p className="self-center text-caption text-muted-foreground">
                      Existing skill: {row.conflict?.existing_skill_id}
                    </p>
                  )}
                </div>
              ) : null}
            </div>
          ))}
          {conflicts.length > 0 ? (
            <Button type="button" onClick={() => void resolveConflicts()} disabled={busy}>
              Resolve conflicts ({conflicts.length})
            </Button>
          ) : null}
        </div>
      ) : null}
    </Panel>
  );
}
