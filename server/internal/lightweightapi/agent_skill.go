package lightweightapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/agentconfigsecret"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

type invocationTargetRequest struct {
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
}

type invocationTargetResponse struct {
	ID         string `json:"id"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
}

type agentSkillResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type agentResponse struct {
	ID                    string                     `json:"id"`
	WorkspaceID           string                     `json:"workspace_id"`
	RuntimeID             *string                    `json:"runtime_id"`
	OwnerID               string                     `json:"owner_id"`
	Name                  string                     `json:"name"`
	Description           string                     `json:"description"`
	Instructions          string                     `json:"instructions"`
	AvatarURL             *string                    `json:"avatar_url"`
	RuntimeConfig         json.RawMessage            `json:"runtime_config"`
	Status                string                     `json:"status"`
	MaxConcurrentTasks    int32                      `json:"max_concurrent_tasks"`
	CustomEnvKeys         []string                   `json:"custom_env_keys"`
	CustomArgs            json.RawMessage            `json:"custom_args"`
	MCPConfigured         bool                       `json:"mcp_configured"`
	Model                 *string                    `json:"model"`
	ThinkingLevel         *string                    `json:"thinking_level"`
	ServiceTier           *string                    `json:"service_tier"`
	PermissionMode        string                     `json:"permission_mode"`
	DisabledRuntimeSkills json.RawMessage            `json:"disabled_runtime_skills"`
	ArchivedAt            *string                    `json:"archived_at"`
	ArchivedBy            *string                    `json:"archived_by"`
	Skills                []agentSkillResponse       `json:"skills"`
	InvocationTargets     []invocationTargetResponse `json:"invocation_targets"`
	CreatedAt             string                     `json:"created_at"`
	UpdatedAt             string                     `json:"updated_at"`
}

func (h *Handler) canManage(r *http.Request, ownerID pgtype.UUID) bool {
	member, ok := MemberFromContext(r.Context())
	if ok && (member.Role == "owner" || member.Role == "admin") {
		return true
	}
	principal, ok := principalFromContext(r.Context())
	return ok && principal.UserID == uuidString(ownerID)
}

func (h *Handler) agentResponse(ctx *http.Request, agent lwdb.Agent) (agentResponse, error) {
	var runtimeID *string
	if agent.RuntimeID.Valid {
		value := uuidString(agent.RuntimeID)
		runtimeID = &value
	}
	envKeys := []string{}
	if len(agent.CustomEnv) > 0 {
		keys, err := customEnvKeys(agent.CustomEnv)
		if err != nil {
			return agentResponse{}, err
		}
		envKeys = keys
	}
	mcpConfigured := len(agent.McpConfig) > 0 && string(agent.McpConfig) != "{}"
	if h.cfg.AgentSecrets != nil && mcpConfigured {
		if plaintext, err := h.cfg.AgentSecrets.DecryptMCPConfig(agent.McpConfig); err == nil {
			mcpConfigured = string(plaintext) != "{}"
		}
	}
	skills, err := h.q.ListAgentSkills(ctx.Context(), agent.ID)
	if err != nil {
		return agentResponse{}, err
	}
	skillResponse := make([]agentSkillResponse, 0, len(skills))
	for _, skill := range skills {
		skillResponse = append(skillResponse, agentSkillResponse{ID: uuidString(skill.ID), Name: skill.Name, Enabled: skill.Enabled})
	}
	targets, err := h.q.ListAgentInvocationTargets(ctx.Context(), agent.ID)
	if err != nil {
		return agentResponse{}, err
	}
	targetResponse := make([]invocationTargetResponse, 0, len(targets))
	for _, target := range targets {
		targetResponse = append(targetResponse, invocationTargetResponse{ID: uuidString(target.ID), TargetType: target.TargetType, TargetID: uuidString(target.TargetID)})
	}
	return agentResponse{
		ID: uuidString(agent.ID), WorkspaceID: uuidString(agent.WorkspaceID), RuntimeID: runtimeID,
		OwnerID: uuidString(agent.OwnerID), Name: agent.Name, Description: agent.Description, Instructions: agent.Instructions,
		AvatarURL: textPointer(agent.AvatarUrl), RuntimeConfig: agent.RuntimeConfig, Status: agent.Status, MaxConcurrentTasks: agent.MaxConcurrentTasks,
		CustomEnvKeys: envKeys, CustomArgs: agent.CustomArgs, MCPConfigured: mcpConfigured,
		Model: textPointer(agent.Model), ThinkingLevel: textPointer(agent.ThinkingLevel), ServiceTier: textPointer(agent.ServiceTier),
		PermissionMode: agent.PermissionMode, DisabledRuntimeSkills: agent.DisabledRuntimeSkills,
		ArchivedAt: timePointer(agent.ArchivedAt), ArchivedBy: optionalUUIDPointer(agent.ArchivedBy),
		Skills: skillResponse, InvocationTargets: targetResponse,
		CreatedAt: timeString(agent.CreatedAt), UpdatedAt: timeString(agent.UpdatedAt),
	}, nil
}

func customEnvKeys(document []byte) ([]string, error) {
	if string(document) == "{}" {
		return []string{}, nil
	}
	return agentConfigKeys(document)
}

func agentConfigKeys(document []byte) ([]string, error) {
	return agentconfigsecret.CustomEnvKeys(document)
}

type agentMutation struct {
	Name                  *string                   `json:"name"`
	Description           *string                   `json:"description"`
	Instructions          *string                   `json:"instructions"`
	RuntimeID             json.RawMessage           `json:"runtime_id"`
	Model                 json.RawMessage           `json:"model"`
	ThinkingLevel         json.RawMessage           `json:"thinking_level"`
	ServiceTier           json.RawMessage           `json:"service_tier"`
	RuntimeConfig         json.RawMessage           `json:"runtime_config"`
	CustomArgs            json.RawMessage           `json:"custom_args"`
	MCPConfig             json.RawMessage           `json:"mcp_config"`
	MaxConcurrentTasks    *int32                    `json:"max_concurrent_tasks"`
	PermissionMode        *string                   `json:"permission_mode"`
	InvocationTargets     []invocationTargetRequest `json:"invocation_targets"`
	DisabledRuntimeSkills json.RawMessage           `json:"disabled_runtime_skills"`
}

func (h *Handler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	principal, _ := principalFromContext(r.Context())
	ownerID, _ := parseUUID(principal.UserID)
	var request agentMutation
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Name == nil || request.RuntimeID == nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	*request.Name = strings.TrimSpace(*request.Name)
	if *request.Name == "" || len(*request.Name) > 200 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if h.cfg.AgentSecrets == nil {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}
	runtimeID, ok := requiredUUID(request.RuntimeID)
	if !ok {
		writeCode(w, http.StatusBadRequest, "agent_runtime_required")
		return
	}
	if _, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID}); err != nil {
		writeCode(w, http.StatusBadRequest, "agent_runtime_required")
		return
	}
	if !validateAgentJSON(&request) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	encryptedEnv, err := h.cfg.AgentSecrets.EncryptCustomEnv(map[string]string{})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	mcp := request.MCPConfig
	if mcp == nil {
		mcp = json.RawMessage(`{}`)
	}
	encryptedMCP, err := h.cfg.AgentSecrets.EncryptMCPConfig(mcp)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	params := createAgentParams(request, workspaceID, runtimeID, ownerID, encryptedEnv, encryptedMCP)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	if _, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID}); err != nil {
		writeCode(w, http.StatusBadRequest, "agent_runtime_required")
		return
	}
	agent, err := qtx.CreateAgent(r.Context(), params)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := replaceInvocationTargets(r, qtx, agent, ownerID, request.InvocationTargets); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response, err := h.agentResponse(r, agent)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventAgentCreated, uuidString(workspaceID), "member", principal.UserID, map[string]any{"agent": broadcastAgentSummary(response)})
	writeJSON(w, http.StatusCreated, response)
}

func createAgentParams(request agentMutation, workspaceID, runtimeID, ownerID pgtype.UUID, encryptedEnv, encryptedMCP []byte) lwdb.CreateAgentParams {
	maxTasks := int32(1)
	if request.MaxConcurrentTasks != nil {
		maxTasks = *request.MaxConcurrentTasks
	}
	permission := "private"
	if request.PermissionMode != nil {
		permission = *request.PermissionMode
	}
	description, instructions := "", ""
	if request.Description != nil {
		description = *request.Description
	}
	if request.Instructions != nil {
		instructions = *request.Instructions
	}
	return lwdb.CreateAgentParams{
		WorkspaceID: workspaceID, RuntimeID: runtimeID, OwnerID: ownerID, Name: *request.Name,
		Description: description, Instructions: instructions, RuntimeConfig: jsonDefault(request.RuntimeConfig, `{}`),
		MaxConcurrentTasks: maxTasks, CustomEnv: encryptedEnv, CustomArgs: jsonDefault(request.CustomArgs, `[]`),
		McpConfig: encryptedMCP, Model: rawOptionalText(request.Model), ThinkingLevel: rawOptionalText(request.ThinkingLevel),
		ServiceTier: rawOptionalText(request.ServiceTier), PermissionMode: permission,
		DisabledRuntimeSkills: jsonDefault(request.DisabledRuntimeSkills, `[]`),
	}
}

func validateAgentJSON(request *agentMutation) bool {
	if request.MaxConcurrentTasks != nil && *request.MaxConcurrentTasks < 1 {
		return false
	}
	if request.PermissionMode != nil && *request.PermissionMode != "private" && *request.PermissionMode != "public_to" {
		return false
	}
	for _, field := range []struct {
		raw       json.RawMessage
		firstByte byte
	}{
		{request.RuntimeConfig, '{'},
		{request.CustomArgs, '['},
		{request.MCPConfig, '{'},
		{request.DisabledRuntimeSkills, '['},
	} {
		if field.raw == nil {
			continue
		}
		trimmed := bytes.TrimSpace(field.raw)
		if !json.Valid(trimmed) || len(trimmed) == 0 || trimmed[0] != field.firstByte {
			return false
		}
	}
	for _, raw := range []json.RawMessage{request.Model, request.ThinkingLevel, request.ServiceTier} {
		if raw == nil {
			continue
		}
		if _, ok := optionalText(raw); !ok {
			return false
		}
	}
	return true
}

func (h *Handler) ListAgents(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	rows, err := h.q.ListAgents(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]agentResponse, 0, len(rows))
	for _, row := range rows {
		item, err := h.agentResponse(r, row)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		response = append(response, item)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	response, err := h.agentResponse(r, agent)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if agent.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "agent_archived")
		return
	}
	var request agentMutation
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || !validateAgentJSON(&request) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if request.Name != nil {
		*request.Name = strings.TrimSpace(*request.Name)
		if *request.Name == "" || len(*request.Name) > 200 {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	params := updateAgentParams(request, agent)
	if request.RuntimeID != nil {
		runtimeID, valid := requiredUUID(request.RuntimeID)
		if !valid {
			writeCode(w, http.StatusBadRequest, "agent_runtime_required")
			return
		}
		if _, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: agent.WorkspaceID}); err != nil {
			writeCode(w, http.StatusBadRequest, "agent_runtime_required")
			return
		}
		params.SetRuntimeID, params.RuntimeID = true, runtimeID
	}
	if request.MCPConfig != nil {
		if h.cfg.AgentSecrets == nil {
			writeCode(w, http.StatusServiceUnavailable, "internal_error")
			return
		}
		encrypted, err := h.cfg.AgentSecrets.EncryptMCPConfig(request.MCPConfig)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.SetMcpConfig, params.McpConfig = true, encrypted
	}
	principal, _ := principalFromContext(r.Context())
	actorID, _ := parseUUID(principal.UserID)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	var selectedRuntime lwdb.AgentRuntime
	if params.SetRuntimeID {
		selectedRuntime, err = qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: params.RuntimeID, WorkspaceID: agent.WorkspaceID})
		if err != nil {
			writeCode(w, http.StatusBadRequest, "agent_runtime_required")
			return
		}
	}
	lockedAgent, err := qtx.LockAgent(r.Context(), lwdb.LockAgentParams{ID: agent.ID, WorkspaceID: agent.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, lockedAgent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if lockedAgent.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "agent_archived")
		return
	}
	if params.SetRuntimeID {
		if _, headErr := qtx.GetAgentToolBundleHead(r.Context(), lwdb.GetAgentToolBundleHeadParams{WorkspaceID: agent.WorkspaceID, AgentID: agent.ID}); headErr == nil {
			if h.cfg.GatewayProviderPolicy == nil || !h.cfg.GatewayProviderPolicy.ProviderSupported(selectedRuntime.Provider) {
				writeCode(w, http.StatusConflict, "provider_mcp_unsupported")
				return
			}
		} else if !errors.Is(headErr, pgx.ErrNoRows) {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	updated, err := qtx.UpdateAgent(r.Context(), params)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if request.InvocationTargets != nil {
		if err := replaceInvocationTargets(r, qtx, updated, actorID, request.InvocationTargets); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response, err := h.agentResponse(r, updated)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventAgentUpdated, uuidString(agent.WorkspaceID), "member", principal.UserID, map[string]any{"agent": broadcastAgentSummary(response)})
	writeJSON(w, http.StatusOK, response)
}

func updateAgentParams(request agentMutation, agent lwdb.Agent) lwdb.UpdateAgentParams {
	params := lwdb.UpdateAgentParams{ID: agent.ID, WorkspaceID: agent.WorkspaceID}
	if request.Name != nil {
		value := strings.TrimSpace(*request.Name)
		params.SetName, params.Name = true, value
	}
	if request.Description != nil {
		params.SetDescription, params.Description = true, *request.Description
	}
	if request.Instructions != nil {
		params.SetInstructions, params.Instructions = true, *request.Instructions
	}
	if request.RuntimeConfig != nil {
		params.SetRuntimeConfig, params.RuntimeConfig = true, request.RuntimeConfig
	}
	if request.CustomArgs != nil {
		params.SetCustomArgs, params.CustomArgs = true, request.CustomArgs
	}
	if request.Model != nil {
		params.SetModel, params.Model = true, rawOptionalText(request.Model)
	}
	if request.ThinkingLevel != nil {
		params.SetThinkingLevel, params.ThinkingLevel = true, rawOptionalText(request.ThinkingLevel)
	}
	if request.ServiceTier != nil {
		params.SetServiceTier, params.ServiceTier = true, rawOptionalText(request.ServiceTier)
	}
	if request.MaxConcurrentTasks != nil {
		params.SetMaxConcurrentTasks, params.MaxConcurrentTasks = true, *request.MaxConcurrentTasks
	}
	if request.PermissionMode != nil {
		params.SetPermissionMode, params.PermissionMode = true, *request.PermissionMode
	}
	if request.DisabledRuntimeSkills != nil {
		params.SetDisabledRuntimeSkills, params.DisabledRuntimeSkills = true, request.DisabledRuntimeSkills
	}
	return params
}

func (h *Handler) ArchiveAgent(w http.ResponseWriter, r *http.Request) {
	h.setAgentArchived(w, r, true)
}

func (h *Handler) RestoreAgent(w http.ResponseWriter, r *http.Request) {
	h.setAgentArchived(w, r, false)
}

func (h *Handler) setAgentArchived(w http.ResponseWriter, r *http.Request, archive bool) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	if !archive {
		if !agent.RuntimeID.Valid {
			writeCode(w, http.StatusConflict, "agent_runtime_required")
			return
		}
		if _, err := qtx.LockAgentRuntime(r.Context(), lwdb.LockAgentRuntimeParams{ID: agent.RuntimeID, WorkspaceID: agent.WorkspaceID}); err != nil {
			writeCode(w, http.StatusConflict, "agent_runtime_required")
			return
		}
	}
	agent, err = qtx.LockAgent(r.Context(), lwdb.LockAgentParams{ID: agent.ID, WorkspaceID: agent.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if archive {
		if agent.ArchivedAt.Valid {
			writeCode(w, http.StatusConflict, "agent_archived")
			return
		}
		cancelled, err := h.cancelAgentTasksTx(r.Context(), qtx, agent.WorkspaceID, agent.ID)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		principal, _ := principalFromContext(r.Context())
		actorID, _ := parseUUID(principal.UserID)
		agent, err = qtx.ArchiveAgent(r.Context(), lwdb.ArchiveAgentParams{ActorID: actorID, ID: agent.ID, WorkspaceID: agent.WorkspaceID})
		if err != nil {
			writeCode(w, http.StatusConflict, "conflict")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		response, err := h.agentResponse(r, agent)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		h.publish(protocol.EventAgentArchived, uuidString(agent.WorkspaceID), "member", principal.UserID, map[string]any{"agent": broadcastAgentSummary(response)})
		for _, task := range cancelled {
			h.publish(protocol.EventTaskCancelled, uuidString(task.WorkspaceID), "member", principal.UserID, taskLifecycleDTO(task))
			if initiator := cancelledChatInitiator(r.Context(), h.q, task); initiator != "" {
				h.publishCancelledChatFinalized(task, initiator)
			}
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if !agent.ArchivedAt.Valid || !agent.RuntimeID.Valid {
		writeCode(w, http.StatusConflict, "agent_runtime_required")
		return
	}
	agent, err = qtx.RestoreAgent(r.Context(), lwdb.RestoreAgentParams{ID: agent.ID, WorkspaceID: agent.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
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
	h.publish(protocol.EventAgentRestored, uuidString(agent.WorkspaceID), "member", principal.UserID, map[string]any{"agent": broadcastAgentSummary(response)})
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) loadAgent(w http.ResponseWriter, r *http.Request) (lwdb.Agent, bool) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return lwdb.Agent{}, false
	}
	agentID, err := parseUUID(chi.URLParam(r, "agentId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Agent{}, false
	}
	agent, err := h.q.GetAgent(r.Context(), lwdb.GetAgentParams{ID: agentID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Agent{}, false
	}
	return agent, true
}

type envUpdateRequest struct {
	CustomEnv map[string]string `json:"custom_env"`
}

func (h *Handler) GetAgentEnv(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.cfg.AgentSecrets == nil {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}
	values, err := h.cfg.AgentSecrets.DecryptCustomEnv(agent.CustomEnv)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := h.audit(r, h.q, agent.WorkspaceID, "agent.env.reveal", map[string]any{"agent_id": uuidString(agent.ID), "keys": mapKeys(values)}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"custom_env": values})
}

func (h *Handler) UpdateAgentEnv(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if h.cfg.AgentSecrets == nil {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}
	var request envUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.CustomEnv == nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	encrypted, err := h.cfg.AgentSecrets.EncryptCustomEnv(request.CustomEnv)
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
	updated, err := qtx.UpdateAgentCustomEnv(r.Context(), lwdb.UpdateAgentCustomEnvParams{CustomEnv: encrypted, ID: agent.ID, WorkspaceID: agent.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := h.audit(r, qtx, agent.WorkspaceID, "agent.env.update", map[string]any{"agent_id": uuidString(agent.ID), "keys": mapKeys(request.CustomEnv)}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	redacted := make(map[string]string, len(request.CustomEnv))
	for key := range request.CustomEnv {
		redacted[key] = "****"
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": uuidString(updated.ID), "custom_env": redacted})
}

func (h *Handler) audit(r *http.Request, q *lwdb.Queries, workspaceID pgtype.UUID, action string, details any) error {
	principal, _ := principalFromContext(r.Context())
	actorID, _ := parseUUID(principal.UserID)
	payload, _ := json.Marshal(details)
	_, err := q.CreateActivity(r.Context(), lwdb.CreateActivityParams{
		WorkspaceID: workspaceID, ActorType: "member", ActorID: actorID, Action: action, Details: payload,
	})
	return err
}

type skillResponse struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Content     string          `json:"content"`
	Config      json.RawMessage `json:"config"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

func toSkillResponse(skill lwdb.Skill) skillResponse {
	return skillResponse{ID: uuidString(skill.ID), WorkspaceID: uuidString(skill.WorkspaceID), Name: skill.Name,
		Description: skill.Description, Content: skill.Content, Config: skill.Config, CreatedBy: uuidString(skill.CreatedBy),
		CreatedAt: timeString(skill.CreatedAt), UpdatedAt: timeString(skill.UpdatedAt)}
}

type skillMutation struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Content     *string         `json:"content"`
	Config      json.RawMessage `json:"config"`
}

func (h *Handler) CreateSkill(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	principal, _ := principalFromContext(r.Context())
	creatorID, _ := parseUUID(principal.UserID)
	var request skillMutation
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Name == nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	*request.Name = strings.TrimSpace(*request.Name)
	if *request.Name == "" {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	description, content := "", ""
	if request.Description != nil {
		description = *request.Description
	}
	if request.Content != nil {
		content = *request.Content
	}
	if request.Config != nil && !json.Valid(request.Config) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	skill, err := h.q.CreateSkill(r.Context(), lwdb.CreateSkillParams{WorkspaceID: workspaceID, Name: *request.Name,
		Description: description, Content: content, Config: jsonDefault(request.Config, `{}`), CreatedBy: creatorID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, toSkillResponse(skill))
}

func (h *Handler) ListSkills(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	rows, err := h.q.ListSkills(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]skillResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toSkillResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetSkill(w http.ResponseWriter, r *http.Request) {
	skill, ok := h.loadSkill(w, r)
	if ok {
		writeJSON(w, http.StatusOK, toSkillResponse(skill))
	}
}

func (h *Handler) UpdateSkill(w http.ResponseWriter, r *http.Request) {
	skill, ok := h.loadSkill(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, skill.CreatedBy) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	var request skillMutation
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || (request.Config != nil && !json.Valid(request.Config)) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	params := lwdb.UpdateSkillParams{ID: skill.ID, WorkspaceID: skill.WorkspaceID}
	if request.Name != nil {
		value := strings.TrimSpace(*request.Name)
		if value == "" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.SetName, params.Name = true, value
	}
	if request.Description != nil {
		params.SetDescription, params.Description = true, *request.Description
	}
	if request.Content != nil {
		params.SetContent, params.Content = true, *request.Content
	}
	if request.Config != nil {
		params.SetConfig, params.Config = true, request.Config
	}
	updated, err := h.q.UpdateSkill(r.Context(), params)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, toSkillResponse(updated))
}

func (h *Handler) DeleteSkill(w http.ResponseWriter, r *http.Request) {
	skill, ok := h.loadSkill(w, r)
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
	skill, err = qtx.LockSkill(r.Context(), lwdb.LockSkillParams{ID: skill.ID, WorkspaceID: skill.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, skill.CreatedBy) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	bindings, err := qtx.CountAgentSkillBindings(r.Context(), skill.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if bindings > 0 {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	if _, err := qtx.DeleteSkillFilesBySkill(r.Context(), skill.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	deleted, err := qtx.DeleteSkill(r.Context(), lwdb.DeleteSkillParams{ID: skill.ID, WorkspaceID: skill.WorkspaceID})
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

type skillFileRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type skillFileResponse struct {
	ID        string `json:"id"`
	SkillID   string `json:"skill_id"`
	Path      string `json:"path"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func toSkillFileResponse(file lwdb.SkillFile) skillFileResponse {
	return skillFileResponse{ID: uuidString(file.ID), SkillID: uuidString(file.SkillID), Path: file.Path,
		Content: file.Content, CreatedAt: timeString(file.CreatedAt), UpdatedAt: timeString(file.UpdatedAt)}
}

func (h *Handler) ListSkillFiles(w http.ResponseWriter, r *http.Request) {
	skill, ok := h.loadSkill(w, r)
	if !ok {
		return
	}
	rows, err := h.q.ListSkillFiles(r.Context(), skill.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]skillFileResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toSkillFileResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) UpsertSkillFiles(w http.ResponseWriter, r *http.Request) {
	skill, ok := h.loadSkill(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, skill.CreatedBy) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	var request struct {
		Files []skillFileRequest `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	seen := make(map[string]struct{}, len(request.Files))
	for i := range request.Files {
		request.Files[i].Path = path.Clean(strings.TrimSpace(request.Files[i].Path))
		if request.Files[i].Path == "." || strings.HasPrefix(request.Files[i].Path, "/") || strings.HasPrefix(request.Files[i].Path, "../") {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		if _, duplicate := seen[request.Files[i].Path]; duplicate {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		seen[request.Files[i].Path] = struct{}{}
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	lockedSkill, err := qtx.LockSkill(r.Context(), lwdb.LockSkillParams{ID: skill.ID, WorkspaceID: skill.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, lockedSkill.CreatedBy) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if _, err := qtx.DeleteSkillFilesBySkill(r.Context(), skill.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]skillFileResponse, 0, len(request.Files))
	for _, file := range request.Files {
		row, err := qtx.UpsertSkillFile(r.Context(), lwdb.UpsertSkillFileParams{SkillID: skill.ID, Path: file.Path, Content: file.Content})
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		response = append(response, toSkillFileResponse(row))
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) DeleteSkillFile(w http.ResponseWriter, r *http.Request) {
	skill, ok := h.loadSkill(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, skill.CreatedBy) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	fileID, err := parseUUID(chi.URLParam(r, "fileId"))
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
	lockedSkill, err := qtx.LockSkill(r.Context(), lwdb.LockSkillParams{ID: skill.ID, WorkspaceID: skill.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, lockedSkill.CreatedBy) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	deleted, err := qtx.DeleteSkillFile(r.Context(), lwdb.DeleteSkillFileParams{ID: fileID, SkillID: skill.ID})
	if err != nil || deleted != 1 {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListAgentSkills(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	rows, err := h.q.ListAgentSkills(r.Context(), agent.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]agentSkillResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, agentSkillResponse{ID: uuidString(row.ID), Name: row.Name, Enabled: row.Enabled})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) SetAgentSkills(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	var request struct {
		Skills []struct {
			SkillID string `json:"skill_id"`
			Enabled bool   `json:"enabled"`
		} `json:"skills"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	type parsedBinding struct {
		id      pgtype.UUID
		enabled bool
	}
	parsed := make([]parsedBinding, 0, len(request.Skills))
	seen := make(map[string]struct{}, len(request.Skills))
	for _, binding := range request.Skills {
		skillID, err := parseUUID(binding.SkillID)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		key := uuidString(skillID)
		if _, duplicate := seen[key]; duplicate {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		seen[key] = struct{}{}
		parsed = append(parsed, parsedBinding{id: skillID, enabled: binding.Enabled})
	}
	sort.Slice(parsed, func(i, j int) bool { return uuidString(parsed[i].id) < uuidString(parsed[j].id) })
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	lockedAgent, err := qtx.LockAgent(r.Context(), lwdb.LockAgentParams{ID: agent.ID, WorkspaceID: agent.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if !h.canManage(r, lockedAgent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if lockedAgent.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "agent_archived")
		return
	}
	for _, binding := range parsed {
		if _, err := qtx.LockSkill(r.Context(), lwdb.LockSkillParams{ID: binding.id, WorkspaceID: agent.WorkspaceID}); err != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
	}
	if _, err := qtx.DeleteAgentSkills(r.Context(), agent.ID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	for _, binding := range parsed {
		if _, err := qtx.ReplaceAgentSkill(r.Context(), lwdb.ReplaceAgentSkillParams{AgentID: agent.ID, SkillID: binding.id, Enabled: binding.enabled}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.ListAgentSkills(w, r)
	principal, _ := principalFromContext(r.Context())
	h.publish(protocol.EventAgentUpdated, uuidString(agent.WorkspaceID), "member", principal.UserID, map[string]any{"agent_id": uuidString(agent.ID)})
}

func (h *Handler) loadSkill(w http.ResponseWriter, r *http.Request) (lwdb.Skill, bool) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return lwdb.Skill{}, false
	}
	skillID, err := parseUUID(chi.URLParam(r, "skillId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Skill{}, false
	}
	skill, err := h.q.GetSkill(r.Context(), lwdb.GetSkillParams{ID: skillID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Skill{}, false
	}
	return skill, true
}

func replaceInvocationTargets(r *http.Request, q *lwdb.Queries, agent lwdb.Agent, actorID pgtype.UUID, targets []invocationTargetRequest) error {
	if _, err := q.DeleteAgentInvocationTargets(r.Context(), agent.ID); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		targetID, err := parseUUID(target.TargetID)
		if err != nil {
			return err
		}
		key := target.TargetType + ":" + target.TargetID
		if _, duplicate := seen[key]; duplicate {
			return pgx.ErrNoRows
		}
		seen[key] = struct{}{}
		switch target.TargetType {
		case "workspace":
			if targetID != agent.WorkspaceID {
				return pgx.ErrNoRows
			}
		case "member":
			if _, err := q.GetMemberByID(r.Context(), lwdb.GetMemberByIDParams{ID: targetID, WorkspaceID: agent.WorkspaceID}); err != nil {
				return err
			}
		default:
			return pgx.ErrNoRows
		}
		if _, err := q.CreateAgentInvocationTarget(r.Context(), lwdb.CreateAgentInvocationTargetParams{
			AgentID: agent.ID, TargetType: target.TargetType, TargetID: targetID, CreatedBy: actorID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func requiredUUID(raw json.RawMessage) (pgtype.UUID, bool) {
	if raw == nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return pgtype.UUID{}, false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return pgtype.UUID{}, false
	}
	parsed, err := parseUUID(value)
	return parsed, err == nil
}

func rawOptionalText(raw json.RawMessage) pgtype.Text {
	value, _ := optionalText(raw)
	return value
}

func jsonDefault(raw json.RawMessage, fallback string) []byte {
	if raw == nil {
		return []byte(fallback)
	}
	return raw
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
