package lightweightapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type workspaceResponse struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description *string         `json:"description"`
	Context     string          `json:"context"`
	Settings    json.RawMessage `json:"settings"`
	Repos       json.RawMessage `json:"repos"`
	IssuePrefix string          `json:"issue_prefix"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

func toWorkspaceResponse(workspace lwdb.Workspace) workspaceResponse {
	return workspaceResponse{
		ID: uuidString(workspace.ID), Name: workspace.Name, Slug: workspace.Slug,
		Description: textPointer(workspace.Description), Context: workspace.Context,
		Settings: workspace.Settings, Repos: workspace.Repos, IssuePrefix: workspace.IssuePrefix,
		CreatedAt: timeString(workspace.CreatedAt), UpdatedAt: timeString(workspace.UpdatedAt),
	}
}

type createWorkspaceRequest struct {
	Name string  `json:"name"`
	Slug *string `json:"slug"`
}

func (h *Handler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var request createWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len(request.Name) > 120 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	slug := ""
	if request.Slug != nil {
		slug = strings.ToLower(strings.TrimSpace(*request.Slug))
	}
	if slug == "" {
		slug = generatedWorkspaceSlug(request.Name)
	}
	if len(slug) > 63 || !slugPattern.MatchString(slug) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	workspace, err := qtx.CreateWorkspace(r.Context(), lwdb.CreateWorkspaceParams{
		Name: request.Name, Slug: slug, IssuePrefix: issuePrefix(slug),
	})
	if err != nil {
		writeCode(w, http.StatusConflict, "conflict")
		return
	}
	if _, err := qtx.CreateMember(r.Context(), lwdb.CreateMemberParams{
		WorkspaceID: workspace.ID, UserID: userID, Role: "owner",
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, toWorkspaceResponse(workspace))
}

func (h *Handler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
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
	rows, err := h.q.ListWorkspacesForUser(r.Context(), userID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]workspaceResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toWorkspaceResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := parseUUID(WorkspaceIDFromContext(r.Context()))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	workspace, err := h.q.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, toWorkspaceResponse(workspace))
}

type updateWorkspaceRequest struct {
	Name        *string         `json:"name"`
	Description json.RawMessage `json:"description"`
	Context     *string         `json:"context"`
	Settings    json.RawMessage `json:"settings"`
	Repos       json.RawMessage `json:"repos"`
}

func (h *Handler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := parseUUID(WorkspaceIDFromContext(r.Context()))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	var request updateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	params := lwdb.UpdateWorkspaceParams{ID: workspaceID}
	if request.Name != nil {
		value := strings.TrimSpace(*request.Name)
		if value == "" || len(value) > 120 {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.SetName, params.Name = true, value
	}
	if request.Description != nil {
		params.SetDescription = true
		if string(request.Description) != "null" {
			var description string
			if err := json.Unmarshal(request.Description, &description); err != nil {
				writeCode(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			params.Description = pgtype.Text{String: description, Valid: true}
		}
	}
	if request.Context != nil {
		params.SetContext, params.Context = true, *request.Context
	}
	if request.Settings != nil {
		if !json.Valid(request.Settings) {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.SetSettings, params.Settings = true, request.Settings
	}
	if request.Repos != nil {
		if !json.Valid(request.Repos) {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		params.SetRepos, params.Repos = true, request.Repos
	}
	workspace, err := h.q.UpdateWorkspace(r.Context(), params)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, toWorkspaceResponse(workspace))
}

type memberResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	CreatedAt   string `json:"created_at"`
}

func (h *Handler) ListWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := parseUUID(WorkspaceIDFromContext(r.Context()))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	rows, err := h.q.ListWorkspaceMembers(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]memberResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, memberResponse{
			ID: uuidString(row.ID), WorkspaceID: uuidString(row.WorkspaceID), UserID: uuidString(row.UserID),
			Role: row.Role, Name: row.UserName, Email: row.UserEmail, CreatedAt: timeString(row.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := parseUUID(WorkspaceIDFromContext(r.Context()))
	if err != nil {
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
	active, err := qtx.CountActiveTasksByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	online, err := qtx.CountOnlineRuntimesByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if active > 0 || online > 0 {
		writeCode(w, http.StatusConflict, "active_tasks_exist")
		return
	}
	agentAvatars, err := qtx.ListAgentAvatarURLsByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	squadAvatars, err := qtx.ListSquadAvatarURLsByWorkspace(r.Context(), workspaceID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := deleteWorkspaceDependents(r, qtx, workspaceID); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	deleted, err := qtx.DeleteWorkspace(r.Context(), workspaceID)
	if err != nil || deleted != 1 {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	avatarURLs := append(validTextStrings(agentAvatars), validTextStrings(squadAvatars)...)
	if err := h.cleanupAvatarObjects(r.Context(), avatarURLs); err != nil {
		// Database delete already committed; surface a diagnostic without
		// pretending the workspace row still exists.
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_deleted":    true,
			"avatar_cleanup_error": err.Error(),
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func generatedWorkspaceSlug(name string) string {
	base := strings.ToLower(name)
	var out strings.Builder
	lastDash := false
	for _, ch := range base {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			out.WriteRune(ch)
			lastDash = false
		} else if out.Len() > 0 && !lastDash {
			out.WriteByte('-')
			lastDash = true
		}
	}
	prefix := strings.Trim(out.String(), "-")
	if len(prefix) > 48 {
		prefix = strings.Trim(prefix[:48], "-")
	}
	if prefix == "" {
		prefix = "workspace"
	}
	var suffix [4]byte
	_, _ = rand.Read(suffix[:])
	return prefix + "-" + hex.EncodeToString(suffix[:])
}

func issuePrefix(slug string) string {
	var prefix strings.Builder
	for _, ch := range strings.ToUpper(slug) {
		if ch >= 'A' && ch <= 'Z' {
			prefix.WriteRune(ch)
			if prefix.Len() == 3 {
				break
			}
		}
	}
	if prefix.Len() == 0 {
		return "RUN"
	}
	return prefix.String()
}

func deleteWorkspaceDependents(r *http.Request, q *lwdb.Queries, workspaceID pgtype.UUID) error {
	steps := []func() error{
		func() error { _, err := q.DeleteDaemonTokensByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteTaskTokensByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteActivityByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteTaskUsageByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteTaskMessagesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteChatDraftRestoresByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteAgentBuilderDraftsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteChatMessagesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteTasksByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteAgentToolBundleHeadsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.RevokeToolBundlesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteToolBundleItemsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteToolBundlesByWorkspace(r.Context(), workspaceID); return err },
		func() error {
			_, err := q.ClearToolSourceCurrentRevisionsByWorkspace(r.Context(), workspaceID)
			return err
		},
		func() error { _, err := q.DeleteToolDefinitionsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteToolSourceRevisionsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteToolSourceArtifactsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteToolSourceSecretsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteToolSourcesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteChatSessionsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteCommentsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteIssuesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteSquadMembersByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteSquadsByWorkspace(r.Context(), workspaceID); return err },
		func() error {
			_, err := q.DeleteAgentInvocationTargetsByWorkspace(r.Context(), workspaceID)
			return err
		},
		func() error { _, err := q.DeleteAgentSkillsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteSkillFilesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteSkillsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteAgentsByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteAgentRuntimesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteRuntimeProfilesByWorkspace(r.Context(), workspaceID); return err },
		func() error { _, err := q.DeleteMembersByWorkspace(r.Context(), workspaceID); return err },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}
