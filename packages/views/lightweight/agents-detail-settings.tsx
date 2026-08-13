"use client";

import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  EMPTY_AGENT_DRAFT,
  buildInvocationTargets,
  deriveDuplicateAccess,
  isRuntimeUsableForUser,
  lightweightApi,
  lightweightKeys,
  runtimeDisplayLabel,
  type AgentPermissionScope,
  type LightweightAgent,
  type LightweightRuntime,
} from "@dars/core/lightweight";
import { useAuthStore } from "@dars/core/auth";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import { Textarea } from "@dars/ui/components/ui/textarea";
import { classifyAgentUpdateError } from "./agents-detail-hooks";
import { ErrorState, Field, Panel, SubmitButton, TextField, formValue, onForm } from "./components";

type SettingsSection = "general" | "access" | "environment" | "custom-args" | "runtime-config";

export function AgentDetailSettingsPanel({
  agent,
  runtimes,
  workspaceId,
  canManage,
  canRevealEnv,
  members,
}: {
  agent: LightweightAgent;
  runtimes: LightweightRuntime[];
  workspaceId: string;
  canManage: boolean;
  canRevealEnv: boolean;
  members: Array<{ id: string; user_id: string; name: string }>;
}) {
  const user = useAuthStore((state) => state.user);
  const queryClient = useQueryClient();
  const [section, setSection] = useState<SettingsSection>("general");
  const [revealedEnv, setRevealedEnv] = useState<Record<string, string> | null>(null);
  const [conflict, setConflict] = useState(false);
  const access = deriveDuplicateAccess(agent);
  const [permissionScope, setPermissionScope] = useState<AgentPermissionScope>(access.permissionScope);
  const [memberIds, setMemberIds] = useState<Set<string>>(access.memberIds);

  useEffect(() => {
    const next = deriveDuplicateAccess(agent);
    setPermissionScope(next.permissionScope);
    setMemberIds(new Set(next.memberIds));
  }, [agent.invocation_targets, agent.permission_mode, agent.updated_at]);

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: lightweightKeys.agents(workspaceId) });
    void queryClient.invalidateQueries({ queryKey: ["lightweight", workspaceId, "agent-detail", agent.id] });
  };

  const update = useMutation({
    mutationFn: (body: Record<string, unknown>) => lightweightApi.updateAgent(agent.id, body),
    onSuccess: () => {
      setConflict(false);
      refresh();
    },
    onError: (error) => {
      const classified = classifyAgentUpdateError(error);
      if (classified.conflict) {
        setConflict(true);
        refresh();
      }
    },
  });

  const lifecycle = useMutation({
    mutationFn: () => (agent.archived_at ? lightweightApi.restoreAgent(agent.id) : lightweightApi.archiveAgent(agent.id)),
    onSuccess: refresh,
  });

  const env = useMutation({
    mutationFn: (value: Record<string, string>) => lightweightApi.updateAgentEnv(agent.id, value),
    onSuccess: () => {
      setRevealedEnv(null);
      refresh();
    },
  });

  const avatar = useMutation({
    mutationFn: (file: File) => lightweightApi.uploadAgentAvatar(agent.id, file, file.name),
    onSuccess: refresh,
  });

  const usableRuntimes = runtimes.filter((runtime) => isRuntimeUsableForUser(runtime, user?.id ?? null));

  return (
    <div className="grid gap-5">
      <div className="flex flex-wrap gap-2 border-b pb-3">
        {([
          ["general", "General"],
          ["access", "Access"],
          ["environment", "Environment"],
          ["custom-args", "Custom args"],
          ["runtime-config", "Runtime config"],
        ] as const).map(([key, label]) => (
          <Button key={key} size="sm" variant={section === key ? "secondary" : "ghost"} onClick={() => setSection(key)}>
            {label}
          </Button>
        ))}
      </div>

      {conflict ? (
        <Panel>
          <p className="text-body text-destructive">
            This agent changed elsewhere. Settings were refetched — review and save again.
          </p>
        </Panel>
      ) : null}

      {section === "general" ? (
        <Panel>
          <form
            className="grid gap-4"
            onSubmit={onForm(async (form) => {
              if (!canManage) return;
              await update.mutateAsync({
                name: formValue(form, "name"),
                description: formValue(form, "description"),
                runtime_id: formValue(form, "runtime_id"),
                model: formValue(form, "model") || null,
                thinking_level: formValue(form, "thinking_level") || null,
                service_tier: formValue(form, "service_tier") || null,
                max_concurrent_tasks: Number(formValue(form, "max_concurrent_tasks")),
              });
            })}
          >
            <div className="grid gap-2">
              <span className="text-body font-medium">Avatar</span>
              <Input
                type="file"
                accept="image/png,image/jpeg"
                disabled={!canManage}
                onChange={(event) => {
                  const file = event.target.files?.[0];
                  if (file) void avatar.mutateAsync(file);
                }}
              />
            </div>
            <Field label="Name" name="name" defaultValue={agent.name} required />
            <TextField label="Description" name="description" defaultValue={agent.description} rows={3} />
            <div className="grid gap-2">
              <label className="text-body font-medium" htmlFor="runtime_id">Runtime</label>
              <select
                id="runtime_id"
                name="runtime_id"
                defaultValue={agent.runtime_id ?? ""}
                required
                disabled={!canManage}
                className="h-9 rounded-md border bg-background px-3 text-body"
              >
                {usableRuntimes.map((runtime) => (
                  <option key={runtime.id} value={runtime.id}>
                    {runtimeDisplayLabel(runtime)} · {runtime.status}
                  </option>
                ))}
              </select>
            </div>
            <div className="grid gap-4 md:grid-cols-3">
              <Field label="Model" name="model" defaultValue={agent.model ?? ""} />
              <Field label="Thinking" name="thinking_level" defaultValue={agent.thinking_level ?? ""} />
              <Field label="Service tier" name="service_tier" defaultValue={agent.service_tier ?? ""} />
            </div>
            <Field
              label="Max concurrent tasks"
              name="max_concurrent_tasks"
              type="number"
              defaultValue={agent.max_concurrent_tasks}
              required
            />
            <ErrorState error={update.error ?? avatar.error} />
            {canManage ? <SubmitButton pending={update.isPending}>Save general</SubmitButton> : null}
            {canManage ? (
              <Button type="button" variant="outline" onClick={() => lifecycle.mutate()}>
                {agent.archived_at ? "Restore agent" : "Archive agent"}
              </Button>
            ) : null}
            <ErrorState error={lifecycle.error} />
          </form>
        </Panel>
      ) : null}

      {section === "access" ? (
        <Panel>
          <form
            className="grid gap-4"
            onSubmit={onForm(async () => {
              if (!canManage) return;
              const permission_mode = permissionScope === "private" ? "private" : "public_to";
              await update.mutateAsync({
                permission_mode,
                invocation_targets: buildInvocationTargets({
                  ...EMPTY_AGENT_DRAFT,
                  permissionScope,
                  memberIds,
                }),
              });
            })}
          >
            <label className="grid gap-2 text-body">
              <span className="font-medium">Access</span>
              <select
                className="h-9 rounded-md border bg-background px-3"
                value={permissionScope}
                disabled={!canManage}
                onChange={(event) => {
                  const value = event.target.value as AgentPermissionScope;
                  setPermissionScope(value);
                  if (value !== "members") setMemberIds(new Set());
                }}
              >
                <option value="private">Private</option>
                <option value="workspace">Workspace</option>
                <option value="members">Specified members</option>
              </select>
            </label>
            {permissionScope === "members" ? (
              <div className="grid gap-2">
                {members.map((member) => (
                  <label key={member.id} className="flex items-center gap-2 text-body">
                    <Checkbox
                      checked={memberIds.has(member.id)}
                      disabled={!canManage}
                      onChange={() => {
                        setMemberIds((current) => {
                          const next = new Set(current);
                          if (next.has(member.id)) next.delete(member.id);
                          else next.add(member.id);
                          return next;
                        });
                      }}
                    />
                    {member.name}
                  </label>
                ))}
              </div>
            ) : null}
            <ErrorState error={update.error} />
            {canManage ? <SubmitButton pending={update.isPending}>Save access</SubmitButton> : null}
          </form>
        </Panel>
      ) : null}

      {section === "environment" ? (
        <Panel>
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="font-semibold">Encrypted environment</h3>
              <p className="text-body text-muted-foreground">Reveal and update are audited. Only owners and admins may edit.</p>
            </div>
            {canRevealEnv ? (
              <Button
                variant="outline"
                onClick={() =>
                  lightweightApi.revealAgentEnv(agent.id).then((result) => setRevealedEnv(result.custom_env))
                }
              >
                Reveal
              </Button>
            ) : null}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            {agent.custom_env_keys.map((keyName) => (
              <Badge key={keyName} variant="outline">
                {keyName}=****
              </Badge>
            ))}
            {agent.custom_env_keys.length === 0 ? (
              <p className="text-body text-muted-foreground">No environment keys configured.</p>
            ) : null}
          </div>
          {revealedEnv && canRevealEnv ? (
            <form
              className="mt-4 grid gap-3"
              onSubmit={onForm(async (form) => {
                try {
                  const parsed = JSON.parse(formValue(form, "custom_env")) as Record<string, string>;
                  await env.mutateAsync(parsed);
                } catch {
                  throw new Error("Environment JSON is invalid");
                }
              })}
            >
              <Textarea name="custom_env" rows={8} defaultValue={JSON.stringify(revealedEnv, null, 2)} />
              <ErrorState error={env.error} />
              <SubmitButton pending={env.isPending}>Update encrypted env</SubmitButton>
            </form>
          ) : null}
        </Panel>
      ) : null}

      {section === "custom-args" ? (
        <Panel>
          <form
            className="grid gap-3"
            onSubmit={onForm(async (form) => {
              if (!canManage) return;
              try {
                const parsed = JSON.parse(formValue(form, "custom_args"));
                if (!Array.isArray(parsed)) throw new Error("Custom args must be a JSON array");
                await update.mutateAsync({ custom_args: parsed });
              } catch (error) {
                throw error instanceof Error ? error : new Error("Custom args JSON is invalid");
              }
            })}
          >
            <Textarea
              name="custom_args"
              rows={8}
              readOnly={!canManage}
              defaultValue={JSON.stringify(agent.custom_args ?? [], null, 2)}
            />
            <ErrorState error={update.error} />
            {canManage ? <SubmitButton pending={update.isPending}>Save custom args</SubmitButton> : null}
          </form>
        </Panel>
      ) : null}

      {section === "runtime-config" ? (
        <Panel>
          {agent.runtime_id ? (
            <form
              className="grid gap-3"
              onSubmit={onForm(async (form) => {
                if (!canManage) return;
                try {
                  const parsed = JSON.parse(formValue(form, "runtime_config"));
                  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
                    throw new Error("Runtime config must be a JSON object");
                  }
                  await update.mutateAsync({ runtime_config: parsed });
                } catch (error) {
                  throw error instanceof Error ? error : new Error("Runtime config JSON is invalid");
                }
              })}
            >
              <Textarea
                name="runtime_config"
                rows={10}
                readOnly={!canManage}
                defaultValue={JSON.stringify(agent.runtime_config ?? {}, null, 2)}
              />
              <ErrorState error={update.error} />
              {canManage ? <SubmitButton pending={update.isPending}>Save runtime config</SubmitButton> : null}
            </form>
          ) : (
            <p className="text-body text-muted-foreground">Bind a runtime in General settings to edit runtime-specific config.</p>
          )}
        </Panel>
      ) : null}
    </div>
  );
}
