/** Frozen protocol constants shared by the Lightweight Web runtime. */
export const LIGHTWEIGHT_DAEMON_PROTOCOL_VERSION = "lightweight-runtime-v1" as const;
export const LIGHTWEIGHT_WEB_SOCKET_PATH = "/ws" as const;
export const LIGHTWEIGHT_WEB_SOCKET_WORKSPACE_PARAM = "workspace_id" as const;

export const LIGHTWEIGHT_WS_EVENT_TYPES = [
  "agent:archived",
  "agent:created",
  "agent:restored",
  "agent:status",
  "agent:updated",
  "chat:cancel_finalized",
  "chat:done",
  "chat:message",
  "chat:session_deleted",
  "chat:session_updated",
  "comment:created",
  "daemon:register",
  "issue:created",
  "issue:deleted",
  "issue:updated",
  "skill:created",
  "skill:deleted",
  "skill:updated",
  "squad:created",
  "squad:deleted",
  "squad:updated",
  "task:cancelled",
  "task:completed",
  "task:dispatch",
  "task:failed",
  "task:message",
  "task:progress",
  "task:queued",
  "task:running",
  "task:waiting_local_directory",
  "workspace:deleted",
  "workspace:updated",
] as const;

export type LightweightWSEventType = (typeof LIGHTWEIGHT_WS_EVENT_TYPES)[number];

const lightweightWSEventSet: ReadonlySet<string> = new Set(LIGHTWEIGHT_WS_EVENT_TYPES);

export function isLightweightWSEventType(value: string): value is LightweightWSEventType {
  return lightweightWSEventSet.has(value);
}
