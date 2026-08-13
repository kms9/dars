package lightweightapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/auth"
	"github.com/kms9/dars/internal/daemonws"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

type registerRuntimeRequest struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Version   string  `json:"version"`
	Status    string  `json:"status"`
	ProfileID *string `json:"profile_id"`
}

type daemonRegisterRequest struct {
	ProtocolVersion string                   `json:"protocol_version"`
	WorkspaceID     string                   `json:"workspace_id"`
	DaemonID        string                   `json:"daemon_id"`
	DeviceName      string                   `json:"device_name"`
	CLIVersion      string                   `json:"cli_version"`
	Runtimes        []registerRuntimeRequest `json:"runtimes"`
	FailedProfiles  json.RawMessage          `json:"failed_profiles"`
}

type runtimeResponse struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	DaemonID    string          `json:"daemon_id"`
	Name        string          `json:"name"`
	Provider    string          `json:"provider"`
	Status      string          `json:"status"`
	DeviceInfo  string          `json:"device_info"`
	Metadata    json.RawMessage `json:"metadata"`
	OwnerID     string          `json:"owner_id"`
	ProfileID   *string         `json:"profile_id"`
	LastSeenAt  *string         `json:"last_seen_at"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

func toRuntimeResponse(runtime lwdb.AgentRuntime) runtimeResponse {
	var profileID *string
	if runtime.ProfileID.Valid {
		value := uuidString(runtime.ProfileID)
		profileID = &value
	}
	return runtimeResponse{
		ID: uuidString(runtime.ID), WorkspaceID: uuidString(runtime.WorkspaceID), DaemonID: runtime.DaemonID,
		Name: runtime.Name, Provider: runtime.Provider, Status: runtime.Status, DeviceInfo: runtime.DeviceInfo,
		Metadata: runtime.Metadata, OwnerID: uuidString(runtime.OwnerID), ProfileID: profileID,
		LastSeenAt: timePointer(runtime.LastSeenAt),
		CreatedAt:  timeString(runtime.CreatedAt), UpdatedAt: timeString(runtime.UpdatedAt),
	}
}

type daemonRegisterResponse struct {
	Token       string            `json:"token"`
	WorkspaceID string            `json:"workspace_id"`
	DaemonID    string            `json:"daemon_id"`
	ExpiresAt   string            `json:"expires_at"`
	Runtimes    []runtimeResponse `json:"runtimes"`
}

func (h *Handler) DaemonRegister(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || (principal.Kind != principalJWT && principal.Kind != principalPAT) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	var request daemonRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if request.ProtocolVersion != protocolVersion {
		writeCode(w, http.StatusBadRequest, "protocol_version_unsupported")
		return
	}
	request.DaemonID = strings.TrimSpace(request.DaemonID)
	request.DeviceName = strings.TrimSpace(request.DeviceName)
	if request.DaemonID == "" || len(request.DaemonID) > 200 || len(request.Runtimes) == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	workspaceID, err := parseUUID(request.WorkspaceID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if _, err := h.q.GetMember(r.Context(), lwdb.GetMemberParams{WorkspaceID: workspaceID, UserID: userID}); err != nil {
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
	runtimes := make([]runtimeResponse, 0, len(request.Runtimes))
	for _, candidate := range request.Runtimes {
		provider := strings.ToLower(strings.TrimSpace(candidate.Type))
		name := strings.TrimSpace(candidate.Name)
		if provider == "" || name == "" || len(provider) > 80 || len(name) > 200 {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		status := "online"
		if candidate.Status == "offline" {
			status = "offline"
		}
		var profileID pgtype.UUID
		if candidate.ProfileID != nil && strings.TrimSpace(*candidate.ProfileID) != "" {
			profileID, err = parseUUID(*candidate.ProfileID)
			if err != nil {
				writeCode(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			profile, profileErr := qtx.GetRuntimeProfile(r.Context(), lwdb.GetRuntimeProfileParams{ID: profileID, WorkspaceID: workspaceID})
			if profileErr != nil || !profile.Enabled {
				writeCode(w, http.StatusConflict, "conflict")
				return
			}
			provider = profile.ProtocolFamily
		}
		deviceInfo := request.DeviceName
		if candidate.Version != "" {
			if deviceInfo != "" {
				deviceInfo += " / "
			}
			deviceInfo += candidate.Version
		}
		metadata, _ := json.Marshal(map[string]string{"runtime_version": candidate.Version, "cli_version": request.CLIVersion})
		row, err := qtx.UpsertAgentRuntime(r.Context(), lwdb.UpsertAgentRuntimeParams{
			WorkspaceID: workspaceID, DaemonID: request.DaemonID, Name: name, Provider: provider,
			Status: status, DeviceInfo: deviceInfo, Metadata: metadata, OwnerID: userID, ProfileID: profileID,
		})
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		runtimes = append(runtimes, toRuntimeResponse(row))
	}
	plaintext, err := auth.GenerateDaemonToken()
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	expiresAt := pgtype.Timestamptz{Time: h.now().Add(defaultDaemonTTL), Valid: true}
	if _, err := qtx.CreateDaemonToken(r.Context(), lwdb.CreateDaemonTokenParams{
		TokenHash: auth.HashToken(plaintext), WorkspaceID: workspaceID, DaemonID: request.DaemonID, ExpiresAt: expiresAt,
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, daemonRegisterResponse{
		Token: plaintext, WorkspaceID: request.WorkspaceID, DaemonID: request.DaemonID,
		ExpiresAt: expiresAt.Time.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Runtimes: runtimes,
	})
}

func (h *Handler) DaemonDeregister(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var request struct {
		RuntimeIDs []string `json:"runtime_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.RuntimeIDs) == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	workspaceID, _ := parseUUID(principal.WorkspaceID)
	for _, rawID := range request.RuntimeIDs {
		runtimeID, err := parseUUID(rawID)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
		if err != nil || runtime.DaemonID != principal.DaemonID {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	if _, err := qtx.MarkAgentRuntimesOffline(r.Context(), lwdb.MarkAgentRuntimesOfflineParams{WorkspaceID: workspaceID, DaemonID: principal.DaemonID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.DeleteDaemonTokensByDaemon(r.Context(), lwdb.DeleteDaemonTokensByDaemonParams{WorkspaceID: workspaceID, DaemonID: principal.DaemonID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) DaemonHeartbeat(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var request struct {
		ProtocolVersion     string `json:"protocol_version"`
		RuntimeID           string `json:"runtime_id"`
		SupportsBatchImport bool   `json:"supports_batch_import"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if request.ProtocolVersion != protocolVersion {
		writeCode(w, http.StatusBadRequest, "protocol_version_unsupported")
		return
	}
	runtimeID, err := parseUUID(request.RuntimeID)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	workspaceID, _ := parseUUID(principal.WorkspaceID)
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil || runtime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if h.cfg.Heartbeat != nil {
		h.cfg.Heartbeat.Schedule(runtimeID)
	} else if _, err := h.q.TouchAgentRuntime(r.Context(), lwdb.TouchAgentRuntimeParams{
		ID: runtimeID, WorkspaceID: workspaceID, DaemonID: principal.DaemonID,
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, h.runtimeRequestHeartbeatAck(request.RuntimeID, request.SupportsBatchImport))
}

func (h *Handler) DaemonWebSocket(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if h.cfg.DaemonHub == nil {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}
	if r.Header.Get("X-Daemon-Protocol-Version") != protocolVersion || r.URL.Query().Get("protocol_version") != protocolVersion {
		writeCode(w, http.StatusBadRequest, "protocol_version_unsupported")
		return
	}
	runtimeIDs := parseDaemonRuntimeIDs(r.URL.Query()["runtime_ids"])
	if len(runtimeIDs) == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	workspaceID, err := parseUUID(principal.WorkspaceID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	for _, rawRuntimeID := range runtimeIDs {
		runtimeID, err := parseUUID(rawRuntimeID)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
		if err != nil || runtime.DaemonID != principal.DaemonID {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
	}
	h.cfg.DaemonHub.HandleWebSocket(w, r, daemonws.ClientIdentity{
		DaemonID: principal.DaemonID, WorkspaceID: principal.WorkspaceID,
		WorkspaceIDs: []string{principal.WorkspaceID}, RuntimeIDs: runtimeIDs,
		ClientVersion: r.Header.Get("X-Client-Version"), Capabilities: r.Header.Get("X-Client-Capabilities"),
	})
}

func parseDaemonRuntimeIDs(rawValues []string) []string {
	seen := make(map[string]struct{})
	var runtimeIDs []string
	for _, rawValue := range rawValues {
		for _, rawID := range strings.Split(rawValue, ",") {
			id := strings.TrimSpace(rawID)
			if id == "" {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			runtimeIDs = append(runtimeIDs, id)
		}
	}
	return runtimeIDs
}

func (h *Handler) HandleDaemonWSHeartbeat(ctx context.Context, identity daemonws.ClientIdentity, rawRuntimeID string, supportsBatchImport bool) (*protocol.DaemonHeartbeatAckPayload, error) {
	runtimeID, err := parseUUID(rawRuntimeID)
	if err != nil {
		return nil, fmt.Errorf("invalid runtime id")
	}
	workspaceID, err := parseUUID(identity.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace scope")
	}
	runtime, err := h.q.GetAgentRuntime(ctx, lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil {
		if notFound(err) {
			return &protocol.DaemonHeartbeatAckPayload{RuntimeID: rawRuntimeID, Status: protocol.HeartbeatStatusRuntimeGone, RuntimeGone: true}, nil
		}
		return nil, fmt.Errorf("runtime lookup failed")
	}
	if runtime.DaemonID != identity.DaemonID {
		return nil, fmt.Errorf("runtime outside daemon scope")
	}
	updated, err := h.q.TouchAgentRuntime(ctx, lwdb.TouchAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID, DaemonID: identity.DaemonID})
	if err != nil {
		return nil, fmt.Errorf("runtime heartbeat failed")
	}
	if updated != 1 {
		return &protocol.DaemonHeartbeatAckPayload{RuntimeID: rawRuntimeID, Status: protocol.HeartbeatStatusRuntimeGone, RuntimeGone: true}, nil
	}
	return h.runtimeRequestHeartbeatAck(rawRuntimeID, supportsBatchImport), nil
}

func (h *Handler) ListDaemonWorkspaces(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workspaceID, _ := parseUUID(principal.WorkspaceID)
	workspace, err := h.q.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, []workspaceResponse{toWorkspaceResponse(workspace)})
}

func (h *Handler) GetDaemonWorkspaceRepos(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if chi.URLParam(r, "workspaceId") != principal.WorkspaceID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	workspaceID, _ := parseUUID(principal.WorkspaceID)
	workspace, err := h.q.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	hash := sha256.Sum256(workspace.Repos)
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id":  principal.WorkspaceID,
		"repos":         json.RawMessage(workspace.Repos),
		"repos_version": hex.EncodeToString(hash[:]),
	})
}

func (h *Handler) DaemonListRuntimeProfiles(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if chi.URLParam(r, "workspaceId") != principal.WorkspaceID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	workspaceID, err := parseUUID(principal.WorkspaceID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	profiles, err := h.q.ListRuntimeProfiles(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]runtimeProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		if profile.Enabled {
			response = append(response, toRuntimeProfileResponse(profile))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspace_id": principal.WorkspaceID, "runtime_profiles": response})
}
