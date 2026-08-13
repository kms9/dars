package lightweightapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const maxAgentBuilderDraftBytes = 256 * 1024

type createAgentBuilderSessionRequest struct {
	RuntimeID string `json:"runtime_id"`
	Model     string `json:"model,omitempty"`
}

type createAgentBuilderSessionResponse struct {
	SessionID      string `json:"session_id"`
	BuilderAgentID string `json:"builder_agent_id"`
	RuntimeID      string `json:"runtime_id"`
}

type agentBuilderSessionSummary struct {
	SessionID          string          `json:"session_id"`
	Title              string          `json:"title"`
	RuntimeID          string          `json:"runtime_id"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
	LastMessageContent string          `json:"last_message_content"`
	LastMessageRole    string          `json:"last_message_role"`
	LastMessageAt      string          `json:"last_message_at"`
	Draft              json.RawMessage `json:"draft,omitempty"`
}

type listAgentBuilderSessionsResponse struct {
	Sessions []agentBuilderSessionSummary `json:"sessions"`
}

type saveAgentBuilderDraftRequest struct {
	Draft    json.RawMessage `json:"draft"`
	Finalize *bool           `json:"finalize,omitempty"`
}

type finalizeAgentBuilderResponse struct {
	AgentID string `json:"agent_id"`
}

type switchAgentBuilderRuntimeRequest struct {
	RuntimeID string `json:"runtime_id"`
}

type switchAgentBuilderRuntimeResponse struct {
	RuntimeID string `json:"runtime_id"`
}

func optionalPgText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func isAgentBuilderCarrier(agent lwdb.Agent) bool {
	return agent.Kind == "system" &&
		agent.SystemKey.Valid &&
		strings.HasPrefix(agent.SystemKey.String, "agent_builder:")
}

func (h *Handler) ListAgentBuilderSessions(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return
	}
	rows, err := h.q.ListAgentBuilderSessionsByCreator(r.Context(), lwdb.ListAgentBuilderSessionsByCreatorParams{
		WorkspaceID: workspaceID,
		CreatorID:   userID,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	sessions := make([]agentBuilderSessionSummary, 0, len(rows))
	for _, row := range rows {
		if tombstone, ok := parseAgentBuilderDraftTombstone(row.StoredDraft); ok {
			_ = tombstone
			continue
		}
		summary := agentBuilderSessionSummary{
			SessionID:          uuidString(row.ID),
			Title:              row.Title,
			RuntimeID:          uuidString(row.RuntimeID),
			CreatedAt:          timeString(row.CreatedAt),
			UpdatedAt:          timeString(row.UpdatedAt),
			LastMessageContent: row.LastMessageContent,
			LastMessageRole:    row.LastMessageRole,
		}
		if row.LastMessageAt.Valid {
			summary.LastMessageAt = timeString(row.LastMessageAt)
		}
		if len(row.StoredDraft) > 0 && json.Valid(row.StoredDraft) {
			summary.Draft = json.RawMessage(row.StoredDraft)
		}
		sessions = append(sessions, summary)
	}
	writeJSON(w, http.StatusOK, listAgentBuilderSessionsResponse{Sessions: sessions})
}

func (h *Handler) CreateAgentBuilderSession(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return
	}
	var request createAgentBuilderSessionRequest
	if decodeStrictTaskBody(r, &request) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtimeID, err := parseUUID(strings.TrimSpace(request.RuntimeID))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtime, ok := h.resolveBuilderRuntime(w, r, workspaceID, runtimeID)
	if !ok {
		return
	}
	flowID := uuid.NewString()
	model := strings.TrimSpace(request.Model)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	if _, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: runtime.ID, WorkspaceID: workspaceID}); err != nil {
		writeCode(w, http.StatusBadRequest, "agent_runtime_required")
		return
	}
	carrier, err := qtx.CreateAgentBuilderCarrier(r.Context(), lwdb.CreateAgentBuilderCarrierParams{
		WorkspaceID:  workspaceID,
		RuntimeID:    runtime.ID,
		OwnerID:      userID,
		Name:         fmt.Sprintf(".dars-agent-builder-%s", flowID),
		Instructions: agentBuilderInstructions,
		Model:        pgtype.Text{String: model, Valid: model != ""},
		SystemKey:    pgtype.Text{String: fmt.Sprintf("agent_builder:%s", flowID), Valid: true},
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	session, err := qtx.CreateChatSession(r.Context(), lwdb.CreateChatSessionParams{
		WorkspaceID: workspaceID,
		AgentID:     carrier.ID,
		CreatorID:   userID,
		RuntimeID:   runtime.ID,
		Title:       "Create an agent",
	})
	if err != nil || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, createAgentBuilderSessionResponse{
		SessionID:      uuidString(session.ID),
		BuilderAgentID: uuidString(carrier.ID),
		RuntimeID:      uuidString(runtime.ID),
	})
}

func (h *Handler) resolveBuilderRuntime(w http.ResponseWriter, r *http.Request, workspaceID, runtimeID pgtype.UUID) (lwdb.AgentRuntime, bool) {
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusBadRequest, "agent_runtime_required")
		return lwdb.AgentRuntime{}, false
	}
	if runtime.Status != "online" {
		writeCode(w, http.StatusConflict, "runtime_unavailable")
		return lwdb.AgentRuntime{}, false
	}
	return runtime, true
}

func (h *Handler) loadBuilderSession(w http.ResponseWriter, r *http.Request) (lwdb.ChatSession, lwdb.Agent, pgtype.UUID, pgtype.UUID, bool) {
	_, workspaceID, userID, ok := h.chatHumanScope(w, r)
	if !ok {
		return lwdb.ChatSession{}, lwdb.Agent{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	sessionID, err := parseUUID(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, lwdb.Agent{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	session, err := h.q.GetChatSession(r.Context(), lwdb.GetChatSessionParams{ID: sessionID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, lwdb.Agent{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	if session.CreatorID != userID {
		writeCode(w, http.StatusForbidden, "forbidden")
		return lwdb.ChatSession{}, lwdb.Agent{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	agent, err := h.q.GetAgentRow(r.Context(), lwdb.GetAgentRowParams{ID: session.AgentID, WorkspaceID: workspaceID})
	if err != nil || !isAgentBuilderCarrier(agent) {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.ChatSession{}, lwdb.Agent{}, pgtype.UUID{}, pgtype.UUID{}, false
	}
	return session, agent, workspaceID, userID, true
}

func (h *Handler) SaveAgentBuilderDraft(w http.ResponseWriter, r *http.Request) {
	session, carrier, workspaceID, userID, ok := h.loadBuilderSession(w, r)
	if !ok {
		return
	}
	var request saveAgentBuilderDraftRequest
	if decodeStrictTaskBody(r, &request) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if len(request.Draft) == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if len(request.Draft) > maxAgentBuilderDraftBytes {
		writeCode(w, http.StatusRequestEntityTooLarge, "invalid_argument")
		return
	}
	if !json.Valid(request.Draft) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if request.Finalize != nil && *request.Finalize {
		h.finalizeAgentBuilder(w, r, session, carrier, workspaceID, userID, request.Draft)
		return
	}
	if _, err := parseAgentBuilderDraft(request.Draft); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	locked, err := qtx.LockChatSession(r.Context(), lwdb.LockChatSessionParams{ID: session.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if locked.Status != "active" {
		writeCode(w, http.StatusConflict, "builder_session_completed")
		return
	}
	if _, err := qtx.UpsertAgentBuilderDraft(r.Context(), lwdb.UpsertAgentBuilderDraftParams{
		ChatSessionID: locked.ID,
		WorkspaceID:   locked.WorkspaceID,
		Draft:         request.Draft,
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

func (h *Handler) finalizeAgentBuilder(
	w http.ResponseWriter,
	r *http.Request,
	session lwdb.ChatSession,
	carrier lwdb.Agent,
	workspaceID, userID pgtype.UUID,
	rawDraft json.RawMessage,
) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > chatIdempotencyMaxBytes {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	requestHash := requestDigest(map[string]any{"draft": json.RawMessage(rawDraft), "finalize": true})
	if existing, err := h.q.GetAgentBuilderDraft(r.Context(), lwdb.GetAgentBuilderDraftParams{
		ChatSessionID: session.ID,
		WorkspaceID:   workspaceID,
	}); err == nil {
		if tombstone, ok := parseAgentBuilderDraftTombstone(existing.Draft); ok {
			if tombstone.IdempotencyKey != "" && tombstone.IdempotencyKey != idempotencyKey {
				writeCode(w, http.StatusConflict, "idempotency_key_reused")
				return
			}
			writeJSON(w, http.StatusOK, finalizeAgentBuilderResponse{AgentID: tombstone.FinalizedAgentID})
			return
		}
	}
	if session.Status != "active" {
		writeCode(w, http.StatusConflict, "builder_session_completed")
		return
	}
	draft, err := parseAgentBuilderDraft(rawDraft)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtimeID := carrier.RuntimeID
	if trimmed := strings.TrimSpace(draft.RuntimeID); trimmed != "" {
		parsed, parseErr := parseUUID(trimmed)
		if parseErr != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		runtimeID = parsed
	}
	if h.cfg.AgentSecrets == nil {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}
	encryptedEnv, err := h.cfg.AgentSecrets.EncryptCustomEnv(map[string]string{})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	encryptedMCP, err := h.cfg.AgentSecrets.EncryptMCPConfig(json.RawMessage(`{}`))
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	maxTasks := int32(1)
	if draft.MaxConcurrent != nil {
		maxTasks = *draft.MaxConcurrent
	}
	permissionMode := "private"
	if draft.PermissionScope == "workspace" || draft.PermissionScope == "members" {
		permissionMode = "public_to"
	}
	skillIDs := make([]pgtype.UUID, 0, len(draft.SkillIDs))
	seenSkills := make(map[string]struct{}, len(draft.SkillIDs))
	for _, skillID := range draft.SkillIDs {
		parsed, parseErr := parseUUID(strings.TrimSpace(skillID))
		if parseErr != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		key := uuidString(parsed)
		if _, duplicate := seenSkills[key]; duplicate {
			continue
		}
		seenSkills[key] = struct{}{}
		skillIDs = append(skillIDs, parsed)
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	lockedSession, err := qtx.LockChatSession(r.Context(), lwdb.LockChatSessionParams{ID: session.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if lockedSession.Status != "active" {
		writeCode(w, http.StatusConflict, "builder_session_completed")
		return
	}
	if _, err := qtx.GetPendingChatTask(r.Context(), lwdb.GetPendingChatTaskParams{
		WorkspaceID: workspaceID, ChatSessionID: lockedSession.ID,
	}); err == nil {
		writeCode(w, http.StatusConflict, "builder_task_active")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID}); err != nil {
		writeCode(w, http.StatusBadRequest, "agent_runtime_required")
		return
	}
	for _, skillID := range skillIDs {
		if _, err := qtx.LockSkill(r.Context(), lwdb.LockSkillParams{ID: skillID, WorkspaceID: workspaceID}); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	agent, err := qtx.CreateAgent(r.Context(), lwdb.CreateAgentParams{
		WorkspaceID: workspaceID, RuntimeID: runtimeID, OwnerID: userID, Name: draft.Name,
		Description: draft.Description, Instructions: draft.Instructions,
		RuntimeConfig: json.RawMessage(`{}`), MaxConcurrentTasks: maxTasks,
		CustomEnv: encryptedEnv, CustomArgs: json.RawMessage(`[]`), McpConfig: encryptedMCP,
		Model:          optionalPgText(draft.Model),
		ThinkingLevel:  optionalPgText(draft.ThinkingLevel),
		ServiceTier:    optionalPgText(draft.ServiceTier),
		PermissionMode: permissionMode, DisabledRuntimeSkills: json.RawMessage(`[]`),
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if draft.PermissionScope == "workspace" {
		if err := replaceInvocationTargets(r, qtx, agent, userID, []invocationTargetRequest{{
			TargetType: "workspace", TargetID: uuidString(workspaceID),
		}}); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	if draft.PermissionScope == "members" {
		targets := make([]invocationTargetRequest, 0, len(draft.MemberIDs))
		for _, memberID := range draft.MemberIDs {
			targets = append(targets, invocationTargetRequest{TargetType: "member", TargetID: strings.TrimSpace(memberID)})
		}
		if err := replaceInvocationTargets(r, qtx, agent, userID, targets); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	for _, skillID := range skillIDs {
		if _, err := qtx.ReplaceAgentSkill(r.Context(), lwdb.ReplaceAgentSkillParams{
			AgentID: agent.ID, SkillID: skillID, Enabled: true,
		}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	if _, err := qtx.UpdateChatSession(r.Context(), lwdb.UpdateChatSessionParams{
		SetStatus: true, Status: "archived",
		ID: lockedSession.ID, WorkspaceID: workspaceID,
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	tombstone := marshalAgentBuilderDraftTombstone(uuidString(agent.ID), idempotencyKey)
	if _, err := qtx.UpsertAgentBuilderDraft(r.Context(), lwdb.UpsertAgentBuilderDraftParams{
		ChatSessionID: lockedSession.ID,
		WorkspaceID:   workspaceID,
		Draft:         tombstone,
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	_ = requestHash
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response, err := h.agentResponse(r, agent)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	principal, _ := principalFromContext(r.Context())
	h.publish(protocol.EventAgentCreated, uuidString(workspaceID), "member", principal.UserID, map[string]any{"agent": broadcastAgentSummary(response)})
	h.publish(protocol.EventChatSessionUpdated, uuidString(workspaceID), "member", uuidString(userID), chatSessionDTO(lwdb.ChatSession{
		ID: lockedSession.ID, WorkspaceID: workspaceID, Status: "archived",
	}))
	writeJSON(w, http.StatusCreated, finalizeAgentBuilderResponse{AgentID: uuidString(agent.ID)})
}

func (h *Handler) SwitchAgentBuilderRuntime(w http.ResponseWriter, r *http.Request) {
	session, carrier, workspaceID, _, ok := h.loadBuilderSession(w, r)
	if !ok {
		return
	}
	if session.Status != "active" {
		writeCode(w, http.StatusConflict, "builder_session_completed")
		return
	}
	var request switchAgentBuilderRuntimeRequest
	if decodeStrictTaskBody(r, &request) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtimeID, err := parseUUID(strings.TrimSpace(request.RuntimeID))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtime, ok := h.resolveBuilderRuntime(w, r, workspaceID, runtimeID)
	if !ok {
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	if _, err := qtx.LockChatSession(r.Context(), lwdb.LockChatSessionParams{ID: session.ID, WorkspaceID: workspaceID}); err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if _, err := qtx.GetPendingChatTask(r.Context(), lwdb.GetPendingChatTaskParams{
		WorkspaceID: workspaceID, ChatSessionID: session.ID,
	}); err == nil {
		writeCode(w, http.StatusConflict, "builder_task_active")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	updated, err := qtx.UpdateAgentBuilderCarrierRuntime(r.Context(), lwdb.UpdateAgentBuilderCarrierRuntimeParams{
		ID: carrier.ID, WorkspaceID: workspaceID, RuntimeID: runtime.ID, Model: pgtype.Text{},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, switchAgentBuilderRuntimeResponse{RuntimeID: uuidString(updated.RuntimeID)})
}
