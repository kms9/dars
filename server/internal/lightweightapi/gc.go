package lightweightapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

const (
	maxIssueGCBatchSize      = 500
	maxIssueGCBatchBodyBytes = 64 << 10
)

type issueGCRequest struct {
	IssueIDs []string `json:"issue_ids"`
}

type issueGCItem struct {
	ID        string  `json:"id"`
	Found     bool    `json:"found"`
	Status    string  `json:"status,omitempty"`
	UpdatedAt *string `json:"updated_at,omitempty"`
}

func daemonWorkspaceScope(w http.ResponseWriter, r *http.Request) (Principal, pgtype.UUID, bool) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return Principal{}, pgtype.UUID{}, false
	}
	workspaceID, err := parseUUID(principal.WorkspaceID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return Principal{}, pgtype.UUID{}, false
	}
	return principal, workspaceID, true
}

func (h *Handler) BatchIssueGCCheck(w http.ResponseWriter, r *http.Request) {
	principal, workspaceID, ok := daemonWorkspaceScope(w, r)
	if !ok {
		return
	}
	pathWorkspaceID, err := parseUUID(chi.URLParam(r, "workspaceId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if uuidString(pathWorkspaceID) != principal.WorkspaceID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxIssueGCBatchBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request issueGCRequest
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if len(request.IssueIDs) > maxIssueGCBatchSize {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}

	issueIDs := make([]pgtype.UUID, 0, len(request.IssueIDs))
	canonicalIDs := make([]string, 0, len(request.IssueIDs))
	for _, rawID := range request.IssueIDs {
		issueID, err := parseUUID(rawID)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		issueIDs = append(issueIDs, issueID)
		canonicalIDs = append(canonicalIDs, uuidString(issueID))
	}

	byID := make(map[string]lwdb.ListIssueGCStatusesRow, len(issueIDs))
	if len(issueIDs) > 0 {
		rows, err := h.q.ListIssueGCStatuses(r.Context(), lwdb.ListIssueGCStatusesParams{
			WorkspaceID: workspaceID,
			IssueIds:    issueIDs,
		})
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		for _, row := range rows {
			byID[uuidString(row.ID)] = row
		}
	}

	items := make([]issueGCItem, 0, len(request.IssueIDs))
	for index, originalID := range request.IssueIDs {
		row, found := byID[canonicalIDs[index]]
		item := issueGCItem{ID: originalID, Found: found}
		if found {
			item.Status = row.Status
			value := timeString(row.UpdatedAt)
			item.UpdatedAt = &value
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": items})
}

func (h *Handler) GetIssueGCCheck(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := daemonWorkspaceScope(w, r)
	if !ok {
		return
	}
	issueID, err := parseUUID(chi.URLParam(r, "issueId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	issue, err := h.q.GetIssue(r.Context(), lwdb.GetIssueParams{ID: issueID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": issue.Status, "updated_at": timeString(issue.UpdatedAt)})
}

func (h *Handler) GetChatSessionGCCheck(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, ok := daemonWorkspaceScope(w, r)
	if !ok {
		return
	}
	sessionID, err := parseUUID(chi.URLParam(r, "sessionId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	session, err := h.q.GetChatSession(r.Context(), lwdb.GetChatSessionParams{ID: sessionID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": session.Status, "updated_at": timeString(session.UpdatedAt)})
}

func (h *Handler) GetTaskGCCheck(w http.ResponseWriter, r *http.Request) {
	principal, workspaceID, ok := daemonWorkspaceScope(w, r)
	if !ok {
		return
	}
	taskID, err := parseUUID(chi.URLParam(r, "taskId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	task, err := h.q.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: task.RuntimeID, WorkspaceID: workspaceID})
	if err != nil || !strings.EqualFold(runtime.DaemonID, principal.DaemonID) {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": task.Status, "completed_at": timePointer(task.CompletedAt)})
}
