import { describe, expect, it } from "vitest";
import { decodeBuilderInput, encodeBuilderInput, mergeBuilderDraft, parseBuilderDraft, stripBuilderDraft } from "./builder-protocol";
import type { StoredAgentDraft } from "./types";

const draft = (): StoredAgentDraft => ({
  name: "Old name",
  description: "Old description",
  instructions: "Old instructions",
  avatar_url: null,
  model: "model-1",
  thinking_level: "",
  service_tier: "",
  skill_ids: ["skill-1"],
  permission_scope: "private",
  member_ids: [],
  max_concurrent_tasks: 1,
});

describe("builder protocol", () => {
  it("parses and hides structured draft blocks", () => {
    const content = 'Here is a draft.\n<agent_draft>{"name":"Researcher","permission_scope":"workspace"}</agent_draft>';
    expect(parseBuilderDraft(content)).toEqual({ name: "Researcher", permission_scope: "workspace" });
    expect(stripBuilderDraft(content)).toBe("Here is a draft.");
  });

  it("round-trips builder input for chat display", () => {
    const encoded = encodeBuilderInput("Create a release manager", draft());
    expect(decodeBuilderInput(encoded)).toBe("Create a release manager");
  });

  it("rejects unknown workspace references when merging", () => {
    const result = mergeBuilderDraft(
      draft(),
      {
        name: "Release manager",
        skill_ids: ["skill-2", "unknown"],
        permission_scope: "members",
        member_ids: ["member-1", "unknown"],
      },
      new Set(["skill-1", "skill-2"]),
      new Set(["member-1"]),
    );
    expect(result.name).toBe("Release manager");
    expect(result.skill_ids).toEqual(["skill-2"]);
    expect(result.member_ids).toEqual(["member-1"]);
  });
});
