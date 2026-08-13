package lightweightapi

import "github.com/kms9/dars/internal/events"

func (h *Handler) publish(eventType, workspaceID, actorType, actorID string, payload any) {
	if h.cfg.EventBus == nil {
		return
	}
	h.cfg.EventBus.Publish(events.Event{
		Type: eventType, WorkspaceID: workspaceID, ActorType: actorType, ActorID: actorID, Payload: payload,
	})
}
