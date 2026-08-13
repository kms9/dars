"use client";

import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@dars/core/auth";
import {
  buildSquadListRows,
  computeSquadScopeCounts,
  paginateRows,
  rowMatchesSquadFilters,
  sortSquadRows,
  squadScopeMatches,
  useSquadsViewStore,
  type SquadListRow,
  type SquadsScope,
} from "@dars/core/lightweight";
import { lightweightApi, lightweightKeys } from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button, buttonVariants } from "@dars/ui/components/ui/button";
import { Checkbox } from "@dars/ui/components/ui/checkbox";
import { Input } from "@dars/ui/components/ui/input";
import { Textarea } from "@dars/ui/components/ui/textarea";
import { AppLink, useNavigation } from "../navigation";
import { EmptyState, ErrorState, PageFrame, Panel, SubmitButton } from "./components";

function ModalFrame({
  title,
  description,
  children,
  onClose,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-background/70 p-4 backdrop-blur-sm">
      <div className="w-full max-w-lg rounded-xl border bg-card p-5 shadow-lg">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="text-title-sm font-semibold">{title}</h2>
            {description ? <p className="mt-1 text-body text-muted-foreground">{description}</p> : null}
          </div>
          <Button size="sm" variant="ghost" onClick={onClose}>Close</Button>
        </div>
        <div className="mt-4">{children}</div>
      </div>
    </div>
  );
}

function scopeLabel(scope: SquadsScope, counts: { mine: number; all: number }): string {
  if (scope === "mine") return `My (${counts.mine})`;
  return `All (${counts.all})`;
}

function SquadAvatar({ name, url, size = "md" }: { name: string; url: string | null; size?: "sm" | "md" }) {
  const className = size === "sm" ? "size-8 text-caption" : "size-10 text-body";
  if (url) {
    return (
      <img src={url} alt="" className={`${className} rounded-full object-cover`} />
    );
  }
  return (
    <div className={`flex ${className} items-center justify-center rounded-full bg-muted font-semibold`}>
      {name.slice(0, 1).toUpperCase()}
    </div>
  );
}

function MemberPreviewStack({ row }: { row: SquadListRow }) {
  const preview = row.squad.member_preview.slice(0, 3);
  return (
    <div className="flex items-center gap-1">
      {preview.map((member) => (
        <SquadAvatar key={member.agent_id} name={member.name} url={null} size="sm" />
      ))}
      {row.squad.member_count > preview.length ? (
        <span className="text-caption text-muted-foreground">+{row.squad.member_count - preview.length}</span>
      ) : null}
    </div>
  );
}

function CreateSquadModal({
  open,
  onOpenChange,
  agents,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agents: Array<{ id: string; name: string; archived_at: string | null }>;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [instructions, setInstructions] = useState("");
  const [leaderId, setLeaderId] = useState("");
  const [additionalIds, setAdditionalIds] = useState<Set<string>>(new Set());
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [avatarRetrySquadId, setAvatarRetrySquadId] = useState<string | null>(null);
  const [avatarError, setAvatarError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const callableAgents = agents.filter((agent) => !agent.archived_at);

  useEffect(() => {
    if (!avatarFile) {
      setAvatarPreview(null);
      return;
    }
    const url = URL.createObjectURL(avatarFile);
    setAvatarPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [avatarFile]);

  const reset = () => {
    setName("");
    setDescription("");
    setInstructions("");
    setLeaderId("");
    setAdditionalIds(new Set());
    setAvatarFile(null);
    setError(null);
    setAvatarRetrySquadId(null);
    setAvatarError(null);
  };

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!leaderId || !name.trim()) return;
    setPending(true);
    setError(null);
    setAvatarError(null);
    try {
      const squad = await lightweightApi.createSquad({
        name: name.trim(),
        description,
        instructions,
        leader_id: leaderId,
        additional_agent_ids: [...additionalIds],
      });
      if (avatarFile) {
        try {
          await lightweightApi.uploadSquadAvatar(squad.id, avatarFile, avatarFile.name);
          setAvatarRetrySquadId(null);
        } catch (uploadError) {
          setAvatarRetrySquadId(squad.id);
          setAvatarError(uploadError instanceof Error ? uploadError.message : "Avatar upload failed");
          onCreated();
          setPending(false);
          return;
        }
      }
      reset();
      onOpenChange(false);
      onCreated();
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "Create failed");
    } finally {
      setPending(false);
    }
  };

  const retryAvatar = async () => {
    if (!avatarRetrySquadId || !avatarFile) return;
    setAvatarError(null);
    try {
      await lightweightApi.uploadSquadAvatar(avatarRetrySquadId, avatarFile, avatarFile.name);
      setAvatarRetrySquadId(null);
      onOpenChange(false);
      onCreated();
    } catch (uploadError) {
      setAvatarError(uploadError instanceof Error ? uploadError.message : "Avatar upload failed");
    }
  };

  if (!open) return null;

  return (
    <ModalFrame
      title="Create squad"
      description="Agent-only roster with one leader. Additional agents are added in the same transaction."
      onClose={() => {
        reset();
        onOpenChange(false);
      }}
    >
      <form className="grid gap-4" onSubmit={submit}>
          <div className="flex items-center gap-4">
            {avatarPreview ? (
              <img src={avatarPreview} alt="" className="size-14 rounded-full object-cover" />
            ) : (
              <div className="flex size-14 items-center justify-center rounded-full bg-muted text-title-sm font-semibold">
                {name.slice(0, 1).toUpperCase() || "S"}
              </div>
            )}
            <Input type="file" accept="image/png,image/jpeg" onChange={(event) => setAvatarFile(event.target.files?.[0] ?? null)} />
          </div>
          <label className="grid gap-2 text-body">
            <span className="font-medium">Name</span>
            <Input value={name} onChange={(event) => setName(event.target.value)} required maxLength={200} />
          </label>
          <label className="grid gap-2 text-body">
            <span className="font-medium">Description</span>
            <Textarea value={description} onChange={(event) => setDescription(event.target.value)} rows={2} />
          </label>
          <label className="grid gap-2 text-body">
            <span className="font-medium">Instructions</span>
            <Textarea value={instructions} onChange={(event) => setInstructions(event.target.value)} rows={4} />
          </label>
          <label className="grid gap-2 text-body">
            <span className="font-medium">Leader agent</span>
            <select
              className="h-9 rounded-md border bg-background px-3"
              value={leaderId}
              onChange={(event) => setLeaderId(event.target.value)}
              required
            >
              <option value="">Select leader</option>
              {callableAgents.map((agent) => (
                <option key={agent.id} value={agent.id}>{agent.name}</option>
              ))}
            </select>
          </label>
          <div className="grid gap-2">
            <span className="text-body font-medium">Additional agents</span>
            <div className="grid gap-2 rounded-lg border p-3">
              {callableAgents.filter((agent) => agent.id !== leaderId).map((agent) => (
                <label key={agent.id} className="flex items-center gap-2 text-body">
                  <Checkbox
                    checked={additionalIds.has(agent.id)}
                    onChange={() => {
                      setAdditionalIds((current) => {
                        const next = new Set(current);
                        if (next.has(agent.id)) next.delete(agent.id);
                        else next.add(agent.id);
                        return next;
                      });
                    }}
                  />
                  {agent.name}
                </label>
              ))}
              {callableAgents.length <= 1 ? <p className="text-caption text-muted-foreground">No other callable agents.</p> : null}
            </div>
          </div>
          <ErrorState error={error} />
          {avatarError ? (
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-body text-destructive">{avatarError}</p>
              {avatarRetrySquadId ? (
                <Button size="sm" variant="outline" onClick={() => void retryAvatar()}>Retry avatar</Button>
              ) : null}
            </div>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" onClick={() => { reset(); onOpenChange(false); }}>Cancel</Button>
            <SubmitButton pending={pending}>Create squad</SubmitButton>
          </div>
        </form>
    </ModalFrame>
  );
}

export function SquadsPage() {
  const workspace = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const user = useAuthStore((state) => state.user);
  const queryClient = useQueryClient();
  const view = useSquadsViewStore();
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [createOpen, setCreateOpen] = useState(false);

  const squadsKey = lightweightKeys.squads(workspace?.id ?? "");
  const squads = useQuery({ queryKey: squadsKey, queryFn: lightweightApi.listSquads, enabled: !!workspace });
  const agents = useQuery({ queryKey: lightweightKeys.agents(workspace?.id ?? ""), queryFn: lightweightApi.listAgents, enabled: !!workspace });
  const members = useQuery({
    queryKey: lightweightKeys.members(workspace?.id ?? ""),
    queryFn: () => lightweightApi.listMembers(workspace!.id),
    enabled: !!workspace,
  });

  const memberRole = members.data?.find((member) => member.user_id === user?.id)?.role ?? null;

  const rows = useMemo(() => {
    const built = buildSquadListRows({
      squads: squads.data ?? [],
      members: members.data ?? [],
      currentUserId: user?.id ?? null,
      memberRole,
    });
    return sortSquadRows(
      built.filter((row) => squadScopeMatches(row.squad, view.scope, user?.id ?? null, memberRole))
        .filter((row) => rowMatchesSquadFilters(row, view.filters, query)),
      view.sortField,
      view.sortDirection,
    );
  }, [
    squads.data,
    members.data,
    user?.id,
    memberRole,
    view.scope,
    view.filters,
    view.sortDirection,
    view.sortField,
    query,
  ]);

  const counts = computeSquadScopeCounts(
    buildSquadListRows({
      squads: squads.data ?? [],
      members: members.data ?? [],
      currentUserId: user?.id ?? null,
      memberRole,
    }),
    user?.id ?? null,
    memberRole,
  );
  const pageInfo = paginateRows(rows, page, view.pageSize);
  const leaderOptions = [...new Map(rows.map((row) => [row.squad.leader_id, row.leaderName])).entries()];
  const creatorOptions = [...new Map(rows.map((row) => [row.squad.creator_id, row.creatorName])).entries()];
  const memberOptions = agents.data?.filter((agent) => !agent.archived_at) ?? [];

  const refresh = () => void queryClient.invalidateQueries({ queryKey: squadsKey });

  const archive = useMutation({
    mutationFn: (id: string) => lightweightApi.archiveSquad(id),
    onSuccess: refresh,
  });

  return (
    <PageFrame
      title="Squads"
      description="Agent-only teams with exactly one leader and explicit role changes."
      action={
        <Button onClick={() => setCreateOpen(true)}>Create squad</Button>
      }
    >
      <Panel>
        <div className="flex flex-wrap items-center gap-3">
          {(["mine", "all"] as SquadsScope[]).map((scope) => (
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
            aria-label="Search squads"
            className="max-w-sm"
          />
          <span className="text-caption text-muted-foreground">{rows.length} result{rows.length === 1 ? "" : "s"}</span>
        </div>
        <div className="mt-4 flex flex-wrap gap-3">
          <label className="grid gap-1 text-caption">
            Leader
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.filters.leaders[0] ?? ""}
              onChange={(event) => {
                view.clearFilters();
                if (event.target.value) view.toggleFilter("leaders", event.target.value);
                setPage(1);
              }}
            >
              <option value="">Any</option>
              {leaderOptions.map(([id, label]) => (
                <option key={id} value={id}>{label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-caption">
            Creator
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.filters.creators[0] ?? ""}
              onChange={(event) => {
                view.clearFilters();
                if (event.target.value) view.toggleFilter("creators", event.target.value);
                setPage(1);
              }}
            >
              <option value="">Any</option>
              {creatorOptions.map(([id, label]) => (
                <option key={id} value={id}>{label}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-caption">
            Member agent
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.filters.members[0] ?? ""}
              onChange={(event) => {
                view.clearFilters();
                if (event.target.value) view.toggleFilter("members", event.target.value);
                setPage(1);
              }}
            >
              <option value="">Any</option>
              {memberOptions.map((agent) => (
                <option key={agent.id} value={agent.id}>{agent.name}</option>
              ))}
            </select>
          </label>
          <label className="grid gap-1 text-caption">
            Sort
            <select
              className="h-9 rounded-md border bg-background px-2"
              value={view.sortField}
              onChange={(event) => {
                view.setSortField(event.target.value as typeof view.sortField);
                setPage(1);
              }}
            >
              <option value="updated">Updated</option>
              <option value="name">Name</option>
              <option value="members">Members</option>
              <option value="created">Created</option>
            </select>
          </label>
          <Button size="sm" variant="ghost" onClick={() => view.toggleSort(view.sortField)}>
            {view.sortDirection === "asc" ? "Ascending" : "Descending"}
          </Button>
        </div>
      </Panel>

      {squads.isPending ? <Panel><p>Loading…</p></Panel> : null}
      <ErrorState error={squads.error} />

      {pageInfo.pageRows.length === 0 && !squads.isPending ? <EmptyState>No squads in this scope.</EmptyState> : null}

      <div className="overflow-hidden rounded-xl border">
        <table className="w-full text-left text-body">
          <thead className="bg-muted/40 text-caption">
            <tr>
              <th className="px-4 py-3">Squad</th>
              <th className="px-4 py-3">Leader</th>
              <th className="px-4 py-3">Members</th>
              <th className="px-4 py-3">Creator</th>
              <th className="px-4 py-3 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {pageInfo.pageRows.map((row) => (
              <tr key={row.squad.id} className="border-t">
                <td className="px-4 py-3">
                  <AppLink href={paths.squadDetail(row.squad.id)} className="flex items-center gap-3 hover:underline">
                    <SquadAvatar name={row.squad.name} url={row.squad.avatar_url} />
                    <span className="font-medium">{row.squad.name}</span>
                  </AppLink>
                </td>
                <td className="px-4 py-3 text-muted-foreground">{row.leaderName}</td>
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <MemberPreviewStack row={row} />
                    <span className="text-caption text-muted-foreground">{row.squad.member_count}</span>
                  </div>
                </td>
                <td className="px-4 py-3 text-muted-foreground">{row.creatorName}</td>
                <td className="px-4 py-3 text-right">
                  {row.canManage ? (
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={archive.isPending}
                      onClick={() => archive.mutate(row.squad.id)}
                    >
                      Archive
                    </Button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {pageInfo.totalPages > 1 ? (
        <div className="flex items-center gap-3">
          <Button size="sm" variant="outline" disabled={pageInfo.page <= 1} onClick={() => setPage(pageInfo.page - 1)}>
            Previous
          </Button>
          <span className="text-caption text-muted-foreground">Page {pageInfo.page} of {pageInfo.totalPages}</span>
          <Button
            size="sm"
            variant="outline"
            disabled={pageInfo.page >= pageInfo.totalPages}
            onClick={() => setPage(pageInfo.page + 1)}
          >
            Next
          </Button>
        </div>
      ) : null}

      <CreateSquadModal
        open={createOpen}
        onOpenChange={setCreateOpen}
        agents={agents.data ?? []}
        onCreated={refresh}
      />
    </PageFrame>
  );
}

type SquadDetailTab = "profile" | "members" | "instructions";

export function SquadDetailPage({ id }: { id: string }) {
  const workspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const user = useAuthStore((state) => state.user);
  const key = lightweightKeys.squads(workspace?.id ?? "");

  const squad = useQuery({ queryKey: [...key, id], queryFn: () => lightweightApi.getSquad(id), enabled: !!workspace && !!id });
  const members = useQuery({ queryKey: [...key, id, "members"], queryFn: () => lightweightApi.listSquadMembers(id), enabled: !!workspace && !!id });
  const statuses = useQuery({
    queryKey: [...key, id, "status"],
    queryFn: () => lightweightApi.listSquadMemberStatus(id),
    enabled: !!workspace && !!id,
    refetchInterval: 10000,
  });
  const agents = useQuery({ queryKey: lightweightKeys.agents(workspace?.id ?? ""), queryFn: lightweightApi.listAgents, enabled: !!workspace });
  const workspaceMembers = useQuery({
    queryKey: lightweightKeys.members(workspace?.id ?? ""),
    queryFn: () => lightweightApi.listMembers(workspace!.id),
    enabled: !!workspace,
  });

  const [tab, setTab] = useState<SquadDetailTab>("profile");
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [instructions, setInstructions] = useState("");
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const [dirtyPrompt, setDirtyPrompt] = useState<null | { action: () => void }>(null);

  useEffect(() => {
    if (!squad.data) return;
    setName(squad.data.name);
    setDescription(squad.data.description);
    setInstructions(squad.data.instructions);
    setAvatarPreview(squad.data.avatar_url);
    setAvatarFile(null);
  }, [squad.data]);

  const memberRole = workspaceMembers.data?.find((member) => member.user_id === user?.id)?.role ?? null;
  const canManage =
    memberRole === "owner" ||
    memberRole === "admin" ||
    (!!user?.id && squad.data?.creator_id === user.id);

  const profileDirty =
    squad.data &&
    (name !== squad.data.name ||
      description !== squad.data.description ||
      avatarFile !== null);
  const instructionsDirty = squad.data && instructions !== squad.data.instructions;
  const isDirty = profileDirty || instructionsDirty;

  useEffect(() => {
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!isDirty) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => window.removeEventListener("beforeunload", handleBeforeUnload);
  }, [isDirty]);

  const refresh = () => void queryClient.invalidateQueries({ queryKey: key });

  const update = useMutation({
    mutationFn: async () => {
      await lightweightApi.updateSquad(id, {
        name: name.trim(),
        description,
        instructions: tab === "instructions" ? instructions : squad.data?.instructions,
      });
      if (avatarFile) {
        await lightweightApi.uploadSquadAvatar(id, avatarFile, avatarFile.name);
      }
    },
    onSuccess: refresh,
  });

  const saveInstructions = useMutation({
    mutationFn: () => lightweightApi.updateSquad(id, { instructions }),
    onSuccess: refresh,
  });

  const add = useMutation({ mutationFn: (agentId: string) => lightweightApi.addSquadMember(id, agentId, "member"), onSuccess: refresh });
  const role = useMutation({
    mutationFn: (value: { agentId: string; role: "leader" | "member" }) =>
      lightweightApi.setSquadMemberRole(id, value.agentId, value.role),
    onSuccess: refresh,
  });
  const removeMember = useMutation({ mutationFn: (agentId: string) => lightweightApi.removeSquadMember(id, agentId), onSuccess: refresh });
  const archive = useMutation({
    mutationFn: () => lightweightApi.archiveSquad(id),
    onSuccess: () => navigation.push(paths.squads()),
  });

  const requestNavigation = (action: () => void) => {
    if (!isDirty) {
      action();
      return;
    }
    setDirtyPrompt({ action });
  };

  const switchTab = (next: SquadDetailTab) => {
    requestNavigation(() => setTab(next));
  };

  if (!squad.data) {
    return (
      <PageFrame title="Squad">
        <ErrorState error={squad.error} />
        {squad.isPending ? <p>Loading…</p> : null}
      </PageFrame>
    );
  }

  const statusByAgent = new Map(statuses.data?.members.map((item) => [item.agent_id, item]));
  const memberIds = new Set(members.data?.map((item) => item.agent_id));
  const creatorName =
    workspaceMembers.data?.find((member) => member.user_id === squad.data.creator_id)?.name ?? squad.data.creator_id;

  return (
    <PageFrame
      title={squad.data.name}
      description="Leader changes are atomic; the current leader cannot be removed directly."
      action={
        canManage && !squad.data.archived_at ? (
          <Button variant="destructive" disabled={archive.isPending} onClick={() => archive.mutate()}>
            Archive
          </Button>
        ) : squad.data.archived_at ? (
          <Badge variant="outline">Archived</Badge>
        ) : null
      }
    >
      <div className="flex flex-wrap gap-2">
        {(["profile", "members", "instructions"] as SquadDetailTab[]).map((item) => (
          <Button key={item} size="sm" variant={tab === item ? "default" : "outline"} onClick={() => switchTab(item)}>
            {item === "profile" ? "Profile" : item === "members" ? "Members" : "Instructions"}
          </Button>
        ))}
        <Button
          size="sm"
          variant="ghost"
          onClick={() => requestNavigation(() => navigation.push(paths.squads()))}
        >
          Back to list
        </Button>
      </div>

      {tab === "profile" ? (
        <Panel>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (!canManage || squad.data.archived_at) return;
              void update.mutateAsync();
            }}
          >
            <div className="flex items-center gap-4">
              <SquadAvatar name={name} url={avatarPreview} size="md" />
              {canManage && !squad.data.archived_at ? (
                <Input
                  type="file"
                  accept="image/png,image/jpeg"
                  onChange={(event) => setAvatarFile(event.target.files?.[0] ?? null)}
                />
              ) : null}
            </div>
            <label className="grid gap-2 text-body">
              <span className="font-medium">Name</span>
              <Input value={name} onChange={(event) => setName(event.target.value)} required disabled={!canManage || !!squad.data.archived_at} />
            </label>
            <label className="grid gap-2 text-body">
              <span className="font-medium">Description</span>
              <Textarea
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                rows={3}
                disabled={!canManage || !!squad.data.archived_at}
              />
            </label>
            <div className="grid gap-1 text-body text-muted-foreground">
              <p>Creator: {creatorName}</p>
              <p>Members: {squad.data.member_count}</p>
              <p>Created: {new Date(squad.data.created_at).toLocaleString()}</p>
              <p>Updated: {new Date(squad.data.updated_at).toLocaleString()}</p>
            </div>
            {canManage && !squad.data.archived_at ? (
              <>
                <ErrorState error={update.error ?? archive.error} />
                <SubmitButton pending={update.isPending}>Save profile</SubmitButton>
              </>
            ) : null}
          </form>
        </Panel>
      ) : null}

      {tab === "instructions" ? (
        <Panel>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (!canManage || squad.data.archived_at) return;
              void saveInstructions.mutateAsync();
            }}
          >
            <Textarea
              value={instructions}
              onChange={(event) => setInstructions(event.target.value)}
              rows={10}
              disabled={!canManage || !!squad.data.archived_at}
            />
            <p className="text-caption text-muted-foreground">
              Saved instructions are injected into Leader task context on the next Daemon claim.
            </p>
            {canManage && !squad.data.archived_at ? (
              <>
                <ErrorState error={saveInstructions.error} />
                <SubmitButton pending={saveInstructions.isPending}>Save instructions</SubmitButton>
              </>
            ) : null}
          </form>
        </Panel>
      ) : null}

      {tab === "members" ? (
        <Panel>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <h2 className="font-semibold">Agents and live status</h2>
            {canManage && !squad.data.archived_at ? (
              <AppLink
                href={`${paths.newAgentBlank()}?from_squad=${encodeURIComponent(id)}`}
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                Create agent
              </AppLink>
            ) : null}
          </div>
          <div className="mt-4 grid gap-2">
            {members.data?.map((member) => {
              const status = statusByAgent.get(member.agent_id);
              return (
                <div key={member.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3 text-body">
                  <div>
                    <strong>{member.name}</strong>
                    <div className="text-caption text-muted-foreground">
                      {member.role} · {status?.status ?? "unknown"}
                      {status?.last_active_at ? ` · last active ${new Date(status.last_active_at).toLocaleString()}` : ""}
                    </div>
                    {status?.active_issue_id && status.active_issue_title ? (
                      <AppLink href={paths.issueDetail(status.active_issue_id)} className="text-caption text-primary hover:underline">
                        Active: {status.active_issue_title}
                      </AppLink>
                    ) : null}
                  </div>
                  {canManage && !squad.data.archived_at ? (
                    <div className="flex gap-2">
                      {member.role !== "leader" ? (
                        <Button size="sm" variant="outline" onClick={() => role.mutate({ agentId: member.agent_id, role: "leader" })}>
                          Promote
                        </Button>
                      ) : null}
                      {member.role !== "leader" ? (
                        <Button size="sm" variant="ghost" onClick={() => removeMember.mutate(member.agent_id)}>
                          Remove
                        </Button>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
          {canManage && !squad.data.archived_at ? (
            <form
              className="mt-4 flex gap-2"
              onSubmit={(event) => {
                event.preventDefault();
                const form = event.currentTarget;
                const agentId = (form.elements.namedItem("agent_id") as HTMLSelectElement).value;
                if (!agentId) return;
                void add.mutateAsync(agentId);
                form.reset();
              }}
            >
              <select name="agent_id" required className="h-9 flex-1 rounded-md border bg-background px-3 text-body">
                <option value="">Add agent</option>
                {agents.data
                  ?.filter((agent) => !agent.archived_at && !memberIds.has(agent.id))
                  .map((agent) => (
                    <option key={agent.id} value={agent.id}>{agent.name}</option>
                  ))}
              </select>
              <SubmitButton pending={add.isPending}>Add</SubmitButton>
            </form>
          ) : null}
          <ErrorState error={add.error ?? role.error ?? removeMember.error} />
        </Panel>
      ) : null}

      {dirtyPrompt !== null ? (
        <ModalFrame
          title="Unsaved changes"
          description="Save your edits, discard them, or stay on this page."
          onClose={() => setDirtyPrompt(null)}
        >
          <div className="flex flex-wrap justify-end gap-2">
            <Button variant="outline" onClick={() => setDirtyPrompt(null)}>Cancel</Button>
            <Button
              variant="outline"
              onClick={() => {
                if (!squad.data) return;
                setName(squad.data.name);
                setDescription(squad.data.description);
                setInstructions(squad.data.instructions);
                setAvatarFile(null);
                setAvatarPreview(squad.data.avatar_url);
                const action = dirtyPrompt?.action;
                setDirtyPrompt(null);
                action?.();
              }}
            >
              Discard
            </Button>
            <Button
              onClick={async () => {
                if (instructionsDirty) await saveInstructions.mutateAsync();
                if (profileDirty) await update.mutateAsync();
                const action = dirtyPrompt?.action;
                setDirtyPrompt(null);
                action?.();
              }}
            >
              Save and continue
            </Button>
          </div>
        </ModalFrame>
      ) : null}
    </PageFrame>
  );
}
