import { z } from "zod";
import { parseWithFallback } from "../../api/schema";
import type {
  AgentToolBundle,
  ToolBundle,
  ToolDefinition,
  ToolSource,
  ToolSourceImportResult,
} from "./types";

const objectSchema = z.record(z.string(), z.unknown());

const revisionWireSchema = z.object({
  id: z.string(),
  revision: z.number(),
  status: z.enum(["validating", "ready", "failed", "disabled"]),
  endpoint: z.string().optional(),
  transport_config: objectSchema,
  artifact_id: z.string().optional(),
  secret_configured: z.boolean(),
  secret_key_id: z.string().optional(),
  validation_code: z.string().optional(),
  created_at: z.string(),
  published_at: z.string().optional(),
}).transform((value) => ({
  id: value.id,
  revision: value.revision,
  status: value.status,
  endpoint: value.endpoint ?? null,
  transportConfig: value.transport_config,
  artifactId: value.artifact_id ?? null,
  secretConfigured: value.secret_configured,
  secretKeyId: value.secret_key_id ?? null,
  validationCode: value.validation_code ?? null,
  createdAt: value.created_at,
  publishedAt: value.published_at ?? null,
}));

export const toolSourceWireSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  name: z.string(),
  kind: z.enum(["openapi", "grpc", "remote_mcp", "server_local"]),
  enabled: z.boolean(),
  current_revision: z.string().optional(),
  revision: revisionWireSchema.optional(),
  created_at: z.string(),
  updated_at: z.string(),
}).transform((value): ToolSource => ({
  id: value.id,
  workspaceId: value.workspace_id,
  name: value.name,
  kind: value.kind,
  enabled: value.enabled,
  currentRevision: value.current_revision ?? null,
  revision: value.revision ?? null,
  createdAt: value.created_at,
  updatedAt: value.updated_at,
}));

export const toolDefinitionWireSchema = z.object({
  id: z.string(),
  public_name: z.string(),
  upstream_name: z.string(),
  description: z.string(),
  input_schema: objectSchema,
  output_schema: objectSchema.optional(),
  operation_metadata: objectSchema,
  enabled: z.boolean(),
}).transform((value): ToolDefinition => ({
  id: value.id,
  publicName: value.public_name,
  upstreamName: value.upstream_name,
  description: value.description,
  inputSchema: value.input_schema,
  outputSchema: value.output_schema ?? null,
  operationMetadata: value.operation_metadata,
  enabled: value.enabled,
}));

const bundleItemWireSchema = z.object({
  ordinal: z.number(),
  exported_name: z.string(),
  canonical_public_name: z.string(),
  source_id: z.string(),
  source_revision_id: z.string(),
  tool_definition_id: z.string(),
  definition: objectSchema,
}).transform((value) => ({
  ordinal: value.ordinal,
  exportedName: value.exported_name,
  canonicalPublicName: value.canonical_public_name,
  sourceId: value.source_id,
  sourceRevisionId: value.source_revision_id,
  toolDefinitionId: value.tool_definition_id,
  definition: value.definition,
}));

export const toolBundleWireSchema = z.object({
  id: z.string(),
  workspace_id: z.string(),
  manifest_hash: z.string(),
  status: z.enum(["active", "revoked"]),
  items: z.array(bundleItemWireSchema),
  created_at: z.string(),
  revoked_at: z.string().optional(),
}).transform((value): ToolBundle => ({
  id: value.id,
  workspaceId: value.workspace_id,
  manifestHash: value.manifest_hash,
  status: value.status,
  items: value.items,
  createdAt: value.created_at,
  revokedAt: value.revoked_at ?? null,
}));

const agentToolBundleWireSchema = z.object({ bundle: toolBundleWireSchema.nullable() });
const artifactWireSchema = z.object({
  id: z.string(), sha256: z.string(), media_type: z.string(), size_bytes: z.number(),
}).transform((value) => ({
  id: value.id, sha256: value.sha256, mediaType: value.media_type, sizeBytes: value.size_bytes,
}));
const importResultWireSchema = z.object({
  source: toolSourceWireSchema,
  tools: z.array(toolDefinitionWireSchema),
  artifact: artifactWireSchema,
});

export function parseToolSources(value: unknown): ToolSource[] {
  return parseWithFallback(z.array(toolSourceWireSchema), value, []);
}

export function parseToolSource(value: unknown): ToolSource | null {
  return parseWithFallback(toolSourceWireSchema.nullable(), value, null);
}

export function parseToolDefinitions(value: unknown): ToolDefinition[] {
  return parseWithFallback(z.array(toolDefinitionWireSchema), value, []);
}

export function parseToolBundle(value: unknown): ToolBundle | null {
  return parseWithFallback(toolBundleWireSchema.nullable(), value, null);
}

export function parseAgentToolBundle(value: unknown): AgentToolBundle {
  return parseWithFallback(agentToolBundleWireSchema, value, { bundle: null });
}

export function parseToolSourceImportResult(value: unknown): ToolSourceImportResult | null {
  return parseWithFallback(importResultWireSchema.nullable(), value, null);
}
