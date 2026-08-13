"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  isRuntimeSkillDisabled,
  lightweightApi,
  lightweightKeys,
  resolveRuntimeLocalSkills,
  setRuntimeSkillEnabled,
  type AgentCapabilitiesSubview,
  type AgentDetailViewState,
  type LightweightAgent,
  type LightweightDisabledRuntimeSkill,
  type LightweightRuntime,
} from "@dars/core/lightweight";
import { useWorkspacePaths } from "@dars/core/paths";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Textarea } from "@dars/ui/components/ui/textarea";
import { AppLink } from "../navigation";
import { ErrorState, Panel, SubmitButton } from "./components";
import { AgentDetailToolsPanel } from "./agents-detail-tools";

export function AgentDetailCapabilitiesPanel({
  agent,
  runtime,
  workspaceId,
  canManage,
  canManageTools,
  state,
  onCapChange,
  instructions,
  onInstructionsChange,
  instructionsDirty,
  onSaveInstructions,
  instructionsError,
  instructionsPending,
}: {
  agent: LightweightAgent;
  runtime: LightweightRuntime | null;
  workspaceId: string;
  canManage: boolean;
  canManageTools: boolean;
  state: AgentDetailViewState;
  onCapChange: (cap: AgentCapabilitiesSubview) => void;
  instructions: string;
  onInstructionsChange: (value: string) => void;
  instructionsDirty: boolean;
  onSaveInstructions: () => Promise<void>;
  instructionsError: unknown;
  instructionsPending: boolean;
}) {
  const paths = useWorkspacePaths();
  const queryClient = useQueryClient();
  const skills = useQuery({
    queryKey: lightweightKeys.skills(workspaceId),
    queryFn: lightweightApi.listSkills,
    enabled: !!workspaceId,
  });
  const [localSkillsError, setLocalSkillsError] = useState<string | null>(null);
  const [localSkills, setLocalSkills] = useState<Awaited<ReturnType<typeof resolveRuntimeLocalSkills>> | null>(null);
  const [discovering, setDiscovering] = useState(false);
  const [disabledRuntimeSkills, setDisabledRuntimeSkills] = useState<LightweightDisabledRuntimeSkill[]>(
    () => [...agent.disabled_runtime_skills],
  );

  useEffect(() => {
    setDisabledRuntimeSkills([...agent.disabled_runtime_skills]);
  }, [agent.disabled_runtime_skills, agent.updated_at]);

  const binding = useMutation({
    mutationFn: (values: Array<{ skill_id: string; enabled: boolean }>) =>
      lightweightApi.setAgentSkills(agent.id, values),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: lightweightKeys.agents(workspaceId),
      }),
  });

  const runtimeSkills = useMutation({
    mutationFn: (next: LightweightDisabledRuntimeSkill[]) =>
      lightweightApi.updateAgent(agent.id, { disabled_runtime_skills: next }),
    onSuccess: () =>
      void queryClient.invalidateQueries({
        queryKey: lightweightKeys.agents(workspaceId),
      }),
  });

  const discover = async () => {
    if (!runtime?.id) return;
    setDiscovering(true);
    setLocalSkillsError(null);
    try {
      const result = await resolveRuntimeLocalSkills(runtime.id);
      setLocalSkills(result);
    } catch (error) {
      setLocalSkills(null);
      setLocalSkillsError(error instanceof Error ? error.message : "Discovery failed");
    } finally {
      setDiscovering(false);
    }
  };

  const runtimeStatus = useMemo(() => {
    if (!runtime) return "No runtime bound";
    if (runtime.status !== "online") return "Runtime offline";
    if (localSkillsError) return "Discovery failed";
    if (localSkills && !localSkills.supported) return "Unsupported on this runtime";
    return null;
  }, [localSkills, localSkillsError, runtime]);

  return (
    <div className="grid gap-5">
      <div className="flex flex-wrap gap-2 border-b pb-3">
        {(["instructions", "skills", ...(canManageTools ? ["tools" as const] : [])] as const).map((cap) => (
          <Button
            key={cap}
            size="sm"
            variant={state.cap === cap ? "secondary" : "ghost"}
            onClick={() => onCapChange(cap)}
          >
            {cap === "instructions" ? "Instructions" : cap === "skills" ? "Skills" : "Tools"}
          </Button>
        ))}
      </div>

      {state.cap === "instructions" ? (
        <Panel>
          <p className="text-body text-muted-foreground">
            Saved instructions are used by the Daemon on the next task claim.
          </p>
          <form
            className="mt-3 grid gap-3"
            onSubmit={(event) => {
              event.preventDefault();
              if (!canManage || !instructionsDirty) return;
              void onSaveInstructions();
            }}
          >
            <Textarea
              rows={12}
              value={instructions}
              readOnly={!canManage}
              onChange={(event) => onInstructionsChange(event.target.value)}
            />
            {instructionsDirty ? <p className="text-caption text-amber-700">Unsaved changes</p> : null}
            <ErrorState error={instructionsError} />
            {canManage ? (
              <SubmitButton pending={instructionsPending} disabled={!instructionsDirty}>
                Save instructions
              </SubmitButton>
            ) : null}
          </form>
        </Panel>
      ) : state.cap === "tools" ? (
        canManageTools ? <AgentDetailToolsPanel agentId={agent.id} workspaceId={workspaceId} /> : <ErrorState error={new Error("Workspace owner or admin permission is required")} />
      ) : (
        <div className="grid gap-5">
          <Panel>
            <h3 className="font-semibold">Workspace skills</h3>
            <p className="mt-1 text-body text-muted-foreground">Bind workspace skills to this agent.</p>
            <form
              className="mt-4 grid gap-3"
              onSubmit={(event) => {
                event.preventDefault();
                if (!canManage) return;
                const form = event.currentTarget;
                const bound = new Set(
                  Array.from(form.querySelectorAll<HTMLInputElement>('input[name="skill_id"]:checked')).map(
                    (input) => input.value,
                  ),
                );
                const enabled = new Set(
                  Array.from(form.querySelectorAll<HTMLInputElement>('input[name="skill_enabled"]:checked')).map(
                    (input) => input.value,
                  ),
                );
                void binding.mutateAsync(
                  (skills.data ?? [])
                    .filter((skill) => bound.has(skill.id))
                    .map((skill) => ({ skill_id: skill.id, enabled: enabled.has(skill.id) })),
                );
              }}
            >
              {(skills.data ?? []).map((skill) => {
                const current = agent.skills.find((item) => item.id === skill.id);
                const bound = Boolean(current);
                return (
                  <div key={skill.id} className="flex flex-wrap items-center gap-4 rounded-lg border px-3 py-2 text-body">
                    <label className="flex items-center gap-2">
                      <Checkbox name="skill_id" value={skill.id} defaultChecked={bound} disabled={!canManage} />
                      <AppLink href={paths.skillDetail(skill.id)}>{skill.name}</AppLink>
                    </label>
                    <label className="flex items-center gap-2 text-caption text-muted-foreground">
                      <Checkbox
                        name="skill_enabled"
                        value={skill.id}
                        defaultChecked={current?.enabled !== false && bound}
                        disabled={!canManage}
                      />
                      Enabled
                    </label>
                  </div>
                );
              })}
              {(skills.data?.length ?? 0) === 0 ? (
                <p className="text-body text-muted-foreground">No workspace skills yet.</p>
              ) : null}
              <ErrorState error={binding.error} />
              {canManage ? <SubmitButton pending={binding.isPending}>Save workspace bindings</SubmitButton> : null}
            </form>
          </Panel>

          <Panel>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="font-semibold">Runtime local skills</h3>
                <p className="text-body text-muted-foreground">Discover skills on the bound runtime and enable or disable them per agent.</p>
              </div>
              <Button variant="outline" size="sm" disabled={!runtime || discovering} onClick={() => void discover()}>
                {discovering ? "Discovering…" : "Discover"}
              </Button>
            </div>
            {runtimeStatus ? <p className="mt-3 text-body text-muted-foreground">{runtimeStatus}</p> : null}
            {localSkillsError ? <p className="mt-2 text-body text-destructive">{localSkillsError}</p> : null}
            {localSkills?.supported === false ? (
              <p className="mt-3 text-body text-muted-foreground">This runtime does not support local skill discovery.</p>
            ) : null}
            {localSkills?.skills.length === 0 ? (
              <p className="mt-3 text-body text-muted-foreground">No local skills were found.</p>
            ) : null}
            <div className="mt-4 grid gap-2">
              {(localSkills?.skills ?? []).map((skill) => {
                const enabled = runtime
                  ? !isRuntimeSkillDisabled(disabledRuntimeSkills, runtime.id, skill)
                  : true;
                return (
                  <div key={skill.key} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2">
                    <div>
                      <div className="font-medium">{skill.name}</div>
                      <div className="text-caption text-muted-foreground">{skill.description || skill.key}</div>
                      <Badge variant="outline" className="mt-1">{skill.provider}</Badge>
                    </div>
                    <Checkbox
                      checked={enabled}
                      disabled={!canManage || !runtime}
                      onChange={(event) => {
                        if (!runtime) return;
                        const next = setRuntimeSkillEnabled(
                          disabledRuntimeSkills,
                          runtime.id,
                          skill,
                          event.target.checked,
                        );
                        setDisabledRuntimeSkills(next);
                        void runtimeSkills.mutateAsync(next);
                      }}
                    />
                  </div>
                );
              })}
            </div>
            <ErrorState error={runtimeSkills.error} />
          </Panel>
        </div>
      )}
    </div>
  );
}
