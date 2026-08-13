"use client";

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@dars/ui/components/ui/button";
import { Input } from "@dars/ui/components/ui/input";
import {
  lightweightApi,
  lightweightKeys,
  unwrapSkillImportResponse,
  type LightweightSkill,
  type LightweightSkillOnConflict,
} from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { useNavigation } from "../navigation";
import { ErrorState, Field, Panel, SubmitButton, TextField, formValue, onForm } from "./components";
import { RuntimeLocalSkillImportPanel } from "./skills-runtime-import";

export type CreateMethod = "chooser" | "manual" | "url" | "runtime";

const METHODS: Array<{ key: Exclude<CreateMethod, "chooser">; title: string; description: string }> = [
  { key: "manual", title: "Manual", description: "Write a new skill with SKILL.md content." },
  { key: "url", title: "URL / file", description: "Import from a public URL or upload a .skill / .zip archive." },
  { key: "runtime", title: "Runtime", description: "Copy local skills discovered on an online Daemon runtime." },
];

function ConflictSelect({ value, onChange }: { value: LightweightSkillOnConflict; onChange: (value: LightweightSkillOnConflict) => void }) {
  return (
    <div className="grid gap-2">
      <label className="text-body font-medium" htmlFor="on_conflict">On conflict</label>
      <select
        id="on_conflict"
        value={value}
        onChange={(event) => onChange(event.target.value as LightweightSkillOnConflict)}
        className="h-9 rounded-md border bg-background px-3 text-body"
      >
        <option value="fail">Fail</option>
        <option value="overwrite">Overwrite (creator only)</option>
        <option value="rename">Rename</option>
        <option value="skip">Skip</option>
      </select>
    </div>
  );
}

function ManualCreateForm({ onCreated, onBack }: { onCreated: (skill: LightweightSkill) => void; onBack: () => void }) {
  const create = useMutation({
    mutationFn: (body: Record<string, unknown>) => lightweightApi.createSkill(body),
    onSuccess: onCreated,
  });
  return (
    <Panel>
      <form
        className="grid gap-4"
        onSubmit={onForm(async (form) => {
          await create.mutateAsync({
            name: formValue(form, "name"),
            description: formValue(form, "description"),
            content: formValue(form, "content"),
            config: {},
          });
        })}
      >
        <Field label="Name" name="name" required />
        <TextField label="Description" name="description" rows={2} />
        <TextField label="SKILL.md content" name="content" required rows={8} />
        <ErrorState error={create.error} />
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={onBack}>Back</Button>
          <SubmitButton pending={create.isPending}>Create skill</SubmitButton>
        </div>
      </form>
    </Panel>
  );
}

function UrlImportForm({ onCreated, onBack }: { onCreated: (skill: LightweightSkill) => void; onBack: () => void }) {
  const [onConflict, setOnConflict] = useState<LightweightSkillOnConflict>("fail");
  const [file, setFile] = useState<File | null>(null);
  const importUrl = useMutation({
    mutationFn: async (url: string) => {
      const result = await lightweightApi.importSkill({ url, on_conflict: onConflict });
      const unwrapped = unwrapSkillImportResponse(result);
      if (!unwrapped.skill) {
        throw new Error(unwrapped.reason || unwrapped.status || "Import did not create a skill");
      }
      return unwrapped.skill as LightweightSkill;
    },
    onSuccess: onCreated,
  });
  const importArchive = useMutation({
    mutationFn: async (upload: File) => {
      const result = await lightweightApi.importSkillArchive(upload, upload.name, onConflict);
      const unwrapped = unwrapSkillImportResponse(result);
      if (!unwrapped.skill) {
        throw new Error(unwrapped.reason || unwrapped.status || "Archive import did not create a skill");
      }
      return unwrapped.skill as LightweightSkill;
    },
    onSuccess: onCreated,
  });
  const pending = importUrl.isPending || importArchive.isPending;
  const error = importUrl.error ?? importArchive.error;

  return (
    <Panel>
      <form
        className="grid gap-4"
        onSubmit={onForm(async (form) => {
          const url = formValue(form, "url");
          if (file) {
            await importArchive.mutateAsync(file);
            return;
          }
          if (!url) throw new Error("Paste a URL or choose a .skill / .zip file");
          await importUrl.mutateAsync(url);
        })}
      >
        <Field label="Skill URL" name="url" placeholder="https://clawhub.ai/owner/skill" />
        <div className="grid gap-2">
          <label className="text-body font-medium" htmlFor="skill-archive">Archive (.skill / .zip)</label>
          <Input
            id="skill-archive"
            type="file"
            accept=".skill,.zip,application/zip"
            onChange={(event) => setFile(event.target.files?.[0] ?? null)}
          />
          {file ? <p className="text-caption text-muted-foreground">Selected: {file.name}</p> : null}
        </div>
        <ConflictSelect value={onConflict} onChange={setOnConflict} />
        <p className="text-caption text-muted-foreground">
          Supported URL hosts: clawhub.ai, skills.sh, github.com. File upload uses the same conflict policy.
        </p>
        <ErrorState error={error} />
        <div className="flex gap-2">
          <Button type="button" variant="outline" onClick={onBack} disabled={pending}>Back</Button>
          <SubmitButton pending={pending}>{file ? "Upload archive" : "Import from URL"}</SubmitButton>
        </div>
      </form>
    </Panel>
  );
}

export function SkillsCreatePanel({ onClose }: { onClose?: () => void } = {}) {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const [method, setMethod] = useState<CreateMethod>("chooser");

  const handleCreated = (skill: LightweightSkill) => {
    void queryClient.invalidateQueries({ queryKey: lightweightKeys.skills(workspace?.id ?? "") });
    onClose?.();
    navigation.push(paths.skillDetail(skill.id));
  };

  if (method === "chooser") {
    return (
      <Panel>
        <h2 className="font-semibold">Create skill</h2>
        <p className="mt-1 text-body text-muted-foreground">Choose how to add a workspace skill.</p>
        <div className="mt-4 grid gap-2">
          {METHODS.map((item) => (
            <button
              key={item.key}
              type="button"
              onClick={() => setMethod(item.key)}
              className="rounded-lg border bg-background p-4 text-left hover:bg-muted/40"
              data-testid={`create-method-${item.key}`}
            >
              <div className="font-medium">{item.title}</div>
              <p className="mt-1 text-caption text-muted-foreground">{item.description}</p>
            </button>
          ))}
        </div>
      </Panel>
    );
  }

  if (method === "manual") {
    return <ManualCreateForm onCreated={handleCreated} onBack={() => setMethod("chooser")} />;
  }
  if (method === "url") {
    return <UrlImportForm onCreated={handleCreated} onBack={() => setMethod("chooser")} />;
  }
  return (
    <RuntimeLocalSkillImportPanel
      onImported={handleCreated}
      onBack={() => setMethod("chooser")}
    />
  );
}
