package lightweightapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/redact"
)

const squadEvaluationReasonMaxRunes = 500

type squadEvaluationRequest struct {
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

type squadEvaluationDetails struct {
	SquadID string `json:"squad_id"`
	TaskID  string `json:"task_id"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

type squadEvaluationResponse struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	CreatedAt string `json:"created_at"`
}

func validSquadEvaluationOutcome(outcome string) bool {
	switch outcome {
	case "action", "no_action", "failed":
		return true
	default:
		return false
	}
}

func activeTaskCredentialStatus(status string) bool {
	switch status {
	case "dispatched", "waiting_local_directory", "running":
		return true
	default:
		return false
	}
}

func squadEvaluationDTO(activity lwdb.ActivityLog) squadEvaluationResponse {
	return squadEvaluationResponse{
		ID: uuidString(activity.ID), Action: activity.Action, CreatedAt: timeString(activity.CreatedAt),
	}
}

// RecordSquadLeaderEvaluation 记录当前小队 Leader Task 的一次不可变评估结论。
// 仅接受 Task Token（dat_），且任务必须 is_leader_task、归属本 Issue 的 assignee squad。
// outcome ∈ action|no_action|failed，写入 activity_log；归因全部来自服务端锁定状态，禁止客户端伪造。
// RecordSquadLeaderEvaluation records one immutable decision for the current
// Squad Leader Task. Actor, Task and Squad attribution are derived exclusively
// from the authenticated Task Token and locked server state.
func (h *Handler) RecordSquadLeaderEvaluation(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, principal, ok := h.issueScope(w, r)
	if !ok {
		return
	}
	if principal.Kind != principalTask {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}

	request := squadEvaluationRequest{}
	if decodeStrictTaskBody(r, &request) != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	request.Outcome = strings.TrimSpace(request.Outcome)
	if !validSquadEvaluationOutcome(request.Outcome) || !utf8.ValidString(request.Reason) ||
		strings.ContainsRune(request.Reason, '\x00') {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	request.Reason = strings.TrimSpace(redact.Text(request.Reason))
	if utf8.RuneCountInString(request.Reason) > squadEvaluationReasonMaxRunes {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if issue.AssigneeType != "squad" || !issue.AssigneeID.Valid {
		writeCode(w, http.StatusBadRequest, "issue_not_squad")
		return
	}

	taskID, err := parseUUID(principal.TaskID)
	if err != nil {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	agentID, err := parseUUID(principal.AgentID)
	if err != nil {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}

	tx, ok := beginTx(r, h.pool)
	if !ok {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)

	lockedIssue, err := qtx.LockIssue(r.Context(), lwdb.LockIssueParams{ID: issue.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if lockedIssue.AssigneeType != "squad" || !lockedIssue.AssigneeID.Valid {
		writeCode(w, http.StatusBadRequest, "issue_not_squad")
		return
	}
	lockedTask, err := qtx.LockAgentTask(r.Context(), lwdb.LockAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil || !lockedTask.IssueID.Valid || lockedTask.IssueID != lockedIssue.ID || lockedTask.AgentID != agentID {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if !activeTaskCredentialStatus(lockedTask.Status) {
		writeCode(w, http.StatusConflict, "invalid_task_state")
		return
	}
	if !lockedTask.IsLeaderTask || !lockedTask.SquadID.Valid || lockedTask.SquadID != lockedIssue.AssigneeID {
		writeCode(w, http.StatusForbidden, "squad_leader_required")
		return
	}
	lockedSquad, err := qtx.LockSquad(r.Context(), lwdb.LockSquadParams{ID: lockedIssue.AssigneeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusBadRequest, "issue_not_squad")
		return
	}
	if lockedSquad.LeaderID != lockedTask.AgentID {
		writeCode(w, http.StatusForbidden, "squad_leader_required")
		return
	}

	details := squadEvaluationDetails{
		SquadID: uuidString(lockedSquad.ID), TaskID: uuidString(lockedTask.ID),
		Outcome: request.Outcome, Reason: request.Reason,
	}
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}

	existing, err := qtx.GetSquadEvaluationByTask(r.Context(), lwdb.GetSquadEvaluationByTaskParams{
		WorkspaceID: workspaceID, IssueID: lockedIssue.ID, TaskID: lockedTask.ID,
	})
	if err == nil {
		var existingDetails squadEvaluationDetails
		if json.Unmarshal(existing.Details, &existingDetails) != nil ||
			existingDetails.Outcome != details.Outcome || existingDetails.Reason != details.Reason ||
			existingDetails.SquadID != details.SquadID || existingDetails.TaskID != details.TaskID {
			writeCode(w, http.StatusConflict, "evaluation_already_recorded")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		writeJSON(w, http.StatusCreated, squadEvaluationDTO(existing))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}

	activity, err := qtx.CreateActivity(r.Context(), lwdb.CreateActivityParams{
		WorkspaceID: workspaceID, IssueID: lockedIssue.ID, ActorType: "agent",
		ActorID: lockedTask.AgentID, Action: "squad_leader_evaluated", Details: detailsJSON,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}

	writeJSON(w, http.StatusCreated, squadEvaluationDTO(activity))
}
