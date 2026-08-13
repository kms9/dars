package lightweightapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

type squadMemberPreviewResponse struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

type squadResponse struct {
	ID            string                       `json:"id"`
	WorkspaceID   string                       `json:"workspace_id"`
	Name          string                       `json:"name"`
	Description   string                       `json:"description"`
	Instructions  string                       `json:"instructions"`
	AvatarURL     *string                      `json:"avatar_url"`
	LeaderID      string                       `json:"leader_id"`
	CreatorID     string                       `json:"creator_id"`
	ArchivedAt    *string                      `json:"archived_at"`
	ArchivedBy    *string                      `json:"archived_by"`
	MemberCount   int                          `json:"member_count"`
	MemberPreview []squadMemberPreviewResponse `json:"member_preview"`
	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
}

type squadMemberResponse struct {
	ID         string  `json:"id"`
	SquadID    string  `json:"squad_id"`
	AgentID    string  `json:"agent_id"`
	Name       string  `json:"name"`
	Role       string  `json:"role"`
	Status     string  `json:"status"`
	ArchivedAt *string `json:"archived_at"`
	CreatedAt  string  `json:"created_at"`
}

type squadMemberStatusResponse struct {
	AgentID          string  `json:"agent_id"`
	Status           string  `json:"status"`
	LastActiveAt     *string `json:"last_active_at"`
	ActiveIssueID    *string `json:"active_issue_id"`
	ActiveIssueTitle *string `json:"active_issue_title"`
}

func squadMemberDTO(row lwdb.ListSquadMembersRow) squadMemberResponse {
	return squadMemberResponse{
		ID: uuidString(row.ID), SquadID: uuidString(row.SquadID), AgentID: uuidString(row.AgentID),
		Name: row.AgentName, Role: row.Role, Status: row.AgentStatus,
		ArchivedAt: timePointer(row.AgentArchivedAt), CreatedAt: timeString(row.CreatedAt),
	}
}

func (h *Handler) squadDTO(ctx *http.Request, squad lwdb.Squad) (squadResponse, error) {
	members, err := h.q.ListSquadMembers(ctx.Context(), squad.ID)
	if err != nil {
		return squadResponse{}, err
	}
	preview := make([]squadMemberPreviewResponse, 0, min(len(members), 3))
	for index, member := range members {
		if index >= 3 {
			break
		}
		preview = append(preview, squadMemberPreviewResponse{AgentID: uuidString(member.AgentID), Name: member.AgentName, Role: member.Role})
	}
	return squadResponse{
		ID: uuidString(squad.ID), WorkspaceID: uuidString(squad.WorkspaceID), Name: squad.Name,
		Description: squad.Description, Instructions: squad.Instructions, AvatarURL: textPointer(squad.AvatarUrl),
		LeaderID: uuidString(squad.LeaderID), CreatorID: uuidString(squad.CreatorID),
		ArchivedAt: timePointer(squad.ArchivedAt), ArchivedBy: optionalUUIDPointer(squad.ArchivedBy),
		MemberCount: len(members), MemberPreview: preview,
		CreatedAt: timeString(squad.CreatedAt), UpdatedAt: timeString(squad.UpdatedAt),
	}, nil
}

func optionalUUIDPointer(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	text := uuidString(value)
	return &text
}

func (h *Handler) loadSquad(w http.ResponseWriter, r *http.Request) (lwdb.Squad, bool) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return lwdb.Squad{}, false
	}
	squadID, err := parseUUID(chi.URLParam(r, "squadId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Squad{}, false
	}
	squad, err := h.q.GetSquad(r.Context(), lwdb.GetSquadParams{ID: squadID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.Squad{}, false
	}
	return squad, true
}

func (h *Handler) canManageSquad(r *http.Request, squad lwdb.Squad) bool {
	member, ok := MemberFromContext(r.Context())
	if ok && (member.Role == "owner" || member.Role == "admin") {
		return true
	}
	principal, ok := principalFromContext(r.Context())
	return ok && principal.UserID == uuidString(squad.CreatorID)
}

func (h *Handler) callableSquadAgent(r *http.Request, workspaceID, userID, agentID pgtype.UUID) (lwdb.Agent, bool) {
	agent, err := h.q.GetAgent(r.Context(), lwdb.GetAgentParams{ID: agentID, WorkspaceID: workspaceID})
	if err != nil || agent.ArchivedAt.Valid || !h.humanCanInvokeAgent(r.Context(), agent, userID) {
		return lwdb.Agent{}, false
	}
	return agent, true
}

func (h *Handler) ListSquads(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	rows, err := h.q.ListSquads(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]squadResponse, 0, len(rows))
	for _, row := range rows {
		dto, err := h.squadDTO(r, row)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		response = append(response, dto)
	}
	writeJSON(w, http.StatusOK, response)
}

type squadMutation struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	Instructions *string `json:"instructions"`
	LeaderID     *string `json:"leader_id"`
}

type squadCreateBody struct {
	Name               *string  `json:"name"`
	Description        *string  `json:"description"`
	Instructions       *string  `json:"instructions"`
	LeaderID           *string  `json:"leader_id"`
	AdditionalAgentIDs []string `json:"additional_agent_ids"`
}

func (h *Handler) CreateSquad(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var body squadCreateBody
	if decodeStrictTaskBody(r, &body) != nil || body.Name == nil || body.LeaderID == nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	name := strings.TrimSpace(*body.Name)
	leaderID, err := parseUUID(*body.LeaderID)
	if err != nil || name == "" || len(name) > 200 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if _, allowed := h.callableSquadAgent(r, workspaceID, userID, leaderID); !allowed {
		writeCode(w, http.StatusForbidden, "invocation_forbidden")
		return
	}
	additionalIDs := make([]pgtype.UUID, 0, len(body.AdditionalAgentIDs))
	seen := map[string]struct{}{uuidString(leaderID): {}}
	for _, raw := range body.AdditionalAgentIDs {
		agentID, err := parseUUID(raw)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		key := uuidString(agentID)
		if _, dup := seen[key]; dup {
			writeCode(w, http.StatusConflict, "duplicate_member")
			return
		}
		seen[key] = struct{}{}
		if _, allowed := h.callableSquadAgent(r, workspaceID, userID, agentID); !allowed {
			writeCode(w, http.StatusForbidden, "invocation_forbidden")
			return
		}
		additionalIDs = append(additionalIDs, agentID)
	}
	description, instructions := "", ""
	if body.Description != nil {
		description = *body.Description
	}
	if body.Instructions != nil {
		instructions = *body.Instructions
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	squad, err := qtx.CreateSquad(r.Context(), lwdb.CreateSquadParams{
		WorkspaceID: workspaceID, Name: name, Description: description, Instructions: instructions,
		LeaderID: leaderID, CreatorID: userID,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.UpsertSquadMember(r.Context(), lwdb.UpsertSquadMemberParams{SquadID: squad.ID, AgentID: leaderID, Role: "leader"}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	for _, agentID := range additionalIDs {
		if _, err := qtx.UpsertSquadMember(r.Context(), lwdb.UpsertSquadMemberParams{SquadID: squad.ID, AgentID: agentID, Role: "member"}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response, err := h.squadDTO(r, squad)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventSquadCreated, uuidString(workspaceID), "member", principal.UserID, map[string]any{"squad": response})
	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) GetSquad(w http.ResponseWriter, r *http.Request) {
	squad, ok := h.loadSquad(w, r)
	if !ok {
		return
	}
	response, err := h.squadDTO(r, squad)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) UpdateSquad(w http.ResponseWriter, r *http.Request) {
	squad, ok := h.loadSquad(w, r)
	if !ok {
		return
	}
	if !h.canManageSquad(r, squad) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if squad.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "squad_archived")
		return
	}
	principal, _ := principalFromContext(r.Context())
	userID, _ := parseUUID(principal.UserID)
	var body squadMutation
	if decodeStrictTaskBody(r, &body) != nil || (body.Name == nil && body.Description == nil && body.Instructions == nil && body.LeaderID == nil) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	params := lwdb.UpdateSquadParams{ID: squad.ID, WorkspaceID: squad.WorkspaceID}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 200 {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.SetName, params.Name = true, name
	}
	if body.Description != nil {
		params.SetDescription, params.Description = true, *body.Description
	}
	if body.Instructions != nil {
		params.SetInstructions, params.Instructions = true, *body.Instructions
	}
	if body.LeaderID != nil {
		leaderID, err := parseUUID(*body.LeaderID)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		if _, allowed := h.callableSquadAgent(r, squad.WorkspaceID, userID, leaderID); !allowed {
			writeCode(w, http.StatusForbidden, "invocation_forbidden")
			return
		}
		params.SetLeaderID, params.LeaderID = true, leaderID
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	locked, err := qtx.LockSquad(r.Context(), lwdb.LockSquadParams{ID: squad.ID, WorkspaceID: squad.WorkspaceID})
	if err != nil || locked.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	if params.SetLeaderID {
		if _, err := qtx.UpsertSquadMember(r.Context(), lwdb.UpsertSquadMemberParams{SquadID: squad.ID, AgentID: params.LeaderID, Role: "leader"}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if _, err := qtx.DemoteSquadLeadersExcept(r.Context(), lwdb.DemoteSquadLeadersExceptParams{SquadID: squad.ID, LeaderID: params.LeaderID}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	updated, err := qtx.UpdateSquad(r.Context(), params)
	if err != nil || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response, err := h.squadDTO(r, updated)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventSquadUpdated, uuidString(squad.WorkspaceID), "member", principal.UserID, map[string]any{"squad": response})
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) DeleteSquad(w http.ResponseWriter, r *http.Request) {
	squad, ok := h.loadSquad(w, r)
	if !ok {
		return
	}
	if !h.canManageSquad(r, squad) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
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
	locked, err := qtx.LockSquad(r.Context(), lwdb.LockSquadParams{ID: squad.ID, WorkspaceID: squad.WorkspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if locked.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "squad_archived")
		return
	}
	leaderAgent, leaderCallable := h.callableSquadAgent(r, squad.WorkspaceID, actorID, locked.LeaderID)
	if !leaderCallable {
		writeCode(w, http.StatusConflict, "leader_required")
		return
	}
	if _, err := qtx.ReassignIssueAssigneesFromSquad(r.Context(), lwdb.ReassignIssueAssigneesFromSquadParams{
		WorkspaceID: squad.WorkspaceID, SquadID: squad.ID, LeaderID: leaderAgent.ID,
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if _, err := qtx.ArchiveSquad(r.Context(), lwdb.ArchiveSquadParams{ActorID: actorID, ID: squad.ID, WorkspaceID: squad.WorkspaceID}); err != nil || tx.Commit(r.Context()) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	h.publish(protocol.EventSquadDeleted, uuidString(squad.WorkspaceID), "member", principal.UserID, map[string]any{"squad_id": uuidString(squad.ID)})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListSquadMembers(w http.ResponseWriter, r *http.Request) {
	squad, ok := h.loadSquad(w, r)
	if !ok {
		return
	}
	rows, err := h.q.ListSquadMembers(r.Context(), squad.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]squadMemberResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, squadMemberDTO(row))
	}
	writeJSON(w, http.StatusOK, response)
}

type squadMemberMutation struct {
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
}

func (h *Handler) AddSquadMember(w http.ResponseWriter, r *http.Request) {
	h.mutateSquadMember(w, r, "add")
}

func (h *Handler) RemoveSquadMember(w http.ResponseWriter, r *http.Request) {
	h.mutateSquadMember(w, r, "remove")
}

func (h *Handler) UpdateSquadMemberRole(w http.ResponseWriter, r *http.Request) {
	h.mutateSquadMember(w, r, "role")
}

func (h *Handler) mutateSquadMember(w http.ResponseWriter, r *http.Request, operation string) {
	squad, ok := h.loadSquad(w, r)
	if !ok {
		return
	}
	if !h.canManageSquad(r, squad) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if squad.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "squad_archived")
		return
	}
	var body squadMemberMutation
	if decodeStrictTaskBody(r, &body) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	agentID, err := parseUUID(body.AgentID)
	if err != nil || (operation != "remove" && body.Role != "member" && body.Role != "leader") {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	principal, _ := principalFromContext(r.Context())
	userID, _ := parseUUID(principal.UserID)
	if operation != "remove" {
		if _, allowed := h.callableSquadAgent(r, squad.WorkspaceID, userID, agentID); !allowed {
			writeCode(w, http.StatusForbidden, "invocation_forbidden")
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
	locked, err := qtx.LockSquad(r.Context(), lwdb.LockSquadParams{ID: squad.ID, WorkspaceID: squad.WorkspaceID})
	if err != nil || locked.ArchivedAt.Valid {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	if operation == "add" {
		existing, err := qtx.ListSquadMembers(r.Context(), squad.ID)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		for _, row := range existing {
			if row.AgentID == agentID {
				writeCode(w, http.StatusConflict, "duplicate_member")
				return
			}
		}
	}
	if operation == "remove" {
		if locked.LeaderID == agentID {
			writeCode(w, http.StatusConflict, "leader_required")
			return
		}
		deleted, err := qtx.DeleteSquadMember(r.Context(), lwdb.DeleteSquadMemberParams{SquadID: squad.ID, AgentID: agentID})
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if deleted != 1 {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		h.publish(protocol.EventSquadUpdated, uuidString(squad.WorkspaceID), "member", principal.UserID, map[string]any{"squad_id": uuidString(squad.ID)})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if body.Role == "member" && locked.LeaderID == agentID {
		writeCode(w, http.StatusConflict, "leader_required")
		return
	}
	member, err := qtx.UpsertSquadMember(r.Context(), lwdb.UpsertSquadMemberParams{SquadID: squad.ID, AgentID: agentID, Role: body.Role})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if body.Role == "leader" {
		if _, err := qtx.DemoteSquadLeadersExcept(r.Context(), lwdb.DemoteSquadLeadersExceptParams{SquadID: squad.ID, LeaderID: agentID}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if _, err := qtx.UpdateSquad(r.Context(), lwdb.UpdateSquadParams{SetLeaderID: true, LeaderID: agentID, ID: squad.ID, WorkspaceID: squad.WorkspaceID}); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	rows, err := h.q.ListSquadMembers(r.Context(), squad.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	for _, row := range rows {
		if row.ID == member.ID {
			h.publish(protocol.EventSquadUpdated, uuidString(squad.WorkspaceID), "member", principal.UserID, map[string]any{"squad_id": uuidString(squad.ID)})
			status := http.StatusOK
			if operation == "add" {
				status = http.StatusCreated
			}
			writeJSON(w, status, squadMemberDTO(row))
			return
		}
	}
	writeCode(w, http.StatusInternalServerError, "internal_error")
}

func (h *Handler) ListSquadMemberStatus(w http.ResponseWriter, r *http.Request) {
	squad, ok := h.loadSquad(w, r)
	if !ok {
		return
	}
	rows, err := h.q.ListSquadMemberStatuses(r.Context(), squad.ID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	now := h.now()
	response := make([]squadMemberStatusResponse, 0, len(rows))
	for _, row := range rows {
		status := "offline"
		switch {
		case row.AgentArchivedAt.Valid:
			status = "archived"
		case row.HasActiveTask:
			status = "working"
		case row.RuntimeStatus.Valid && row.RuntimeStatus.String == "online":
			status = "idle"
		case row.RuntimeLastSeenAt.Valid && now.Sub(row.RuntimeLastSeenAt.Time) < 5*time.Minute:
			status = "unstable"
		}
		lastActiveAt := timePointer(row.LastActiveAt)
		if lastActiveAt == nil {
			lastActiveAt = timePointer(row.RuntimeLastSeenAt)
		}
		var activeIssueTitle *string
		if row.ActiveIssueTitle != "" {
			activeIssueTitle = &row.ActiveIssueTitle
		}
		response = append(response, squadMemberStatusResponse{
			AgentID:          uuidString(row.AgentID),
			Status:           status,
			LastActiveAt:     lastActiveAt,
			ActiveIssueID:    optionalUUIDPointer(row.ActiveIssueID),
			ActiveIssueTitle: activeIssueTitle,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": response})
}
