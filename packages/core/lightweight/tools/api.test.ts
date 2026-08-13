import { beforeEach, describe, expect, it, vi } from "vitest";

import { setApiInstance, type ApiClient } from "../../api";
import { lightweightApi } from "../api";

const request = vi.fn();

describe("Agent ToolBundle API", () => {
  beforeEach(() => {
    request.mockReset().mockResolvedValue({
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
    });
    setApiInstance({ request } as unknown as ApiClient);
  });

  it("publishes Definition identity and exported name as strict items", async () => {
    const bundle = await lightweightApi.publishAgentToolBundle("agent-1", [{
      toolDefinitionId: "tool-1",
      exportedName: "skill.kratos-user.get_user",
    }]);

    expect(bundle.items[0]).toMatchObject({
      exportedName: "skill.kratos-user.get_user",
      canonicalPublicName: "kratos-demo-http.User_GetUser",
    });
    const [path, options] = request.mock.calls[0] as [string, RequestInit];
    expect(path).toBe("/api/agents/agent-1/tool-bundle");
    expect(options.method).toBe("PUT");
    expect(JSON.parse(String(options.body))).toEqual({
      items: [{
        tool_definition_id: "tool-1",
        exported_name: "skill.kratos-user.get_user",
      }],
    });
  });
});
