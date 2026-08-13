package lightweightapi

import (
	"context"
	"strings"

	"github.com/kms9/dars/internal/auth"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

// IsMember implements realtime.MembershipChecker against the lightweight
// member table. Both identifiers are UUIDs in the target protocol; slugs are
// deliberately not resolved at this boundary.
func (h *Handler) IsMember(ctx context.Context, userID, workspaceID string) bool {
	userUUID, err := parseUUID(userID)
	if err != nil {
		return false
	}
	workspaceUUID, err := parseUUID(workspaceID)
	if err != nil {
		return false
	}
	_, err = h.q.GetMember(ctx, lwdb.GetMemberParams{
		WorkspaceID: workspaceUUID,
		UserID:      userUUID,
	})
	return err == nil
}

// ResolveToken implements realtime.PATResolver. WebSocket PAT auth accepts
// only the target dpat_ credential and resolves it through the lightweight
// token table; daemon and task credentials cannot cross this boundary.
func (h *Handler) ResolveToken(ctx context.Context, token string) (string, bool) {
	if !strings.HasPrefix(token, "dpat_") {
		return "", false
	}
	stored, err := h.q.GetPersonalAccessTokenByHash(ctx, auth.HashToken(token))
	if err != nil {
		return "", false
	}
	_, _ = h.q.TouchPersonalAccessToken(ctx, stored.ID)
	return uuidString(stored.UserID), true
}
