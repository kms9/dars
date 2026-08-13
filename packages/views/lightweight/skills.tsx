"use client";

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@dars/core/api";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import {
  lightweightApi,
  lightweightKeys,
  type LightweightAgent,
  type LightweightSkill,
} from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { AppLink, useNavigation } from "../navigation";
import { EmptyState, ErrorState, Field, PageFrame, Panel, SubmitButton, TextField, formValue, onForm } from "./components";
import { SkillsCreatePanel } from "./skills-create";
import { SkillFilesEditor } from "./skills-files";

function skillOriginLabel(skill: LightweightSkill): string {
  const origin = skill.config?.origin;
  if (!origin || typeof origin !== "object") return "manual";
  const type = (origin as { type?: unknown }).type;
  if (typeof type === "string" && type) return type;
  const source = (origin as { source?: unknown }).source;
  if (typeof source === "string" && source) return source;
  return "imported";
}

function matchesSkillQuery(skill: LightweightSkill, query: string): boolean {
  const needle = query.trim().toLowerCase();
  if (!needle) return true;
  return [skill.name, skill.description, skillOriginLabel(skill)].some((value) =>
    value.toLowerCase().includes(needle),
  );
}

export function SkillsPage() {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const key = lightweightKeys.skills(workspace?.id ?? "");
  const skills = useQuery({ queryKey: key, queryFn: lightweightApi.listSkills, enabled: !!workspace });
  const [query, setQuery] = useState("");
  const filtered = useMemo(
    () => (skills.data ?? []).filter((skill) => matchesSkillQuery(skill, query)),
    [skills.data, query],
  );

  return (
    <PageFrame title="Skills" description="Workspace-local instructions and files that can be bound to agents.">
      <SkillsCreatePanel />
      <Panel>
        <div className="flex flex-wrap items-center gap-3">
          <Input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search name, description, or origin"
            aria-label="Search skills"
            className="max-w-md"
          />
          <span className="text-caption text-muted-foreground">
            {filtered.length} of {skills.data?.length ?? 0}
          </span>
        </div>
      </Panel>
      <ErrorState error={skills.error} />
      {skills.data?.length === 0 ? <EmptyState>No skills.</EmptyState> : null}
      {skills.data && skills.data.length > 0 && filtered.length === 0 ? (
        <EmptyState>No skills match this search.</EmptyState>
      ) : null}
      <div className="grid gap-3 md:grid-cols-2">
        {filtered.map((skill) => (
          <AppLink key={skill.id} href={paths.skillDetail(skill.id)} className="rounded-xl border bg-card p-4 hover:bg-muted/40">
            <div className="flex items-center justify-between gap-3">
              <strong>{skill.name}</strong>
              <Badge variant="outline">{skillOriginLabel(skill)}</Badge>
            </div>
            <p className="mt-2 line-clamp-2 text-body text-muted-foreground">{skill.description || "No description"}</p>
          </AppLink>
        ))}
      </div>
    </PageFrame>
  );
}

function SkillAgentBindings({ skillId }: { skillId: string }) {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const agentsKey = lightweightKeys.agents(workspace?.id ?? "");
  const agents = useQuery({ queryKey: agentsKey, queryFn: lightweightApi.listAgents, enabled: !!workspace });
  const [selected, setSelected] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!agents.data) return;
    setSelected(
      new Set(
        agents.data.filter((agent) => agent.skills.some((item) => item.id === skillId)).map((agent) => agent.id),
      ),
    );
  }, [agents.data, skillId]);

  const save = useMutation({
    mutationFn: async (agentIds: Set<string>) => {
      const list = agents.data ?? [];
      await Promise.all(
        list.map(async (agent: LightweightAgent) => {
          const currentlyBound = agent.skills.some((item) => item.id === skillId);
          const shouldBind = agentIds.has(agent.id);
          if (currentlyBound === shouldBind) return;
          const next = agent.skills
            .filter((item) => item.id !== skillId)
            .map((item) => ({ skill_id: item.id, enabled: item.enabled }));
          if (shouldBind) next.push({ skill_id: skillId, enabled: true });
          await lightweightApi.setAgentSkills(agent.id, next);
        }),
      );
    },
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: agentsKey }),
  });

  return (
    <Panel>
      <h2 className="font-semibold">Add to agents</h2>
      <p className="mt-1 text-body text-muted-foreground">Bind this skill to agents you can manage. Saving rewrites each agent’s full skill set.</p>
      <div className="mt-4 grid gap-2">
        {(agents.data ?? []).map((agent) => {
          const binding = agent.skills.find((item) => item.id === skillId);
          return (
            <label key={agent.id} className="flex items-center gap-2 text-body">
              <Checkbox
                checked={selected.has(agent.id)}
                onChange={() => {
                  setSelected((prev) => {
                    const next = new Set(prev);
                    if (next.has(agent.id)) next.delete(agent.id);
                    else next.add(agent.id);
                    return next;
                  });
                }}
              />
              <span className="flex-1">{agent.name}</span>
              {binding ? <Badge variant={binding.enabled ? "secondary" : "outline"}>{binding.enabled ? "enabled" : "disabled"}</Badge> : null}
            </label>
          );
        })}
      </div>
      {(agents.data?.length ?? 0) === 0 ? <p className="mt-3 text-body text-muted-foreground">No agents yet.</p> : null}
      <ErrorState error={save.error} />
      <div className="mt-4">
        <Button type="button" onClick={() => save.mutate(selected)} disabled={save.isPending}>
          {save.isPending ? "Saving…" : "Save agent bindings"}
        </Button>
      </div>
    </Panel>
  );
}

export function SkillDetailPage({ id }: { id: string }) {
  const workspace = useCurrentWorkspace();
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const queryClient = useQueryClient();
  const key = lightweightKeys.skills(workspace?.id ?? "");
  const skill = useQuery({ queryKey: [...key, id], queryFn: () => lightweightApi.getSkill(id), enabled: !!workspace && !!id });
  const files = useQuery({ queryKey: [...key, id, "files"], queryFn: () => lightweightApi.listSkillFiles(id), enabled: !!workspace && !!id });
  const agents = useQuery({
    queryKey: lightweightKeys.agents(workspace?.id ?? ""),
    queryFn: lightweightApi.listAgents,
    enabled: !!workspace,
  });
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const refresh = () => void queryClient.invalidateQueries({ queryKey: key });
  const update = useMutation({ mutationFn: (body: Record<string, unknown>) => lightweightApi.updateSkill(id, body), onSuccess: refresh });
  const replaceFiles = useMutation({
    mutationFn: async ({ value, content }: { value: Array<{ path: string; content: string }>; content: string }) => {
      await lightweightApi.updateSkill(id, {
        name: skill.data?.name,
        description: skill.data?.description,
        content,
      });
      return lightweightApi.replaceSkillFiles(id, value);
    },
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: () => lightweightApi.deleteSkill(id),
    onSuccess: () => navigation.push(paths.skills()),
    onError: (error) => {
      if (error instanceof ApiError && error.status === 409) {
        const boundAgents = (agents.data ?? [])
          .filter((agent) => agent.skills.some((item) => item.id === id))
          .map((agent) => agent.name);
        setDeleteError(
          boundAgents.length > 0
            ? `This skill is still bound to: ${boundAgents.join(", ")}. Unbind it from those agents first.`
            : "This skill is still bound to one or more agents. Unbind it first, then delete.",
        );
        return;
      }
      setDeleteError(error instanceof Error ? error.message : "Delete failed");
    },
  });

  if (!skill.data) {
    return (
      <PageFrame title="Skill">
        <ErrorState error={skill.error} />
        {skill.isPending ? <p>Loading…</p> : null}
      </PageFrame>
    );
  }

  return (
    <PageFrame
      title={skill.data.name}
      description="Edit the canonical skill content and its complete file set."
      action={
        <Button
          variant="destructive"
          onClick={() => {
            setDeleteError(null);
            remove.mutate();
          }}
          disabled={remove.isPending}
        >
          Delete
        </Button>
      }
    >
      {deleteError ? (
        <p role="alert" className="rounded-md bg-destructive/10 px-3 py-2 text-body text-destructive" data-testid="skill-delete-conflict">
          {deleteError}
        </p>
      ) : null}
      <Panel>
        <form
          className="grid gap-4"
          onSubmit={onForm(async (form) =>
            update.mutateAsync({
              name: formValue(form, "name"),
              description: formValue(form, "description"),
              content: formValue(form, "content"),
            }),
          )}
        >
          <Field label="Name" name="name" defaultValue={skill.data.name} required />
          <TextField label="Description" name="description" defaultValue={skill.data.description} rows={2} />
          <TextField label="SKILL.md content" name="content" defaultValue={skill.data.content} rows={12} />
          <div className="flex flex-wrap gap-2 text-caption text-muted-foreground">
            <Badge variant="outline">{skillOriginLabel(skill.data)}</Badge>
          </div>
          <ErrorState error={update.error ?? (deleteError ? null : remove.error)} />
          <SubmitButton pending={update.isPending}>Save skill</SubmitButton>
        </form>
      </Panel>

      <SkillFilesEditor
        skillContent={skill.data.content}
        files={files.data ?? []}
        pending={replaceFiles.isPending}
        error={replaceFiles.error}
        onSave={async (value, content) => replaceFiles.mutateAsync({ value, content })}
      />

      <SkillAgentBindings skillId={id} />
    </PageFrame>
  );
}
