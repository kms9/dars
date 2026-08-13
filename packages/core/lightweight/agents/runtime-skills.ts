import type {
  LightweightDisabledRuntimeSkill,
  LightweightRuntimeLocalSkillSummary,
} from "../types";

export function runtimeSkillIdentity(
  runtimeId: string,
  skill: Pick<LightweightRuntimeLocalSkillSummary, "key">,
): string {
  return `${runtimeId}:${skill.key}`;
}

export function isRuntimeSkillDisabled(
  disabled: readonly LightweightDisabledRuntimeSkill[],
  runtimeId: string,
  skill: Pick<LightweightRuntimeLocalSkillSummary, "key">,
): boolean {
  return disabled.some((entry) => entry.runtime_id === runtimeId && entry.key === skill.key);
}

export function setRuntimeSkillEnabled(
  disabled: readonly LightweightDisabledRuntimeSkill[],
  runtimeId: string,
  skill: LightweightRuntimeLocalSkillSummary,
  enabled: boolean,
): LightweightDisabledRuntimeSkill[] {
  const filtered = disabled.filter(
    (entry) => !(entry.runtime_id === runtimeId && entry.key === skill.key),
  );
  if (enabled) return [...filtered];
  return [
    ...filtered,
    {
      runtime_id: runtimeId,
      provider: skill.provider,
      root: skill.root ?? "provider",
      key: skill.key,
      name: skill.name,
      plugin: skill.plugin,
    },
  ];
}
