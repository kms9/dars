package lightweightapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/auth"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type daemonTokenResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	DaemonID    string  `json:"daemon_id"`
	UserID      *string `json:"user_id"`
	Name        *string `json:"name"`
	TokenPrefix *string `json:"token_prefix"`
	ExpiresAt   string  `json:"expires_at"`
	CreatedAt   string  `json:"created_at"`
}

type daemonTokenSecretResponse struct {
	daemonTokenResponse
	Token string `json:"token"`
}

func toDaemonTokenResponse(token lwdb.DaemonToken) daemonTokenResponse {
	var userID *string
	if token.UserID.Valid {
		value := uuidString(token.UserID)
		userID = &value
	}
	return daemonTokenResponse{
		ID: uuidString(token.ID), WorkspaceID: uuidString(token.WorkspaceID), DaemonID: token.DaemonID,
		UserID: userID, Name: textPointer(token.Name), TokenPrefix: textPointer(token.TokenPrefix),
		ExpiresAt: timeString(token.ExpiresAt), CreatedAt: timeString(token.CreatedAt),
	}
}

func (h *Handler) ListDaemonTokens(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := parseUUID(WorkspaceIDFromContext(r.Context()))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	rows, err := h.q.ListDaemonTokensByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]daemonTokenResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toDaemonTokenResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) CreateDaemonToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || (principal.Kind != principalJWT && principal.Kind != principalPAT) {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var request struct {
		DaemonID  string `json:"daemon_id"`
		Name      string `json:"name"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	request.DaemonID = strings.TrimSpace(request.DaemonID)
	request.Name = strings.TrimSpace(request.Name)
	if request.DaemonID == "" || len(request.DaemonID) > 200 || request.Name == "" || len(request.Name) > 120 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	workspaceID, err := parseUUID(WorkspaceIDFromContext(r.Context()))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	now := h.now()
	expiry := now.Add(defaultDaemonTTL)
	if strings.TrimSpace(request.ExpiresAt) != "" {
		expiry, err = parseRFC3339Future(request.ExpiresAt, now)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	if expiry.After(now.Add(maxDaemonTTL)) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	plaintext, err := auth.GenerateDaemonToken()
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	row, err := qtx.CreateDaemonToken(r.Context(), lwdb.CreateDaemonTokenParams{
		TokenHash: auth.HashToken(plaintext), WorkspaceID: workspaceID, DaemonID: request.DaemonID,
		ExpiresAt: pgtype.Timestamptz{Time: expiry, Valid: true}, UserID: userID,
		Name:        pgtype.Text{String: request.Name, Valid: true},
		TokenPrefix: pgtype.Text{String: tokenPrefix(plaintext), Valid: true},
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.MarkAgentRuntimesOffline(r.Context(), lwdb.MarkAgentRuntimesOfflineParams{
		WorkspaceID: workspaceID, DaemonID: request.DaemonID,
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, daemonTokenSecretResponse{
		daemonTokenResponse: toDaemonTokenResponse(row), Token: plaintext,
	})
}

func (h *Handler) RevokeDaemonToken(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := parseUUID(WorkspaceIDFromContext(r.Context()))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	tokenID, err := parseUUID(chi.URLParam(r, "tokenId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	row, err := qtx.DeleteDaemonTokenByID(r.Context(), lwdb.DeleteDaemonTokenByIDParams{
		ID: tokenID, WorkspaceID: workspaceID,
	})
	if err != nil {
		if notFound(err) {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.MarkAgentRuntimesOffline(r.Context(), lwdb.MarkAgentRuntimesOfflineParams{
		WorkspaceID: workspaceID, DaemonID: row.DaemonID,
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
