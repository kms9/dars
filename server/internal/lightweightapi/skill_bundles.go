package lightweightapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/service"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

func loadAgentSkillBundles(ctx context.Context, q *lwdb.Queries, agentID pgtype.UUID) ([]service.AgentSkillData, error) {
	rows, err := q.ListAgentSkills(ctx, agentID)
	if err != nil {
		return nil, err
	}
	skills := make([]service.AgentSkillData, 0, len(rows)+len(service.BuiltinSkills()))
	for _, skill := range rows {
		if !skill.Enabled {
			continue
		}
		files, err := q.ListSkillFiles(ctx, skill.ID)
		if err != nil {
			return nil, err
		}
		data := service.AgentSkillData{
			ID: uuidString(skill.ID), Source: "workspace", Name: skill.Name,
			Description: skill.Description, Content: skill.Content,
		}
		for _, file := range files {
			data.Files = append(data.Files, service.AgentSkillFileData{Path: file.Path, Content: file.Content})
		}
		skills = append(skills, data)
	}
	skills = append(skills, service.BuiltinSkills()...)
	bundles, _ := service.BuildAgentSkillBundles(skills)
	return bundles, nil
}

type resolveSkillBundlesRequest struct {
	Skills []resolveSkillBundleRef `json:"skills"`
}

type resolveSkillBundleRef struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Hash   string `json:"hash"`
}

// ResolveTaskSkillBundles returns only bundles assigned to the task's Agent.
// A stale requested hash intentionally resolves to the current bundle because
// the Lightweight baseline does not snapshot skill content at claim time.
func (h *Handler) ResolveTaskSkillBundles(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalDaemon {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	workspaceID, err := parseUUID(principal.WorkspaceID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	runtimeID, err := parseUUID(chi.URLParam(r, "runtimeId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	taskID, err := parseUUID(chi.URLParam(r, "taskId"))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil || runtime.DaemonID != principal.DaemonID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	task, err := h.q.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil || task.RuntimeID != runtimeID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if task.Status != "dispatched" && task.Status != "waiting_local_directory" {
		writeCode(w, http.StatusConflict, "task_not_preparing")
		return
	}

	var request resolveSkillBundlesRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	bundles, err := loadAgentSkillBundles(r.Context(), h.q, task.AgentID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	allowed := make(map[string]service.AgentSkillData, len(bundles))
	for _, bundle := range bundles {
		allowed[bundle.Source+"\x00"+bundle.ID] = bundle
	}
	resolved := make([]service.AgentSkillData, 0, len(request.Skills))
	for _, ref := range request.Skills {
		ref.ID = strings.TrimSpace(ref.ID)
		ref.Source = strings.TrimSpace(ref.Source)
		ref.Hash = strings.TrimSpace(ref.Hash)
		if ref.ID == "" || ref.Source == "" || ref.Hash == "" {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		bundle, ok := allowed[ref.Source+"\x00"+ref.ID]
		if !ok {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		resolved = append(resolved, bundle)
	}
	writeJSON(w, http.StatusOK, map[string]any{"bundles": resolved})
}
