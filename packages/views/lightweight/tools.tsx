"use client";

import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@dars/core/auth";
import {
  lightweightApi,
  lightweightKeys,
  toolDefinitionsOptions,
  toolKeys,
  toolSourceOptions,
  toolSourcesOptions,
  type ToolSource,
} from "@dars/core/lightweight";
import { useCurrentWorkspace, useWorkspacePaths } from "@dars/core/paths";
import { Badge } from "@dars/ui/components/ui/badge";
import { Button } from "@dars/ui/components/ui/button";
import { Input } from "@dars/ui/components/ui/input";
import { Label } from "@dars/ui/components/ui/label";
import { AppLink, useNavigation } from "../navigation";
import { EmptyState, ErrorState, PageFrame, Panel, SubmitButton } from "./components";

type ImportMode = "openapi-file" | "openapi-url" | "grpc-file" | "remote-mcp";

function formatFileSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KiB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MiB`;
}

function useToolAdminAccess(workspaceId: string) {
  const user = useAuthStore((state) => state.user);
  const members = useQuery({
    queryKey: lightweightKeys.members(workspaceId),
    queryFn: () => lightweightApi.listMembers(workspaceId),
    enabled: !!workspaceId,
  });
  const role = members.data?.find((member) => member.user_id === user?.id)?.role ?? null;
  return { members, allowed: role === "owner" || role === "admin" };
}

function SourceStatus({ source }: { source: ToolSource }) {
  const status = source.revision?.status ?? (source.currentRevision ? "ready" : "validating");
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge variant={status === "failed" ? "destructive" : "outline"}>{status}</Badge>
      <Badge variant={source.enabled ? "secondary" : "outline"}>{source.enabled ? "enabled" : "disabled"}</Badge>
      <Badge variant="outline">{source.kind}</Badge>
    </div>
  );
}

function SourceImportForm({ workspaceId }: { workspaceId: string }) {
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<ImportMode>("openapi-file");
  const [file, setFile] = useState<File | null>(null);

  const create = useMutation({
    mutationFn: async (form: HTMLFormElement) => {
      const values = new FormData(form);
      const name = String(values.get("name") ?? "").trim();
      const endpoint = String(values.get("endpoint") ?? "").trim();
      const token = String(values.get("token") ?? "").trim();
      const auth = token ? { kind: "bearer", token } : undefined;
      if (mode === "openapi-file" || mode === "grpc-file") {
        if (!file) throw new Error("Choose a file to import");
        const imported = await lightweightApi.importToolSource({
          name,
          kind: mode === "grpc-file" ? "grpc" : "openapi",
          endpoint,
          file,
          filename: file.name,
          mediaType: file.type || undefined,
          auth,
        });
        return imported.source;
      }
      const documentUrl = String(values.get("document_url") ?? "").trim();
      const staged = await lightweightApi.createToolSource({
        name,
        kind: mode === "remote-mcp" ? "remote_mcp" : "openapi",
        endpoint,
        transportConfig: mode === "openapi-url" ? { document_url: documentUrl } : {},
        auth,
      });
      if (!staged.revision) throw new Error("Server did not return a staged revision");
      return lightweightApi.validateToolSource(staged.id, staged.revision.id);
    },
    onSuccess: (source) => {
      void queryClient.invalidateQueries({ queryKey: toolKeys.sources(workspaceId) });
      navigation.push(paths.toolDetail(source.id));
    },
  });

  return (
    <Panel>
      <h2 className="font-semibold">Import Tool Source</h2>
      <p className="mt-1 text-body text-muted-foreground">
        Files are parsed, converted, validated, and stored by the Server. The browser never executes imported definitions.
      </p>
      <form
        className="mt-4 grid gap-4"
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault();
          create.mutate(event.currentTarget);
        }}
      >
        <div className="grid gap-2">
          <Label htmlFor="source-mode">Format</Label>
          <select
            id="source-mode"
            className="h-9 rounded-md border bg-background px-3 text-body"
            value={mode}
            onChange={(event) => {
              setMode(event.target.value as ImportMode);
              setFile(null);
            }}
          >
            <option value="openapi-file">OpenAPI 3 / Swagger 2 file</option>
            <option value="openapi-url">OpenAPI 3 / Swagger 2 URL</option>
            <option value="grpc-file">gRPC Proto / ZIP / descriptor set</option>
            <option value="remote-mcp">Remote MCP (Streamable HTTP)</option>
          </select>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="source-name">Canonical namespace</Label>
          <Input id="source-name" name="name" required placeholder="customer-platform" />
          <p className="text-caption text-muted-foreground">Used to generate immutable catalog names. Agent-specific call names are configured from Agent Capabilities &gt; Tools.</p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="source-endpoint">
            {mode === "remote-mcp" ? "Remote MCP endpoint" : mode === "grpc-file" ? "gRPC endpoint" : "API base endpoint"}
          </Label>
          <Input id="source-endpoint" name="endpoint" type="url" required placeholder="https://api.example.com" />
        </div>
        {mode === "openapi-url" ? (
          <div className="grid gap-2">
            <Label htmlFor="document-url">Swagger / OpenAPI document URL</Label>
            <Input id="document-url" name="document_url" type="url" required placeholder="https://api.example.com/openapi.yaml" />
          </div>
        ) : null}
        {mode === "openapi-file" || mode === "grpc-file" ? (
          <div className="grid gap-2">
            <Label htmlFor="source-file">Artifact</Label>
            <Input
              id="source-file"
              type="file"
              required
              accept={mode === "grpc-file" ? ".proto,.zip,.pb,.bin" : ".json,.yaml,.yml"}
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
            />
            {file ? <p className="text-caption text-muted-foreground">Selected: {file.name} · {formatFileSize(file.size)}</p> : null}
          </div>
        ) : null}
        <div className="grid gap-2">
          <Label htmlFor="source-token">Bearer token (optional)</Label>
          <Input id="source-token" name="token" type="password" autoComplete="new-password" />
          <p className="text-caption text-muted-foreground">Stored encrypted by the Server and never shown again.</p>
        </div>
        <ErrorState error={create.error} />
        <SubmitButton pending={create.isPending}>Import and validate</SubmitButton>
      </form>
    </Panel>
  );
}

export function ToolsPage() {
  const workspace = useCurrentWorkspace();
  const workspaceId = workspace?.id ?? "";
  const paths = useWorkspacePaths();
  const access = useToolAdminAccess(workspaceId);
  const sources = useQuery({ ...toolSourcesOptions(workspaceId), enabled: !!workspaceId && access.allowed });

  if (access.members.isPending) return <PageFrame title="Tools"><p>Loading…</p></PageFrame>;
  if (!access.allowed) {
    return (
      <PageFrame title="Tools">
        <ErrorState error={access.members.error ?? new Error("Workspace owner or admin permission is required")} />
      </PageFrame>
    );
  }

  return (
    <PageFrame title="Tools" description="Import and manage Server-hosted MCP tools for this workspace.">
      <SourceImportForm workspaceId={workspaceId} />
      <Panel>
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="font-semibold">Tool Sources</h2>
            <p className="mt-1 text-body text-muted-foreground">Ready Sources can be selected from an Agent&apos;s Capabilities.</p>
          </div>
          <Button variant="outline" size="sm" onClick={() => void sources.refetch()}>Refresh</Button>
        </div>
        <ErrorState error={sources.error} />
        <div className="mt-4 grid gap-3">
          {(sources.data ?? []).map((source) => (
            <AppLink key={source.id} href={paths.toolDetail(source.id)} className="rounded-lg border p-4 hover:bg-muted/40">
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div>
                  <div className="font-medium">{source.name}</div>
                  <div className="mt-1 text-caption text-muted-foreground">
                    Revision {source.revision?.revision ?? "—"} · Updated {new Date(source.updatedAt).toLocaleString()}
                  </div>
                </div>
                <SourceStatus source={source} />
              </div>
              {source.revision?.validationCode ? (
                <p className="mt-2 text-caption text-destructive">{source.revision.validationCode}</p>
              ) : null}
            </AppLink>
          ))}
          {!sources.isPending && (sources.data?.length ?? 0) === 0 ? <EmptyState>No Tool Sources yet.</EmptyState> : null}
        </div>
      </Panel>
    </PageFrame>
  );
}

export function ToolSourceDetailPage({ id }: { id: string }) {
  const workspace = useCurrentWorkspace();
  const workspaceId = workspace?.id ?? "";
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const access = useToolAdminAccess(workspaceId);
  const source = useQuery({ ...toolSourceOptions(workspaceId, id), enabled: !!workspaceId && access.allowed });
  const tools = useQuery({
    ...toolDefinitionsOptions(workspaceId, id),
    enabled: !!workspaceId && access.allowed && !!source.data?.currentRevision,
  });
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: toolKeys.source(workspaceId, id) });
    void queryClient.invalidateQueries({ queryKey: toolKeys.definitions(workspaceId, id) });
    void queryClient.invalidateQueries({ queryKey: toolKeys.sources(workspaceId) });
  };
  const validate = useMutation({
    mutationFn: () => {
      if (!source.data?.revision) throw new Error("No staged revision to validate");
      return lightweightApi.validateToolSource(id, source.data.revision.id);
    },
    onSuccess: refresh,
  });
  const toggle = useMutation({
    mutationFn: (enabled: boolean) => enabled ? lightweightApi.enableToolSource(id) : lightweightApi.disableToolSource(id),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: () => lightweightApi.deleteToolSource(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: toolKeys.sources(workspaceId) });
      navigation.push(paths.tools());
    },
  });
  const update = useMutation({
    mutationFn: async ({ form, file }: { form: HTMLFormElement; file: File | null }) => {
      if (!source.data) throw new Error("Source is not loaded");
      const values = new FormData(form);
      const endpoint = String(values.get("endpoint") ?? "").trim();
      const documentUrl = String(values.get("document_url") ?? "").trim();
      const token = String(values.get("token") ?? "").trim();
      const auth = token ? { kind: "bearer", token } : undefined;
      if (source.data.kind === "grpc" && !file) throw new Error("Choose a Proto, ZIP, or descriptor set artifact");
      if (source.data.kind === "openapi" && !file && !documentUrl) throw new Error("Choose a document file or enter a document URL");
      if (file && (source.data.kind === "openapi" || source.data.kind === "grpc")) {
        return (await lightweightApi.importToolSource({
          sourceId: source.data.id,
          name: source.data.name,
          kind: source.data.kind,
          endpoint,
          file,
          filename: file.name,
          mediaType: file.type || undefined,
          auth,
        })).source;
      }
      const staged = await lightweightApi.updateToolSource(id, {
        endpoint,
        transportConfig: source.data.kind === "openapi" && documentUrl ? { document_url: documentUrl } : {},
        auth,
      });
      if (!staged.revision) throw new Error("Server did not return a staged revision");
      return lightweightApi.validateToolSource(id, staged.revision.id);
    },
    onSuccess: refresh,
  });
  const [revisionFile, setRevisionFile] = useState<File | null>(null);

  if (access.members.isPending || source.isPending) return <PageFrame title="Tool Source"><p>Loading…</p></PageFrame>;
  if (!access.allowed) return <PageFrame title="Tool Source"><ErrorState error={new Error("Workspace owner or admin permission is required")} /></PageFrame>;
  if (!source.data) return <PageFrame title="Tool Source"><ErrorState error={source.error ?? new Error("Tool Source not found")} /></PageFrame>;
  const value = source.data;
  const updateAvailable = value.revision?.status === "ready" && value.currentRevision !== value.revision.id;

  return (
    <PageFrame
      title={value.name}
      description="Server-side Source revision, validation, and discovered tool catalog."
      action={<AppLink href={paths.tools()} className="text-body text-primary underline">Back to Tools</AppLink>}
    >
      <Panel>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <SourceStatus source={value} />
            <p className="mt-3 text-body text-muted-foreground">{value.revision?.endpoint ?? "No endpoint"}</p>
            <p className="mt-1 text-caption text-muted-foreground">
              Current revision: {value.currentRevision ?? "none"} · Latest: {value.revision?.id ?? "none"}
            </p>
            <p className="mt-1 text-caption text-muted-foreground">
              Artifact: {value.revision?.artifactId ?? "none"} · Credential: {value.revision?.secretConfigured ? "configured" : "not configured"}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            {value.revision?.status === "validating" ? (
              <Button size="sm" onClick={() => validate.mutate()} disabled={validate.isPending}>Validate</Button>
            ) : null}
            {value.currentRevision ? (
              <Button size="sm" variant="outline" onClick={() => toggle.mutate(!value.enabled)} disabled={toggle.isPending}>
                {value.enabled ? "Disable" : "Enable"}
              </Button>
            ) : null}
            <Button
              size="sm"
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => {
                if (globalThis.confirm("Delete this Tool Source? Retained Bundles may prevent deletion.")) remove.mutate();
              }}
            >Delete</Button>
          </div>
        </div>
        {updateAvailable ? <p className="mt-3 text-body text-amber-700">A newer revision is ready. Existing Agent Bundles remain pinned until republished.</p> : null}
        {value.revision?.validationCode ? <p className="mt-3 text-body text-destructive">{value.revision.validationCode}</p> : null}
        <ErrorState error={validate.error ?? toggle.error ?? remove.error} />
      </Panel>

      <Panel>
        <h2 className="font-semibold">Publish a new revision</h2>
        <p className="mt-1 text-body text-muted-foreground">Successful updates never rewrite existing Agent Bundles.</p>
        <form
          className="mt-4 grid gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            update.mutate({ form: event.currentTarget, file: revisionFile });
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor="revision-endpoint">Endpoint</Label>
            <Input id="revision-endpoint" name="endpoint" type="url" required defaultValue={value.revision?.endpoint ?? ""} />
          </div>
          {value.kind === "openapi" ? (
            <div className="grid gap-2">
              <Label htmlFor="revision-document-url">New document URL (when no file is selected)</Label>
              <Input id="revision-document-url" name="document_url" type="url" />
            </div>
          ) : null}
          {value.kind === "openapi" || value.kind === "grpc" ? (
            <div className="grid gap-2">
              <Label htmlFor="revision-file">New artifact (optional for OpenAPI URL)</Label>
              <Input id="revision-file" type="file" required={value.kind === "grpc"} accept={value.kind === "grpc" ? ".proto,.zip,.pb,.bin" : ".json,.yaml,.yml"} onChange={(event) => setRevisionFile(event.target.files?.[0] ?? null)} />
              {revisionFile ? <p className="text-caption text-muted-foreground">Selected: {revisionFile.name} · {formatFileSize(revisionFile.size)}</p> : null}
            </div>
          ) : null}
          <div className="grid gap-2">
            <Label htmlFor="revision-token">Replace Bearer token (optional)</Label>
            <Input id="revision-token" name="token" type="password" autoComplete="new-password" />
          </div>
          <ErrorState error={update.error} />
          <SubmitButton pending={update.isPending}>Validate and publish revision</SubmitButton>
        </form>
      </Panel>

      <Panel>
        <h2 className="font-semibold">Discovered Tools</h2>
        <ErrorState error={tools.error} />
        <div className="mt-4 grid gap-3">
          {(tools.data ?? []).map((tool) => (
            <div key={tool.id} className="rounded-lg border p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <strong>Canonical MCP name: {tool.publicName}</strong>
                <Badge variant={tool.enabled ? "secondary" : "outline"}>{tool.enabled ? "enabled" : "disabled"}</Badge>
              </div>
              <p className="mt-1 text-body text-muted-foreground">{tool.description || "No description"}</p>
              <p className="mt-1 text-caption text-muted-foreground">Upstream operation: {tool.upstreamName}</p>
              <p className="mt-1 text-caption text-muted-foreground">To customize the Agent call name, open that Agent&apos;s Capabilities &gt; Tools.</p>
            </div>
          ))}
          {!tools.isPending && (tools.data?.length ?? 0) === 0 ? <EmptyState>No ready tools discovered.</EmptyState> : null}
        </div>
      </Panel>
    </PageFrame>
  );
}
