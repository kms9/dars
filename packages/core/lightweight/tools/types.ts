export type ToolSourceKind = "openapi" | "grpc" | "remote_mcp" | "server_local";
export type ToolSourceStatus = "validating" | "ready" | "failed" | "disabled";

export type ToolSourceRevision = {
  id: string;
  revision: number;
  status: ToolSourceStatus;
  endpoint: string | null;
  transportConfig: Record<string, unknown>;
  artifactId: string | null;
  secretConfigured: boolean;
  secretKeyId: string | null;
  validationCode: string | null;
  createdAt: string;
  publishedAt: string | null;
};

export type ToolSource = {
  id: string;
  workspaceId: string;
  name: string;
  kind: ToolSourceKind;
  enabled: boolean;
  currentRevision: string | null;
  revision: ToolSourceRevision | null;
  createdAt: string;
  updatedAt: string;
};

export type ToolDefinition = {
  id: string;
  publicName: string;
  upstreamName: string;
  description: string;
  inputSchema: Record<string, unknown>;
  outputSchema: Record<string, unknown> | null;
  operationMetadata: Record<string, unknown>;
  enabled: boolean;
};

export type ToolBundleItem = {
  ordinal: number;
  exportedName: string;
  canonicalPublicName: string;
  sourceId: string;
  sourceRevisionId: string;
  toolDefinitionId: string;
  definition: Record<string, unknown>;
};

export type ToolBundle = {
  id: string;
  workspaceId: string;
  manifestHash: string;
  status: "active" | "revoked";
  items: ToolBundleItem[];
  createdAt: string;
  revokedAt: string | null;
};

export type AgentToolBundle = { bundle: ToolBundle | null };

export type ToolBundleSelectionItem = {
  toolDefinitionId: string;
  exportedName: string;
};

export type ToolSourceArtifact = {
  id: string;
  sha256: string;
  mediaType: string;
  sizeBytes: number;
};

export type ToolSourceImportResult = {
  source: ToolSource;
  tools: ToolDefinition[];
  artifact: ToolSourceArtifact;
};

export type CreateToolSourceInput = {
  name: string;
  kind: Exclude<ToolSourceKind, "server_local">;
  endpoint?: string;
  transportConfig?: Record<string, unknown>;
  auth?: Record<string, unknown>;
};

export type ImportToolSourceInput = {
  sourceId?: string;
  name: string;
  kind: "openapi" | "grpc";
  endpoint: string;
  file: File | Blob;
  filename: string;
  mediaType?: string;
  transportConfig?: Record<string, unknown>;
  auth?: Record<string, unknown>;
};
