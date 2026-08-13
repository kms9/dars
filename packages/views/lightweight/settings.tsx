"use client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { lightweightApi, lightweightKeys } from "@dars/core/lightweight";
import { paths, useCurrentWorkspace } from "@dars/core/paths";
import { workspaceKeys, workspaceListOptions } from "@dars/core/workspace";
import { useNavigation } from "../navigation";
import { EmptyState, ErrorState, Field, PageFrame, Panel, SubmitButton, TextField, formValue, onForm } from "./components";

export function SettingsPage() {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const navigation = useNavigation();
  const members = useQuery({ queryKey: lightweightKeys.members(workspace?.id ?? ""), queryFn: () => lightweightApi.listMembers(workspace?.id ?? ""), enabled: !!workspace });
  const refresh = () => void queryClient.invalidateQueries({ queryKey: workspaceKeys.list() });
  const update = useMutation({ mutationFn: (body: Record<string, unknown>) => lightweightApi.updateWorkspace(workspace?.id ?? "", body), onSuccess: refresh });
  const remove = useMutation({ mutationFn: () => lightweightApi.deleteWorkspace(workspace?.id ?? ""), onSuccess: () => { refresh(); navigation.push(paths.newWorkspace()); } });
  if (!workspace) return <PageFrame title="Settings"><p>Loading…</p></PageFrame>;
  return (
    <PageFrame title="Workspace settings" description="Basic workspace metadata and a read-only member list. Invitations are not part of Lightweight P0.">
      <Panel>
        <form className="grid gap-4" onSubmit={onForm(async (form) => update.mutateAsync({ name: formValue(form, "name"), description: formValue(form, "description") || null, context: formValue(form, "context") }))}>
          <Field label="Name" name="name" defaultValue={workspace.name} required />
          <TextField label="Description" name="description" defaultValue={workspace.description ?? ""} rows={3} />
          <TextField label="Workspace context" name="context" defaultValue={workspace.context ?? ""} rows={8} />
          <ErrorState error={update.error} /><SubmitButton pending={update.isPending}>Save workspace</SubmitButton>
        </form>
      </Panel>
      <Panel>
        <h2 className="font-semibold">Members</h2>
        <div className="mt-4 grid gap-2">{members.data?.map((member) => <div key={member.id} className="flex items-center justify-between gap-3 rounded-lg border p-3 text-body"><div><strong>{member.name}</strong><div className="text-caption text-muted-foreground">{member.email}</div></div><Badge variant="outline">{member.role}</Badge></div>)}</div>
        <ErrorState error={members.error} />
      </Panel>
      <Panel className="border-destructive/30"><h2 className="font-semibold text-destructive">Danger zone</h2><p className="mt-1 text-body text-muted-foreground">Deletion is rejected while tasks are active or runtimes are online.</p><Button className="mt-4" variant="destructive" onClick={() => remove.mutate()}>Delete workspace</Button><ErrorState error={remove.error} /></Panel>
    </PageFrame>
  );
}

export function WorkspacePickerPage() {
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const workspaces = useQuery(workspaceListOptions());
  const create = useMutation({ mutationFn: (body: { name: string; slug?: string }) => lightweightApi.createWorkspace(body), onSuccess: (workspace) => { void queryClient.invalidateQueries({ queryKey: workspaceKeys.list() }); navigation.push(paths.workspace(workspace.slug).runs()); } });
  return (
    <main className="h-svh overflow-y-auto bg-muted/20"><div className="mx-auto grid min-h-full max-w-3xl content-center gap-6 p-6"><div><h1 className="text-display-sm font-semibold">Choose a workspace</h1><p className="mt-1 text-body text-muted-foreground">Select an existing workspace or create an isolated Lightweight workspace.</p></div>
      <Panel><div className="grid gap-2">{workspaces.data?.map((workspace) => <Button key={workspace.id} variant="outline" className="justify-start" onClick={() => navigation.push(paths.workspace(workspace.slug).runs())}>{workspace.name}<span className="ml-auto text-caption text-muted-foreground">{workspace.slug}</span></Button>)}{workspaces.data?.length === 0 ? <EmptyState>No workspace yet.</EmptyState> : null}</div></Panel>
      <Panel><form className="grid gap-4" onSubmit={onForm(async (form) => create.mutateAsync({ name: formValue(form, "name"), slug: formValue(form, "slug") || undefined }))}><Field label="Workspace name" name="name" required /><Field label="Slug (optional)" name="slug" placeholder="generated when empty" /><ErrorState error={create.error} /><SubmitButton pending={create.isPending}>Create workspace</SubmitButton></form></Panel>
    </div></main>
  );
}
