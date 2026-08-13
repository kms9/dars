package main

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kms9/dars/internal/events"
	"github.com/kms9/dars/internal/realtime"
	"github.com/kms9/dars/pkg/protocol"
)

func nullableActorID(actorID string) any {
	if strings.TrimSpace(actorID) == "" {
		return nil
	}
	return actorID
}

// registerLightweightListeners is the only Web Workspace event bridge used by
// the target runtime. It intentionally has no personal/global fallbacks: a
// target event must be allowlisted and scoped to one workspace.
func registerLightweightListeners(bus *events.Bus, broadcaster realtime.Broadcaster) {
	bus.SubscribeAll(func(event events.Event) {
		if !protocol.IsLightweightWebEvent(event.Type) || strings.TrimSpace(event.WorkspaceID) == "" {
			return
		}
		frame, err := json.Marshal(map[string]any{
			"type":         event.Type,
			"event_id":     uuid.NewString(),
			"workspace_id": event.WorkspaceID,
			"occurred_at":  time.Now().UTC().Format(time.RFC3339Nano),
			"actor_type":   event.ActorType,
			"actor_id":     nullableActorID(event.ActorID),
			"payload":      event.Payload,
		})
		if err != nil {
			slog.Error("failed to marshal lightweight event", "event_type", event.Type, "error", err)
			return
		}
		realtime.M.RecordEvent(event.Type)
		broadcaster.BroadcastToWorkspace(event.WorkspaceID, frame)
	})
}
