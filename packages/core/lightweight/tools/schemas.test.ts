import { describe, expect, it } from "vitest";
import { parseAgentToolBundle, parseToolSources } from "./schemas";

describe("Tool Control Plane response schemas", () => {
  it("maps wire fields to camelCase", () => {
    const sources = parseToolSources([{
      id: "source-1",
      workspace_id: "workspace-1",
      name: "pet-api",
      kind: "openapi",
      enabled: true,
      current_revision: "revision-1",
      revision: {
        id: "revision-1",
        revision: 1,
        status: "ready",
        endpoint: "https://example.com/api",
        transport_config: {},
        secret_configured: false,
        created_at: "2026-08-11T00:00:00Z",
        published_at: "2026-08-11T00:00:00Z",
      },
      created_at: "2026-08-11T00:00:00Z",
      updated_at: "2026-08-11T00:00:00Z",
    }]);
    expect(sources[0]).toMatchObject({
      workspaceId: "workspace-1",
      currentRevision: "revision-1",
      revision: { transportConfig: {}, secretConfigured: false },
    });
  });

  it("falls back safely for malformed list and Agent Bundle responses", () => {
    expect(parseToolSources([{ id: 42 }])).toEqual([]);
    expect(parseAgentToolBundle({ bundle: { id: "partial" } })).toEqual({ bundle: null });
  });

  it("keeps exported and canonical Bundle names distinct", () => {
    const parsed = parseAgentToolBundle({
      bundle: {
        id: "tb_alias",
        workspace_id: "workspace-1",
        manifest_hash: "hash",
        status: "active",
        items: [{
          ordinal: 0,
          exported_name: "skill.kratos-user.get_user",
          canonical_public_name: "kratos-demo-http.User_GetUser",
          source_id: "source-1",
          source_revision_id: "revision-1",
          tool_definition_id: "tool-1",
          definition: {},
        }],
        created_at: "2026-08-11T00:00:00Z",
      },
    });

    expect(parsed.bundle?.items[0]).toMatchObject({
      exportedName: "skill.kratos-user.get_user",
      canonicalPublicName: "kratos-demo-http.User_GetUser",
    });
  });
});
