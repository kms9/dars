"use client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { lightweightApi, lightweightKeys } from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { AppLink, useNavigation } from "../navigation";
import { EmptyState, ErrorState, Field, PageFrame, Panel, SubmitButton, TextField, formValue, onForm } from "./components";

export function RuntimesPage() {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const queryClient = useQueryClient();
  const runtimeKey = lightweightKeys.runtimes(workspace?.id ?? "");
  const profileKey = lightweightKeys.profiles(workspace?.id ?? "");
  const runtimes = useQuery({ queryKey: runtimeKey, queryFn: lightweightApi.listRuntimes, enabled: !!workspace, refetchInterval: 10000 });
  const profiles = useQuery({ queryKey: profileKey, queryFn: () => lightweightApi.listRuntimeProfiles(workspace?.id ?? ""), enabled: !!workspace });
  const createProfile = useMutation({ mutationFn: (body: Record<string, unknown>) => lightweightApi.createRuntimeProfile(workspace?.id ?? "", body), onSuccess: () => void queryClient.invalidateQueries({ queryKey: profileKey }) });
  const deleteProfile = useMutation({ mutationFn: (id: string) => lightweightApi.deleteRuntimeProfile(workspace?.id ?? "", id), onSuccess: () => void queryClient.invalidateQueries({ queryKey: profileKey }) });
  return (
    <PageFrame title="Runtimes" description="Local Daemon registrations and workspace-scoped runtime profiles.">
      <Panel>
        <h2 className="font-semibold">Daemon runtimes</h2>
        <div className="mt-4 grid gap-3 md:grid-cols-2">{runtimes.data?.map((runtime) => <AppLink key={runtime.id} href={paths.runtimeDetail(runtime.id)} className="rounded-lg border p-4 hover:bg-muted/40"><div className="flex justify-between gap-3"><strong>{runtime.name}</strong><Badge variant={runtime.status === "online" ? "secondary" : "outline"}>{runtime.status}</Badge></div><p className="mt-2 text-body text-muted-foreground">{runtime.provider} · {runtime.device_info || runtime.daemon_id}</p></AppLink>)}</div>
        {runtimes.data?.length === 0 ? <EmptyState>No Daemon has registered a runtime for this workspace.</EmptyState> : null}
        <ErrorState error={runtimes.error} />
      </Panel>

      <Panel>
        <h2 className="font-semibold">Runtime profiles</h2>
        <p className="mt-1 text-body text-muted-foreground">Profiles describe commands a Daemon may register. Deletion is blocked while a runtime uses the profile.</p>
        <form className="mt-4 grid gap-4 md:grid-cols-2" onSubmit={onForm(async (form) => createProfile.mutateAsync({ display_name: formValue(form, "display_name"), protocol_family: formValue(form, "protocol_family"), command_name: formValue(form, "command_name"), description: formValue(form, "description") || null, fixed_args: formValue(form, "fixed_args").split(" ").filter(Boolean), enabled: true }))}>
          <Field label="Display name" name="display_name" required />
          <Field label="Protocol family" name="protocol_family" required placeholder="acp" />
          <Field label="Command" name="command_name" required placeholder="codex-acp" />
          <Field label="Fixed args" name="fixed_args" placeholder="--flag value" />
          <div className="md:col-span-2"><TextField label="Description" name="description" rows={2} /></div>
          <div className="md:col-span-2"><ErrorState error={createProfile.error} /><SubmitButton pending={createProfile.isPending}>Create profile</SubmitButton></div>
        </form>
        <div className="mt-5 grid gap-2">{profiles.data?.map((profile) => <div key={profile.id} className="flex items-center justify-between gap-3 rounded-lg border p-3 text-body"><div><strong>{profile.display_name}</strong><div className="text-caption text-muted-foreground">{profile.protocol_family} · {profile.command_name} {profile.fixed_args.join(" ")}</div></div><Button size="sm" variant="ghost" onClick={() => deleteProfile.mutate(profile.id)}>Delete</Button></div>)}</div>
        <ErrorState error={deleteProfile.error} />
      </Panel>
    </PageFrame>
  );
}

export function RuntimeDetailPage({ id }: { id: string }) {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const key = lightweightKeys.runtimes(workspace?.id ?? "");
  const runtimes = useQuery({ queryKey: key, queryFn: lightweightApi.listRuntimes, enabled: !!workspace });
  const runtime = runtimes.data?.find((item) => item.id === id);
  const rename = useMutation({ mutationFn: (name: string) => lightweightApi.updateRuntime(id, name || null), onSuccess: () => void queryClient.invalidateQueries({ queryKey: key }) });
  const remove = useMutation({ mutationFn: () => lightweightApi.deleteRuntime(id), onSuccess: () => navigation.push(paths.runtimes()) });
  return (
    <PageFrame title={runtime?.name ?? "Runtime"} description="Runtime identity and Daemon heartbeat are server-owned." action={runtime ? <Button variant="destructive" disabled={runtime.status === "online"} onClick={() => remove.mutate()}>Delete offline runtime</Button> : null}>
      <ErrorState error={runtimes.error ?? rename.error ?? remove.error} />
      {runtime ? <Panel><dl className="grid gap-3 text-body md:grid-cols-2"><div><dt className="text-muted-foreground">Provider</dt><dd>{runtime.provider}</dd></div><div><dt className="text-muted-foreground">Status</dt><dd>{runtime.status}</dd></div><div><dt className="text-muted-foreground">Daemon</dt><dd>{runtime.daemon_id}</dd></div><div><dt className="text-muted-foreground">Last seen</dt><dd>{runtime.last_seen_at ? new Date(runtime.last_seen_at).toLocaleString() : "Never"}</dd></div></dl><form className="mt-5 flex gap-2" onSubmit={onForm(async (form) => rename.mutateAsync(formValue(form, "custom_name")))}><div className="flex-1"><Field label="Custom display name" name="custom_name" defaultValue={runtime.name} /></div><div className="self-end"><SubmitButton pending={rename.isPending}>Rename</SubmitButton></div></form></Panel> : runtimes.isPending ? <p>Loading…</p> : <EmptyState>Runtime not found.</EmptyState>}
    </PageFrame>
  );
}
