package lightweightapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type runtimeProfileResponse struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	DisplayName    string          `json:"display_name"`
	ProtocolFamily string          `json:"protocol_family"`
	CommandName    string          `json:"command_name"`
	Description    *string         `json:"description"`
	FixedArgs      json.RawMessage `json:"fixed_args"`
	Enabled        bool            `json:"enabled"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

func toRuntimeProfileResponse(profile lwdb.RuntimeProfile) runtimeProfileResponse {
	return runtimeProfileResponse{
		ID: uuidString(profile.ID), WorkspaceID: uuidString(profile.WorkspaceID), DisplayName: profile.DisplayName,
		ProtocolFamily: profile.ProtocolFamily, CommandName: profile.CommandName,
		Description: textPointer(profile.Description), FixedArgs: profile.FixedArgs, Enabled: profile.Enabled,
		CreatedAt: timeString(profile.CreatedAt), UpdatedAt: timeString(profile.UpdatedAt),
	}
}

func (h *Handler) ListRuntimeProfiles(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	rows, err := h.q.ListRuntimeProfiles(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]runtimeProfileResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toRuntimeProfileResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	profileID, err := parseUUID(chi.URLParam(r, "profileId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	profile, err := h.q.GetRuntimeProfile(r.Context(), lwdb.GetRuntimeProfileParams{ID: profileID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, toRuntimeProfileResponse(profile))
}

type runtimeProfileMutation struct {
	DisplayName    *string         `json:"display_name"`
	ProtocolFamily *string         `json:"protocol_family"`
	CommandName    *string         `json:"command_name"`
	Description    json.RawMessage `json:"description"`
	FixedArgs      json.RawMessage `json:"fixed_args"`
	Enabled        *bool           `json:"enabled"`
}

func validateProfileString(value *string, max int) bool {
	if value == nil {
		return true
	}
	*value = strings.TrimSpace(*value)
	return *value != "" && len(*value) <= max
}

func (h *Handler) CreateRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	principal, _ := principalFromContext(r.Context())
	userID, _ := parseUUID(principal.UserID)
	var request runtimeProfileMutation
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.DisplayName == nil || request.ProtocolFamily == nil || request.CommandName == nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if !validateProfileString(request.DisplayName, 120) || !validateProfileString(request.ProtocolFamily, 80) || !validateProfileString(request.CommandName, 200) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	description, ok := optionalText(request.Description)
	if !ok {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	fixedArgs := json.RawMessage(`[]`)
	if request.FixedArgs != nil {
		trimmed := bytes.TrimSpace(request.FixedArgs)
		if !json.Valid(trimmed) || len(trimmed) == 0 || trimmed[0] != '[' {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		fixedArgs = trimmed
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	profile, err := h.q.CreateRuntimeProfile(r.Context(), lwdb.CreateRuntimeProfileParams{
		WorkspaceID: workspaceID, DisplayName: *request.DisplayName, ProtocolFamily: *request.ProtocolFamily,
		CommandName: *request.CommandName, Description: description, FixedArgs: fixedArgs, CreatedBy: userID, Enabled: enabled,
	})
	if err != nil {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	writeJSON(w, http.StatusCreated, toRuntimeProfileResponse(profile))
}

func (h *Handler) UpdateRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	profileID, err := parseUUID(chi.URLParam(r, "profileId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	var request runtimeProfileMutation
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !validateProfileString(request.DisplayName, 120) || !validateProfileString(request.ProtocolFamily, 80) || !validateProfileString(request.CommandName, 200) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	params := lwdb.UpdateRuntimeProfileParams{ID: profileID, WorkspaceID: workspaceID}
	if request.DisplayName != nil {
		params.SetDisplayName, params.DisplayName = true, *request.DisplayName
	}
	if request.ProtocolFamily != nil {
		params.SetProtocolFamily, params.ProtocolFamily = true, *request.ProtocolFamily
	}
	if request.CommandName != nil {
		params.SetCommandName, params.CommandName = true, *request.CommandName
	}
	if request.Description != nil {
		params.SetDescription = true
		value, ok := optionalText(request.Description)
		if !ok {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.Description = value
	}
	if request.FixedArgs != nil {
		trimmed := bytes.TrimSpace(request.FixedArgs)
		if !json.Valid(trimmed) || len(trimmed) == 0 || trimmed[0] != '[' {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.SetFixedArgs, params.FixedArgs = true, trimmed
	}
	if request.Enabled != nil {
		params.SetEnabled, params.Enabled = true, *request.Enabled
	}
	profile, err := h.q.UpdateRuntimeProfile(r.Context(), params)
	if err != nil {
		if notFound(err) {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	writeJSON(w, http.StatusOK, toRuntimeProfileResponse(profile))
}

func (h *Handler) DeleteRuntimeProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	profileID, err := parseUUID(chi.URLParam(r, "profileId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	count, err := h.q.CountAgentRuntimesByProfile(r.Context(), lwdb.CountAgentRuntimesByProfileParams{WorkspaceID: workspaceID, ProfileID: profileID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if count > 0 {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	deleted, err := h.q.DeleteRuntimeProfile(r.Context(), lwdb.DeleteRuntimeProfileParams{ID: profileID, WorkspaceID: workspaceID})
	if err != nil || deleted != 1 {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListAgentRuntimes(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	rows, err := h.q.ListAgentRuntimes(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]runtimeResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toRuntimeResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) UpdateAgentRuntime(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtimeID, err := parseUUID(chi.URLParam(r, "runtimeId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, runtime.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	var request struct {
		CustomName json.RawMessage `json:"custom_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	customName, ok := optionalText(request.CustomName)
	if !ok || (customName.Valid && len(customName.String) > 200) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	updated, err := h.q.UpdateAgentRuntimeCustomName(r.Context(), lwdb.UpdateAgentRuntimeCustomNameParams{CustomName: customName, ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, toRuntimeResponse(updated))
}

func (h *Handler) DeleteAgentRuntime(w http.ResponseWriter, r *http.Request) {
	h.deleteRuntime(w, r, nil)
}

type unbindRuntimeRequest struct {
	ExpectedActiveAgentIDs []string `json:"expected_active_agent_ids"`
}

func (h *Handler) UnbindAgentsAndDeleteRuntime(w http.ResponseWriter, r *http.Request) {
	member, ok := MemberFromContext(r.Context())
	if !ok || (member.Role != "owner" && member.Role != "admin") {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	var request unbindRuntimeRequest
	if err := decodeStrictTaskBody(r, &request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	seen := make(map[string]struct{}, len(request.ExpectedActiveAgentIDs))
	for index, rawID := range request.ExpectedActiveAgentIDs {
		id, err := parseUUID(rawID)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		normalized := uuidString(id)
		if _, duplicate := seen[normalized]; duplicate {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		seen[normalized] = struct{}{}
		request.ExpectedActiveAgentIDs[index] = normalized
	}
	h.deleteRuntime(w, r, request.ExpectedActiveAgentIDs)
}

func (h *Handler) deleteRuntime(w http.ResponseWriter, r *http.Request, expectedAgentIDs []string) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtimeID, err := parseUUID(chi.URLParam(r, "runtimeId"))
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
	runtime, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, runtime.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if runtime.ProfileID.Valid {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	activeTasks, err := qtx.CountActiveTasksByRuntime(r.Context(), lwdb.CountActiveTasksByRuntimeParams{WorkspaceID: workspaceID, RuntimeID: runtimeID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if activeTasks > 0 {
		writeCode(w, http.StatusConflict, "active_tasks_exist")
		return
	}
	agents, err := qtx.LockAgentsByRuntime(r.Context(), lwdb.LockAgentsByRuntimeParams{WorkspaceID: workspaceID, RuntimeID: runtimeID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	actual := make([]string, 0, len(agents))
	for _, agent := range agents {
		if !agent.ArchivedAt.Valid {
			actual = append(actual, uuidString(agent.ID))
		}
	}
	sort.Strings(actual)
	sort.Strings(expectedAgentIDs)
	if expectedAgentIDs == nil && len(actual) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "active_agent_ids": actual})
		return
	}
	if expectedAgentIDs != nil && strings.Join(actual, "\n") != strings.Join(expectedAgentIDs, "\n") {
		writeJSON(w, http.StatusConflict, map[string]any{"code": "conflict", "active_agent_ids": actual})
		return
	}
	// Clear every binding, including archived Agents that were intentionally
	// omitted from the confirmation set. Without foreign keys this explicit
	// cleanup is what prevents a later restore from reviving a dangling Runtime.
	if _, err := qtx.ClearAgentRuntimeBindings(r.Context(), lwdb.ClearAgentRuntimeBindingsParams{WorkspaceID: workspaceID, RuntimeID: runtimeID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	deleted, err := qtx.DeleteAgentRuntime(r.Context(), lwdb.DeleteAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil || deleted != 1 {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func contextWorkspaceUUID(r *http.Request) (pgtype.UUID, error) {
	return parseUUID(WorkspaceIDFromContext(r.Context()))
}

func optionalText(raw json.RawMessage) (pgtype.Text, bool) {
	if raw == nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return pgtype.Text{}, true
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return pgtype.Text{}, false
	}
	return pgtype.Text{String: value, Valid: true}, true
}
