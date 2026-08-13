import { lightweightApi } from "./api";
import type {
  LightweightCreateRuntimeLocalSkillImportRequest,
  LightweightRuntimeLocalSkillImportResult,
  LightweightRuntimeLocalSkillsResult,
} from "./types";

const POLL_INTERVAL_MS = 500;
const LIST_POLL_TIMEOUT_MS = 30_000;
// Must exceed runtimeImportPendingTimeout + runtimeRequestRunningTimeout on the server.
const IMPORT_POLL_TIMEOUT_MS = 4 * 60_000;

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export async function resolveRuntimeLocalSkills(
  runtimeId: string,
): Promise<LightweightRuntimeLocalSkillsResult> {
  const initial = await lightweightApi.initiateListLocalSkills(runtimeId);
  const start = Date.now();
  let current = initial;

  while (current.status === "pending" || current.status === "running") {
    if (Date.now() - start > LIST_POLL_TIMEOUT_MS) {
      throw new Error("runtime local skill discovery timed out");
    }
    await sleep(POLL_INTERVAL_MS);
    current = await lightweightApi.getListLocalSkillsResult(runtimeId, initial.id);
  }

  if (current.status === "failed" || current.status === "timeout") {
    throw new Error(current.error || "runtime local skill discovery failed");
  }

  return {
    skills: current.skills ?? [],
    supported: current.supported,
  };
}

export async function resolveRuntimeLocalSkillImport(
  runtimeId: string,
  payload: LightweightCreateRuntimeLocalSkillImportRequest,
): Promise<LightweightRuntimeLocalSkillImportResult> {
  const initial = await lightweightApi.initiateImportLocalSkill(runtimeId, payload);
  const start = Date.now();
  let current = initial;

  while (current.status === "pending" || current.status === "running") {
    if (Date.now() - start > IMPORT_POLL_TIMEOUT_MS) {
      throw new Error("runtime local skill import timed out");
    }
    await sleep(POLL_INTERVAL_MS);
    current = await lightweightApi.getImportLocalSkillResult(runtimeId, initial.id);
  }

  if (current.status === "conflict") {
    if (!current.conflict) {
      throw new Error("runtime local skill import conflict missing details");
    }
    return { status: "conflict", conflict: current.conflict };
  }

  if (current.status === "failed" || current.status === "timeout") {
    throw new Error(current.error || "runtime local skill import failed");
  }
  if (!current.skill) {
    throw new Error("runtime local skill import did not return a skill");
  }

  return {
    status: current.action === "overwrite" ? "updated" : "created",
    skill: current.skill,
  };
}

export function unwrapSkillImportResponse(
  result: unknown,
): { skill?: { id: string; name: string }; status?: string; reason?: string } {
  if (!result || typeof result !== "object") return {};
  const value = result as Record<string, unknown>;
  if (typeof value.id === "string" && typeof value.name === "string" && !("status" in value && "skill" in value)) {
    return { skill: value as { id: string; name: string }, status: "created" };
  }
  const skill = value.skill;
  return {
    status: typeof value.status === "string" ? value.status : undefined,
    reason: typeof value.reason === "string" ? value.reason : undefined,
    skill:
      skill && typeof skill === "object" && typeof (skill as { id?: unknown }).id === "string"
        ? (skill as { id: string; name: string })
        : undefined,
  };
}
