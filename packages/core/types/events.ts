import type { LightweightWSEventType } from "./lightweight-protocol";

export type WSEventType = LightweightWSEventType;

export interface WSMessage {
  type: WSEventType;
  event_id?: string;
  workspace_id?: string;
  occurred_at?: string;
  actor_type?: string;
  actor_id?: string;
  payload: unknown;
}
