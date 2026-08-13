package lightweightapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/auth"
	"github.com/kms9/dars/internal/daemonws"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	prepareLeaseDuration = 2 * time.Minute
	taskTokenTTL         = 24 * time.Hour
	maxBatchClaim        = 32
)

type claimRequest struct {
	DaemonID   string   `json:"daemon_id"`
	RuntimeIDs []string `json:"runtime_ids"`
	MaxTasks   int32    `json:"max_tasks"`
}

type claimSkillFile struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type claimSkill struct {
	ID          string           `json:"id"`
	Source      string           `json:"source"`
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Hash        string           `json:"hash,omitempty"`
	SizeBytes   int64            `json:"size_bytes,omitempty"`
	Content     string           `json:"content"`
	Files       []claimSkillFile `json:"files,omitempty"`
}

type claimAgent struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	Instructions          string            `json:"instructions"`
	Skills                []claimSkill      `json:"skills,omitempty"`
	CustomEnv             map[string]string `json:"custom_env,omitempty"`
	CustomArgs            json.RawMessage   `json:"custom_args,omitempty"`
	MCPConfig             json.RawMessage   `json:"mcp_config,omitempty"`
	Model                 string            `json:"model,omitempty"`
	ThinkingLevel         string            `json:"thinking_level,omitempty"`
	ServiceTier           string            `json:"service_tier,omitempty"`
	PermissionMode        string            `json:"permission_mode"`
	DisabledRuntimeSkills json.RawMessage   `json:"disabled_runtime_skills,omitempty"`
	RuntimeConfig         json.RawMessage   `json:"runtime_config,omitempty"`
}

type claimSquadMember struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

type claimSquad struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Instructions string             `json:"instructions,omitempty"`
	Members      []claimSquadMember `json:"members"`
}

type claimedTask struct {
	ID                string          `json:"id"`
	AgentID           string          `json:"agent_id"`
	RuntimeID         string          `json:"runtime_id"`
	IssueID           string          `json:"issue_id,omitempty"`
	ChatSessionID     string          `json:"chat_session_id,omitempty"`
	SquadID           string          `json:"squad_id,omitempty"`
	Squad             *claimSquad     `json:"squad,omitempty"`
	WorkspaceID       string          `json:"workspace_id"`
	WorkspaceContext  string          `json:"workspace_context,omitempty"`
	Context           json.RawMessage `json:"context,omitempty"`
	Repos             json.RawMessage `json:"repos,omitempty"`
	IsLeaderTask      bool            `json:"is_leader_task,omitempty"`
	ForceFreshSession bool            `json:"force_fresh_session,omitempty"`
	HandoffNote       string          `json:"handoff_note,omitempty"`
	ThreadName        string          `json:"thread_name,omitempty"`
	PriorSessionID    string          `json:"prior_session_id,omitempty"`
	PriorWorkDir      string          `json:"prior_work_dir,omitempty"`
	ChatMessage       string          `json:"chat_message,omitempty"`
	CommentIDs        []string        `json:"comment_ids,omitempty"`
	AuthToken         string          `json:"auth_token"`
	Agent             claimAgent      `json:"agent"`
}

// ClaimTasks Daemon 批量领取任务（Bearer ddt_）。
// 调用 ClaimTasksForRuntimes 原子 claim → 对每条任务 buildClaimedTask
// （组装 Agent/skills/小队花名册并签发 dat_）→ publish task:dispatch。
func (h *Handler) ClaimTasks(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if h.cfg.AgentSecrets == nil {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}
	var request claimRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	request.DaemonID = strings.TrimSpace(request.DaemonID)
	if request.DaemonID != principal.DaemonID || len(request.RuntimeIDs) == 0 || request.MaxTasks < 1 || request.MaxTasks > maxBatchClaim {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	workspaceID, err := parseUUID(principal.WorkspaceID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	runtimeIDs := make([]pgtype.UUID, 0, len(request.RuntimeIDs))
	seen := make(map[string]struct{}, len(request.RuntimeIDs))
	for _, rawID := range request.RuntimeIDs {
		rawID = strings.TrimSpace(rawID)
		if _, duplicate := seen[rawID]; duplicate {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		seen[rawID] = struct{}{}
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
		if runtime.Status != "online" {
			writeCode(w, http.StatusConflict, "runtime_unavailable")
			return
		}
		runtimeIDs = append(runtimeIDs, runtimeID)
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	rows, err := qtx.ClaimTasksForRuntimes(r.Context(), lwdb.ClaimTasksForRuntimesParams{
		WorkspaceID: workspaceID, RuntimeIds: runtimeIDs, ClaimLimit: request.MaxTasks,
		PrepareLeaseExpiresAt: pgtype.Timestamptz{Time: h.now().Add(prepareLeaseDuration), Valid: true},
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]claimedTask, 0, len(rows))
	for index, task := range rows {
		delivered, err := qtx.SetTaskDeliveredComments(r.Context(), lwdb.SetTaskDeliveredCommentsParams{ID: task.ID, RuntimeID: task.RuntimeID})
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		rows[index] = delivered
		task = delivered
		claimed, err := h.buildClaimedTask(r, qtx, task)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		response = append(response, claimed)
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	for _, task := range rows {
		h.publish(protocol.EventTaskDispatch, uuidString(task.WorkspaceID), "system", "", taskLifecycleDTO(task))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": response})
}

// buildClaimedTask 组装 Daemon 执行所需的 claim 载荷：解密 env/MCP、加载 skill bundle、
// 签发 task token（dat_）。若 task.SquadID 有效，附带小队 instructions 与成员花名册，
// 并校验 is_leader_task 时 agent 必须是 squad.LeaderID。
func (h *Handler) buildClaimedTask(r *http.Request, q *lwdb.Queries, task lwdb.AgentTaskQueue) (claimedTask, error) {
	agent, err := q.GetAgent(r.Context(), lwdb.GetAgentParams{ID: task.AgentID, WorkspaceID: task.WorkspaceID})
	if err != nil || agent.ArchivedAt.Valid || !agent.RuntimeID.Valid || agent.RuntimeID != task.RuntimeID {
		return claimedTask{}, errClaimContract
	}
	workspace, err := q.GetWorkspace(r.Context(), task.WorkspaceID)
	if err != nil {
		return claimedTask{}, err
	}
	customEnv, err := h.cfg.AgentSecrets.DecryptCustomEnv(agent.CustomEnv)
	if err != nil {
		return claimedTask{}, err
	}
	var mcpConfig json.RawMessage
	if !task.ToolBundleID.Valid {
		mcpConfig, err = h.cfg.AgentSecrets.DecryptMCPConfig(agent.McpConfig)
		if err != nil {
			return claimedTask{}, err
		}
	}
	skills, err := loadAgentSkillBundles(r.Context(), q, agent.ID)
	if err != nil {
		return claimedTask{}, err
	}
	claimSkills := make([]claimSkill, 0, len(skills))
	for _, skill := range skills {
		claimFiles := make([]claimSkillFile, 0, len(skill.Files))
		for _, file := range skill.Files {
			claimFiles = append(claimFiles, claimSkillFile{Path: file.Path, Content: file.Content, SHA256: file.SHA256, SizeBytes: file.SizeBytes})
		}
		claimSkills = append(claimSkills, claimSkill{
			ID: skill.ID, Source: skill.Source, Name: skill.Name, Description: skill.Description,
			Hash: skill.Hash, SizeBytes: skill.SizeBytes, Content: skill.Content, Files: claimFiles,
		})
	}
	plaintextToken, err := auth.GenerateAgentTaskToken()
	if err != nil {
		return claimedTask{}, err
	}
	userID := agent.OwnerID
	for _, candidate := range []pgtype.UUID{task.AccountableUserID, task.OriginatorUserID, task.InitiatorUserID} {
		if candidate.Valid {
			userID = candidate
			break
		}
	}
	if _, err := q.DeleteTaskTokensByTask(r.Context(), task.ID); err != nil {
		return claimedTask{}, err
	}
	if _, err := q.CreateTaskToken(r.Context(), lwdb.CreateTaskTokenParams{
		TokenHash: auth.HashToken(plaintextToken), TaskID: task.ID, AgentID: task.AgentID,
		WorkspaceID: task.WorkspaceID, UserID: userID, ToolBundleID: task.ToolBundleID,
		ExpiresAt: pgtype.Timestamptz{Time: h.now().Add(taskTokenTTL), Valid: true},
	}); err != nil {
		return claimedTask{}, err
	}
	if task.ToolBundleID.Valid {
		if h.cfg.ClaimMCPProjector == nil {
			return claimedTask{}, errClaimContract
		}
		bundleClaim, err := q.GetTaskBundleClaim(r.Context(), lwdb.GetTaskBundleClaimParams{
			TaskID: task.ID, WorkspaceID: task.WorkspaceID, AgentID: task.AgentID,
		})
		if err != nil || bundleClaim.BundleStatus != "active" || !bundleClaim.ToolBundleID.Valid || bundleClaim.ToolBundleID.String != task.ToolBundleID.String {
			return claimedTask{}, errClaimContract
		}
		runtime, err := q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: task.RuntimeID, WorkspaceID: task.WorkspaceID})
		if err != nil {
			return claimedTask{}, errClaimContract
		}
		mcpConfig, err = h.cfg.ClaimMCPProjector.ProjectClaimMCPConfig(task.ToolBundleID.String, runtime.Provider, plaintextToken)
		if err != nil {
			return claimedTask{}, errClaimContract
		}
	}
	var threadName, chatMessage string
	if task.ChatSessionID.Valid {
		session, err := q.GetChatSession(r.Context(), lwdb.GetChatSessionParams{ID: task.ChatSessionID, WorkspaceID: task.WorkspaceID})
		if err != nil || session.AgentID != task.AgentID {
			return claimedTask{}, errClaimContract
		}
		inputTaskID := task.ID
		if task.ChatInputTaskID.Valid {
			inputTaskID = task.ChatInputTaskID
		}
		message, err := q.GetChatMessageByTaskID(r.Context(), lwdb.GetChatMessageByTaskIDParams{
			ChatSessionID: task.ChatSessionID, TaskID: inputTaskID, Role: "user",
		})
		if err != nil {
			return claimedTask{}, errClaimContract
		}
		threadName, chatMessage = session.Title, message.Content
	}
	var squadContext *claimSquad
	if task.SquadID.Valid {
		squad, err := q.GetSquad(r.Context(), lwdb.GetSquadParams{ID: task.SquadID, WorkspaceID: task.WorkspaceID})
		if err != nil || squad.ArchivedAt.Valid {
			return claimedTask{}, errClaimContract
		}
		members, err := q.ListSquadMembers(r.Context(), squad.ID)
		if err != nil || len(members) == 0 {
			return claimedTask{}, errClaimContract
		}
		claimMembers := make([]claimSquadMember, 0, len(members))
		leaderCount := 0
		claimedAgentIsMember := false
		for _, member := range members {
			if member.AgentArchivedAt.Valid {
				return claimedTask{}, errClaimContract
			}
			if member.Role == "leader" {
				leaderCount++
				if member.AgentID != squad.LeaderID {
					return claimedTask{}, errClaimContract
				}
			}
			if member.AgentID == task.AgentID {
				claimedAgentIsMember = true
			}
			claimMembers = append(claimMembers, claimSquadMember{
				AgentID: uuidString(member.AgentID), Name: member.AgentName, Role: member.Role,
			})
		}
		if leaderCount != 1 || !claimedAgentIsMember || (task.IsLeaderTask && task.AgentID != squad.LeaderID) {
			return claimedTask{}, errClaimContract
		}
		squadContext = &claimSquad{
			ID: uuidString(squad.ID), Name: squad.Name, Instructions: squad.Instructions, Members: claimMembers,
		}
	}
	return claimedTask{
		ID: uuidString(task.ID), AgentID: uuidString(task.AgentID), RuntimeID: uuidString(task.RuntimeID),
		IssueID: optionalUUIDString(task.IssueID), ChatSessionID: optionalUUIDString(task.ChatSessionID), SquadID: optionalUUIDString(task.SquadID), Squad: squadContext,
		WorkspaceID: uuidString(task.WorkspaceID), WorkspaceContext: workspace.Context, Context: task.Context,
		Repos: workspace.Repos, IsLeaderTask: task.IsLeaderTask, ForceFreshSession: task.ForceFreshSession,
		HandoffNote: optionalString(task.HandoffNote), ThreadName: threadName,
		PriorSessionID: optionalString(task.SessionID), PriorWorkDir: optionalString(task.WorkDir), ChatMessage: chatMessage,
		CommentIDs: uuidStrings(task.DeliveredCommentIds),
		AuthToken:  plaintextToken,
		Agent: claimAgent{
			ID: uuidString(agent.ID), Name: agent.Name, Instructions: agent.Instructions, Skills: claimSkills,
			CustomEnv: customEnv, CustomArgs: agent.CustomArgs, MCPConfig: mcpConfig,
			Model: optionalString(agent.Model), ThinkingLevel: optionalString(agent.ThinkingLevel), ServiceTier: optionalString(agent.ServiceTier),
			PermissionMode: agent.PermissionMode, DisabledRuntimeSkills: agent.DisabledRuntimeSkills, RuntimeConfig: agent.RuntimeConfig,
		},
	}, nil
}

func uuidStrings(values []pgtype.UUID) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value.Valid {
			result = append(result, uuidString(value))
		}
	}
	return result
}

func optionalUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuidString(value)
}

func optionalString(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

type claimContractError struct{}

func (claimContractError) Error() string { return "claim contract violation" }

var errClaimContract error = claimContractError{}

type rpcResponseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (capture *rpcResponseCapture) Header() http.Header {
	if capture.header == nil {
		capture.header = make(http.Header)
	}
	return capture.header
}

func (capture *rpcResponseCapture) WriteHeader(status int) { capture.status = status }

func (capture *rpcResponseCapture) Write(body []byte) (int, error) {
	if capture.status == 0 {
		capture.status = http.StatusOK
	}
	return capture.body.Write(body)
}

func (h *Handler) DaemonRPCHandler(ctx context.Context, identity daemonws.ClientIdentity, method string, body json.RawMessage) (int, json.RawMessage, error) {
	if method != "tasks.claim" {
		return http.StatusNotFound, nil, fmt.Errorf("unsupported rpc method")
	}
	var request claimRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return http.StatusBadRequest, json.RawMessage(`{"code":"invalid_argument"}`), nil
	}
	allowedRuntimes := make(map[string]struct{}, len(identity.RuntimeIDs))
	for _, runtimeID := range identity.RuntimeIDs {
		allowedRuntimes[runtimeID] = struct{}{}
	}
	for _, runtimeID := range request.RuntimeIDs {
		if _, ok := allowedRuntimes[runtimeID]; !ok {
			return http.StatusNotFound, json.RawMessage(`{"code":"not_found"}`), nil
		}
	}
	requestBody, err := json.Marshal(request)
	if err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("rpc request encoding failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/daemon/tasks/claim", bytes.NewReader(requestBody))
	if err != nil {
		return http.StatusInternalServerError, nil, fmt.Errorf("rpc request construction failed")
	}
	principal := Principal{Kind: principalDaemon, WorkspaceID: identity.PrimaryWorkspaceID(), DaemonID: identity.DaemonID}
	req = req.WithContext(context.WithValue(req.Context(), principalKey, principal))
	capture := &rpcResponseCapture{}
	h.ClaimTasks(capture, req)
	status := capture.status
	if status == 0 {
		status = http.StatusOK
	}
	return status, json.RawMessage(capture.body.Bytes()), nil
}
