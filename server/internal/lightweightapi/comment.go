package lightweightapi

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	commentContentMaxBytes     = 1 << 20
	commentIdempotencyMaxBytes = 255
)

type commentResponse struct {
	ID            string  `json:"id"`
	WorkspaceID   string  `json:"workspace_id"`
	IssueID       string  `json:"issue_id"`
	AuthorType    string  `json:"author_type"`
	AuthorID      *string `json:"author_id"`
	AuthorDisplay string  `json:"author_display"`
	Content       string  `json:"content"`
	Type          string  `json:"type"`
	SourceTaskID  *string `json:"source_task_id"`
	CreatedAt     string  `json:"created_at"`
}

func commentAuthorID(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	result := uuidString(value)
	return &result
}

func commentDTO(comment lwdb.Comment, authorDisplay string) commentResponse {
	return commentResponse{
		ID: uuidString(comment.ID), WorkspaceID: uuidString(comment.WorkspaceID), IssueID: uuidString(comment.IssueID),
		AuthorType: comment.AuthorType, AuthorID: commentAuthorID(comment.AuthorID), AuthorDisplay: authorDisplay,
		Content: comment.Content, Type: comment.Type, SourceTaskID: optionalUUIDPointer(comment.SourceTaskID),
		CreatedAt: timeString(comment.CreatedAt),
	}
}

func commentPageDTO(comment lwdb.ListCommentsPageRow) commentResponse {
	return commentResponse{
		ID: uuidString(comment.ID), WorkspaceID: uuidString(comment.WorkspaceID), IssueID: uuidString(comment.IssueID),
		AuthorType: comment.AuthorType, AuthorID: commentAuthorID(comment.AuthorID), AuthorDisplay: comment.AuthorDisplay,
		Content: comment.Content, Type: comment.Type, SourceTaskID: optionalUUIDPointer(comment.SourceTaskID),
		CreatedAt: timeString(comment.CreatedAt),
	}
}

func (h *Handler) commentAuthorDisplay(r *http.Request, authorType string, authorID pgtype.UUID) (string, error) {
	switch authorType {
	case "member":
		user, err := h.q.GetUserByID(r.Context(), authorID)
		if err != nil {
			return "", err
		}
		return user.Name, nil
	case "agent":
		agent, err := h.q.GetAgent(r.Context(), lwdb.GetAgentParams{ID: authorID, WorkspaceID: mustWorkspaceUUID(r)})
		if err != nil {
			return "", err
		}
		return agent.Name, nil
	default:
		return "System", nil
	}
}

func mustWorkspaceUUID(r *http.Request) pgtype.UUID {
	workspaceID, _ := contextWorkspaceUUID(r)
	return workspaceID
}

func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, _, ok := h.issueScope(w, r)
	if !ok {
		return
	}
	for key := range r.URL.Query() {
		if key != "limit" && key != "cursor" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	limit, cursorAt, cursorID, err := parseChatPage(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_cursor")
		return
	}
	rows, err := h.q.ListCommentsPage(r.Context(), lwdb.ListCommentsPageParams{
		WorkspaceID: workspaceID, IssueID: issue.ID, CursorCreatedAt: cursorAt, CursorID: cursorID, PageLimit: limit + 1,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]commentResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, commentPageDTO(row))
	}
	var nextCursor *string
	if hasMore && len(rows) > 0 {
		value := encodeChatCursor(rows[len(rows)-1].CreatedAt, rows[len(rows)-1].ID)
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}

type createCommentRequest struct {
	Content string `json:"content"`
}

// CreateComment 在 Issue 下追加评论（人类或持 dat_ 的 Agent）。
// 写入 Comment 后调用 routeCreatedComment：Leader 用结构化 mention 派活给成员，
// 或人类跟帖唤醒负责人；成功后 announceQueuedTask 并 publish comment:created。
func (h *Handler) CreateComment(w http.ResponseWriter, r *http.Request) {
	issue, workspaceID, principal, ok := h.issueScope(w, r)
	if !ok {
		return
	}
	var request createCommentRequest
	if decodeStrictTaskBody(r, &request) != nil || strings.TrimSpace(request.Content) == "" ||
		len(request.Content) > commentContentMaxBytes || !utf8.ValidString(request.Content) || strings.ContainsRune(request.Content, '\x00') {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > commentIdempotencyMaxBytes {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}

	authorType := "member"
	authorID, err := parseUUID(principal.UserID)
	var sourceTaskID pgtype.UUID
	var sourceTask *lwdb.AgentTaskQueue
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	if principal.Kind == principalTask {
		authorType = "agent"
		authorID, err = parseUUID(principal.AgentID)
		if err != nil {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		sourceTaskID, err = parseUUID(principal.TaskID)
		if err != nil {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
	}
	requestHash := requestDigest(request)
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	locked, err := qtx.LockIssue(r.Context(), lwdb.LockIssueParams{ID: issue.ID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if principal.Kind == principalTask {
		task, taskErr := qtx.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: sourceTaskID, WorkspaceID: workspaceID})
		if taskErr != nil || !task.IssueID.Valid || task.IssueID != locked.ID || task.AgentID != authorID {
			writeCode(w, http.StatusForbidden, "forbidden")
			return
		}
		sourceTask = &task
	}
	key := pgtype.Text{String: idempotencyKey, Valid: true}
	existing, err := qtx.GetCommentByIdempotencyKey(r.Context(), lwdb.GetCommentByIdempotencyKeyParams{
		WorkspaceID: workspaceID, IssueID: locked.ID, AuthorType: authorType, AuthorID: authorID, IdempotencyKey: key,
	})
	if err == nil {
		if !existing.RequestHash.Valid || existing.RequestHash.String != requestHash {
			writeCode(w, http.StatusConflict, "idempotency_key_reused")
			return
		}
		display, displayErr := h.commentAuthorDisplay(r, authorType, authorID)
		if displayErr != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		writeJSON(w, http.StatusCreated, commentDTO(existing, display))
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	created, err := qtx.CreateComment(r.Context(), lwdb.CreateCommentParams{
		WorkspaceID: workspaceID, IssueID: locked.ID, AuthorType: authorType, AuthorID: authorID,
		Content: request.Content, SourceTaskID: sourceTaskID, IdempotencyKey: key,
		RequestHash: pgtype.Text{String: requestHash, Valid: true},
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	queuedTasks, err := h.routeCreatedComment(r.Context(), qtx, locked, created, principal, sourceTask)
	if err != nil {
		writeIssueResolutionError(w, err)
		return
	}
	if _, err := qtx.TouchIssueForComment(r.Context(), lwdb.TouchIssueForCommentParams{ID: locked.ID, WorkspaceID: workspaceID}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	display, err := h.commentAuthorDisplay(r, authorType, authorID)
	if err != nil {
		display = "System"
	}
	for _, task := range queuedTasks {
		h.announceQueuedTask(task)
	}
	h.publish(protocol.EventCommentCreated, uuidString(workspaceID), authorType, uuidString(authorID), commentDTO(created, display))
	writeJSON(w, http.StatusCreated, commentDTO(created, display))
}
