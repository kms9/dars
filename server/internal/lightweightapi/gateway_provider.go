package lightweightapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

// requireGatewayProviderForNewTask permits all legacy Tasks without an Agent
// Bundle head. Bundle Tasks fail closed unless the selected Runtime provider is
// present in the Server-owned compatibility policy.
func (h *Handler) requireGatewayProviderForNewTask(
	ctx context.Context,
	q *lwdb.Queries,
	workspaceID pgtype.UUID,
	agentID pgtype.UUID,
	runtimeID pgtype.UUID,
) error {
	if _, err := q.GetAgentToolBundleHead(ctx, lwdb.GetAgentToolBundleHeadParams{WorkspaceID: workspaceID, AgentID: agentID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	return h.requireGatewayProviderForPinnedTask(ctx, q, workspaceID, runtimeID)
}

func (h *Handler) requireGatewayProviderForPinnedTask(
	ctx context.Context,
	q *lwdb.Queries,
	workspaceID pgtype.UUID,
	runtimeID pgtype.UUID,
) error {
	runtime, err := q.GetAgentRuntime(ctx, lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil {
		return err
	}
	if h.cfg.GatewayProviderPolicy == nil || !h.cfg.GatewayProviderPolicy.ProviderSupported(runtime.Provider) {
		return issueResolutionFailure(http.StatusConflict, "provider_mcp_unsupported")
	}
	return nil
}

func writeGatewayProviderError(w http.ResponseWriter, err error) {
	var resolution *issueResolutionError
	if errors.As(err, &resolution) && resolution.code == "provider_mcp_unsupported" {
		writeCode(w, resolution.status, resolution.code)
		return
	}
	writeCode(w, http.StatusInternalServerError, "internal_error")
}
