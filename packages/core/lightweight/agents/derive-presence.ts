import type { LightweightAgentSnapshotTask, LightweightRuntime } from "../types";
import type { AgentAvailability, AgentPresenceDetail, AgentWorkload } from "./types";
import { isRuntimeOnline } from "./runtime-access";

export function deriveWorkload(counts: {
  runningCount: number;
  queuedCount: number;
}): AgentWorkload {
  if (counts.runningCount > 0) return "working";
  if (counts.queuedCount > 0) return "queued";
  return "idle";
}

export function deriveWorkloadDetail(tasks: readonly LightweightAgentSnapshotTask[]): {
  workload: AgentWorkload;
  runningCount: number;
  queuedCount: number;
} {
  let runningCount = 0;
  let queuedCount = 0;
  for (const task of tasks) {
    if (task.status === "running") runningCount += 1;
    else if (
      task.status === "queued" ||
      task.status === "dispatched" ||
      task.status === "waiting_local_directory"
    ) {
      queuedCount += 1;
    }
  }
  return {
    workload: deriveWorkload({ runningCount, queuedCount }),
    runningCount,
    queuedCount,
  };
}

export function deriveAgentAvailability(
  runtime: LightweightRuntime | null,
  archived: boolean,
): AgentAvailability {
  if (archived) return "archived";
  if (!runtime) return "offline";
  if (isRuntimeOnline(runtime)) return "online";
  if (runtime.last_seen_at) return "unstable";
  return "offline";
}

export function deriveAgentPresenceDetail(input: {
  runtime: LightweightRuntime | null;
  tasks: readonly LightweightAgentSnapshotTask[];
  archived: boolean;
  maxConcurrentTasks: number;
}): AgentPresenceDetail {
  const availability = deriveAgentAvailability(input.runtime, input.archived);
  const detail = deriveWorkloadDetail(input.tasks);
  return {
    availability,
    workload: detail.workload,
    runningCount: detail.runningCount,
    queuedCount: detail.queuedCount,
    capacity: input.maxConcurrentTasks,
  };
}

