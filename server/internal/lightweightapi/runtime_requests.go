package lightweightapi

import (
	"encoding/json"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	runtimeRequestPendingTimeout = 30 * time.Second
	runtimeImportPendingTimeout  = 3 * time.Minute
	runtimeRequestRunningTimeout = 60 * time.Second
	runtimeRequestRetention      = 5 * time.Minute
	maxRuntimeImportBatch        = 10
)

type runtimeRequestStatus string

const (
	runtimeRequestPending   runtimeRequestStatus = "pending"
	runtimeRequestRunning   runtimeRequestStatus = "running"
	runtimeRequestCompleted runtimeRequestStatus = "completed"
	runtimeRequestConflict  runtimeRequestStatus = "conflict"
	runtimeRequestFailed    runtimeRequestStatus = "failed"
	runtimeRequestTimeout   runtimeRequestStatus = "timeout"
)

type runtimeRequestMeta struct {
	ID           string               `json:"id"`
	RuntimeID    string               `json:"runtime_id"`
	Status       runtimeRequestStatus `json:"status"`
	Error        string               `json:"error,omitempty"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
	RunStartedAt *time.Time           `json:"-"`
}

type runtimeModelThinkingLevel struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type runtimeModelThinking struct {
	SupportedLevels []runtimeModelThinkingLevel `json:"supported_levels"`
	DefaultLevel    string                      `json:"default_level,omitempty"`
}

type runtimeModelServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type runtimeModelEntry struct {
	ID           string                    `json:"id"`
	Label        string                    `json:"label"`
	Provider     string                    `json:"provider,omitempty"`
	Default      bool                      `json:"default,omitempty"`
	Thinking     *runtimeModelThinking     `json:"thinking,omitempty"`
	ServiceTiers []runtimeModelServiceTier `json:"service_tiers,omitempty"`
}

type runtimeModelListRequest struct {
	runtimeRequestMeta
	Models    []runtimeModelEntry `json:"models,omitempty"`
	Supported bool                `json:"supported"`
}

type runtimeLocalSkillSummary struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SourcePath  string `json:"source_path"`
	Provider    string `json:"provider"`
	Root        string `json:"root,omitempty"`
	Plugin      string `json:"plugin,omitempty"`
	CanDisable  bool   `json:"can_disable,omitempty"`
	FileCount   int    `json:"file_count"`
}

// runtimeLocalMCPServerSummary deliberately excludes command arguments,
// environment variables, URLs, headers, and every other secret-bearing field.
type runtimeLocalMCPServerSummary struct {
	Name      string `json:"name"`
	Transport string `json:"transport,omitempty"`
	Source    string `json:"source,omitempty"`
	Enabled   bool   `json:"enabled"`
}

type runtimeLocalSkillListRequest struct {
	runtimeRequestMeta
	Skills       []runtimeLocalSkillSummary     `json:"skills,omitempty"`
	Supported    bool                           `json:"supported"`
	MCPServers   []runtimeLocalMCPServerSummary `json:"mcp_servers,omitempty"`
	MCPSupported bool                           `json:"mcp_supported"`
}

type runtimeLocalSkillImportConflict struct {
	ExistingSkillID   string `json:"existing_skill_id"`
	ExistingCreatedBy string `json:"existing_created_by,omitempty"`
	CanOverwrite      bool   `json:"can_overwrite"`
}

type runtimeImportedSkillResponse struct {
	skillResponse
	Files []skillFileResponse `json:"files"`
}

type runtimeLocalSkillImportRequest struct {
	runtimeRequestMeta
	SkillKey         string                           `json:"skill_key"`
	Name             *string                          `json:"name,omitempty"`
	Description      *string                          `json:"description,omitempty"`
	Action           string                           `json:"action,omitempty"`
	TargetSkillID    string                           `json:"target_skill_id,omitempty"`
	SupportsConflict bool                             `json:"supports_conflict,omitempty"`
	Skill            *runtimeImportedSkillResponse    `json:"skill,omitempty"`
	Conflict         *runtimeLocalSkillImportConflict `json:"conflict,omitempty"`
	CreatorID        string                           `json:"-"`
	processing       bool
}

type runtimeRequestStore struct {
	mu          sync.Mutex
	importWrite sync.Mutex
	models      map[string]*runtimeModelListRequest
	localSkills map[string]*runtimeLocalSkillListRequest
	imports     map[string]*runtimeLocalSkillImportRequest
}

func newRuntimeRequestStore() *runtimeRequestStore {
	return &runtimeRequestStore{
		models:      make(map[string]*runtimeModelListRequest),
		localSkills: make(map[string]*runtimeLocalSkillListRequest),
		imports:     make(map[string]*runtimeLocalSkillImportRequest),
	}
}

func newRuntimeRequestMeta(runtimeID string, now time.Time) runtimeRequestMeta {
	return runtimeRequestMeta{
		ID: uuid.NewString(), RuntimeID: runtimeID, Status: runtimeRequestPending,
		CreatedAt: now, UpdatedAt: now,
	}
}

func runtimeRequestTerminal(status runtimeRequestStatus) bool {
	return status == runtimeRequestCompleted || status == runtimeRequestConflict ||
		status == runtimeRequestFailed || status == runtimeRequestTimeout
}

func applyRuntimeRequestTimeout(meta *runtimeRequestMeta, now time.Time, pendingTimeout time.Duration) {
	switch meta.Status {
	case runtimeRequestPending:
		if now.Sub(meta.CreatedAt) > pendingTimeout {
			meta.Status = runtimeRequestTimeout
			meta.Error = "daemon did not claim the request before timeout"
			meta.UpdatedAt = now
		}
	case runtimeRequestRunning:
		if meta.RunStartedAt != nil && now.Sub(*meta.RunStartedAt) > runtimeRequestRunningTimeout {
			meta.Status = runtimeRequestTimeout
			meta.Error = "daemon did not report the result before timeout"
			meta.UpdatedAt = now
		}
	}
}

func (s *runtimeRequestStore) gcLocked(now time.Time) {
	for id, request := range s.models {
		applyRuntimeRequestTimeout(&request.runtimeRequestMeta, now, runtimeRequestPendingTimeout)
		if now.Sub(request.CreatedAt) > runtimeRequestRetention {
			delete(s.models, id)
		}
	}
	for id, request := range s.localSkills {
		applyRuntimeRequestTimeout(&request.runtimeRequestMeta, now, runtimeRequestPendingTimeout)
		if now.Sub(request.CreatedAt) > runtimeRequestRetention {
			delete(s.localSkills, id)
		}
	}
	for id, request := range s.imports {
		applyRuntimeRequestTimeout(&request.runtimeRequestMeta, now, runtimeImportPendingTimeout)
		if now.Sub(request.CreatedAt) > runtimeRequestRetention {
			delete(s.imports, id)
		}
	}
}

func cloneModelRequest(request *runtimeModelListRequest) *runtimeModelListRequest {
	if request == nil {
		return nil
	}
	clone := *request
	clone.Models = append([]runtimeModelEntry(nil), request.Models...)
	return &clone
}

func cloneLocalSkillListRequest(request *runtimeLocalSkillListRequest) *runtimeLocalSkillListRequest {
	if request == nil {
		return nil
	}
	clone := *request
	clone.Skills = append([]runtimeLocalSkillSummary(nil), request.Skills...)
	clone.MCPServers = append([]runtimeLocalMCPServerSummary(nil), request.MCPServers...)
	return &clone
}

func cloneLocalSkillImportRequest(request *runtimeLocalSkillImportRequest) *runtimeLocalSkillImportRequest {
	if request == nil {
		return nil
	}
	clone := *request
	if request.Skill != nil {
		skill := *request.Skill
		skill.Files = append([]skillFileResponse(nil), request.Skill.Files...)
		clone.Skill = &skill
	}
	if request.Conflict != nil {
		conflict := *request.Conflict
		clone.Conflict = &conflict
	}
	return &clone
}

func (s *runtimeRequestStore) createModel(runtimeID string, now time.Time) *runtimeModelListRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	request := &runtimeModelListRequest{runtimeRequestMeta: newRuntimeRequestMeta(runtimeID, now), Supported: true}
	s.models[request.ID] = request
	return cloneModelRequest(request)
}

func (s *runtimeRequestStore) getModel(id string, now time.Time) *runtimeModelListRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	return cloneModelRequest(s.models[id])
}

func (s *runtimeRequestStore) popModel(runtimeID string, now time.Time) *runtimeModelListRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	var oldest *runtimeModelListRequest
	for _, request := range s.models {
		if request.RuntimeID == runtimeID && request.Status == runtimeRequestPending &&
			(oldest == nil || request.CreatedAt.Before(oldest.CreatedAt)) {
			oldest = request
		}
	}
	if oldest != nil {
		oldest.Status = runtimeRequestRunning
		oldest.RunStartedAt = &now
		oldest.UpdatedAt = now
	}
	return cloneModelRequest(oldest)
}

func (s *runtimeRequestStore) finishModel(id, runtimeID, status, errorMessage string, models []runtimeModelEntry, supported bool, now time.Time) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	request := s.models[id]
	if request == nil || request.RuntimeID != runtimeID {
		return false, false
	}
	if runtimeRequestTerminal(request.Status) {
		return true, true
	}
	request.UpdatedAt = now
	if status == "completed" {
		request.Status = runtimeRequestCompleted
		request.Models = append([]runtimeModelEntry(nil), models...)
		request.Supported = supported
		request.Error = ""
	} else {
		request.Status = runtimeRequestFailed
		request.Error = errorMessage
	}
	return true, false
}

func (s *runtimeRequestStore) createLocalSkillList(runtimeID string, now time.Time) *runtimeLocalSkillListRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	request := &runtimeLocalSkillListRequest{runtimeRequestMeta: newRuntimeRequestMeta(runtimeID, now), Supported: true}
	s.localSkills[request.ID] = request
	return cloneLocalSkillListRequest(request)
}

func (s *runtimeRequestStore) getLocalSkillList(id string, now time.Time) *runtimeLocalSkillListRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	return cloneLocalSkillListRequest(s.localSkills[id])
}

func (s *runtimeRequestStore) popLocalSkillList(runtimeID string, now time.Time) *runtimeLocalSkillListRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	var oldest *runtimeLocalSkillListRequest
	for _, request := range s.localSkills {
		if request.RuntimeID == runtimeID && request.Status == runtimeRequestPending &&
			(oldest == nil || request.CreatedAt.Before(oldest.CreatedAt)) {
			oldest = request
		}
	}
	if oldest != nil {
		oldest.Status = runtimeRequestRunning
		oldest.RunStartedAt = &now
		oldest.UpdatedAt = now
	}
	return cloneLocalSkillListRequest(oldest)
}

func (s *runtimeRequestStore) finishLocalSkillList(id, runtimeID, status, errorMessage string, skills []runtimeLocalSkillSummary, supported bool, mcpServers []runtimeLocalMCPServerSummary, mcpSupported bool, now time.Time) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	request := s.localSkills[id]
	if request == nil || request.RuntimeID != runtimeID {
		return false, false
	}
	if runtimeRequestTerminal(request.Status) {
		return true, true
	}
	request.UpdatedAt = now
	if status == "completed" {
		request.Status = runtimeRequestCompleted
		request.Skills = append([]runtimeLocalSkillSummary(nil), skills...)
		request.Supported = supported
		request.MCPServers = append([]runtimeLocalMCPServerSummary(nil), mcpServers...)
		request.MCPSupported = mcpSupported
		request.Error = ""
	} else {
		request.Status = runtimeRequestFailed
		request.Error = errorMessage
	}
	return true, false
}

func (s *runtimeRequestStore) createImport(runtimeID, creatorID, skillKey string, name, description *string, action, targetSkillID string, supportsConflict bool, now time.Time) *runtimeLocalSkillImportRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	request := &runtimeLocalSkillImportRequest{
		runtimeRequestMeta: newRuntimeRequestMeta(runtimeID, now), SkillKey: skillKey,
		Name: name, Description: description, Action: action, TargetSkillID: targetSkillID,
		SupportsConflict: supportsConflict, CreatorID: creatorID,
	}
	s.imports[request.ID] = request
	return cloneLocalSkillImportRequest(request)
}

func (s *runtimeRequestStore) getImport(id string, now time.Time) *runtimeLocalSkillImportRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	return cloneLocalSkillImportRequest(s.imports[id])
}

func (s *runtimeRequestStore) popImports(runtimeID string, limit int, now time.Time) []*runtimeLocalSkillImportRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	pending := make([]*runtimeLocalSkillImportRequest, 0)
	for _, request := range s.imports {
		if request.RuntimeID == runtimeID && request.Status == runtimeRequestPending {
			pending = append(pending, request)
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].CreatedAt.Equal(pending[j].CreatedAt) {
			return pending[i].ID < pending[j].ID
		}
		return pending[i].CreatedAt.Before(pending[j].CreatedAt)
	})
	if limit > len(pending) {
		limit = len(pending)
	}
	result := make([]*runtimeLocalSkillImportRequest, 0, limit)
	for _, request := range pending[:limit] {
		request.Status = runtimeRequestRunning
		request.RunStartedAt = &now
		request.UpdatedAt = now
		result = append(result, cloneLocalSkillImportRequest(request))
	}
	return result
}

func (s *runtimeRequestStore) beginImportResult(id, runtimeID string, now time.Time) (*runtimeLocalSkillImportRequest, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked(now)
	request := s.imports[id]
	if request == nil || request.RuntimeID != runtimeID {
		return nil, false, false
	}
	if runtimeRequestTerminal(request.Status) || request.processing {
		return cloneLocalSkillImportRequest(request), true, true
	}
	request.processing = true
	return cloneLocalSkillImportRequest(request), true, false
}

func (s *runtimeRequestStore) releaseImport(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request := s.imports[id]; request != nil {
		request.processing = false
	}
}

func (s *runtimeRequestStore) finishImport(id string, status runtimeRequestStatus, errorMessage string, skill *runtimeImportedSkillResponse, conflict *runtimeLocalSkillImportConflict, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request := s.imports[id]; request != nil {
		request.Status = status
		request.Error = errorMessage
		request.Skill = skill
		request.Conflict = conflict
		request.UpdatedAt = now
		request.processing = false
	}
}

func cleanRuntimeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	cleaned := strings.TrimSpace(*value)
	if cleaned == "" {
		return nil
	}
	return &cleaned
}

func (h *Handler) runtimeForWorkspaceMember(w http.ResponseWriter, r *http.Request) (lwdb.AgentRuntime, bool) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return lwdb.AgentRuntime{}, false
	}
	runtimeID, err := parseUUID(chi.URLParam(r, "runtimeId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.AgentRuntime{}, false
	}
	runtime, err := h.q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: runtimeID, WorkspaceID: workspaceID})
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return lwdb.AgentRuntime{}, false
	}
	return runtime, true
}

func (h *Handler) notifyRuntimeRequest(runtimeID, kind string) {
	if h.cfg.Wakeup != nil {
		h.cfg.Wakeup.NotifyPendingWork(runtimeID, kind)
	}
}

func (h *Handler) InitiateListModels(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.runtimeForWorkspaceMember(w, r)
	if !ok {
		return
	}
	if runtime.Status != "online" {
		writeCode(w, http.StatusServiceUnavailable, "runtime_offline")
		return
	}
	runtimeID := uuidString(runtime.ID)
	request := h.requests.createModel(runtimeID, h.now())
	h.notifyRuntimeRequest(runtimeID, protocol.PendingWorkKindModelList)
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) GetModelListRequest(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.runtimeForWorkspaceMember(w, r)
	if !ok {
		return
	}
	request := h.requests.getModel(chi.URLParam(r, "requestId"), h.now())
	if request == nil || request.RuntimeID != uuidString(runtime.ID) {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) InitiateListLocalSkills(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.runtimeForWorkspaceMember(w, r)
	if !ok {
		return
	}
	if runtime.Status != "online" {
		writeCode(w, http.StatusServiceUnavailable, "runtime_offline")
		return
	}
	runtimeID := uuidString(runtime.ID)
	request := h.requests.createLocalSkillList(runtimeID, h.now())
	h.notifyRuntimeRequest(runtimeID, "local_skills")
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) GetLocalSkillListRequest(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.runtimeForWorkspaceMember(w, r)
	if !ok {
		return
	}
	request := h.requests.getLocalSkillList(chi.URLParam(r, "requestId"), h.now())
	if request == nil || request.RuntimeID != uuidString(runtime.ID) {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, request)
}

type createRuntimeLocalSkillImportRequest struct {
	SkillKey         string  `json:"skill_key"`
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	Action           string  `json:"action"`
	TargetSkillID    string  `json:"target_skill_id"`
	SupportsConflict bool    `json:"supports_conflict"`
}

func (h *Handler) InitiateImportLocalSkill(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.runtimeForWorkspaceMember(w, r)
	if !ok {
		return
	}
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.UserID != uuidString(runtime.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	if runtime.Status != "online" {
		writeCode(w, http.StatusServiceUnavailable, "runtime_offline")
		return
	}
	var body createRuntimeLocalSkillImportRequest
	if err := decodeStrictTaskBody(r, &body); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	body.SkillKey = strings.TrimSpace(body.SkillKey)
	body.Name = cleanRuntimeOptional(body.Name)
	body.Description = cleanRuntimeOptional(body.Description)
	if body.SkillKey == "" || len(body.SkillKey) > 1000 || (body.Name != nil && len(*body.Name) > 200) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if body.Action != "" && body.Action != "overwrite" {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if body.Action == "overwrite" {
		target, err := parseUUID(body.TargetSkillID)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		body.TargetSkillID = uuidString(target)
		body.SupportsConflict = true
	} else if strings.TrimSpace(body.TargetSkillID) != "" {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	runtimeID := uuidString(runtime.ID)
	request := h.requests.createImport(runtimeID, principal.UserID, body.SkillKey, body.Name, body.Description,
		body.Action, body.TargetSkillID, body.SupportsConflict, h.now())
	h.notifyRuntimeRequest(runtimeID, "local_skill_import")
	writeJSON(w, http.StatusOK, request)
}

func (h *Handler) GetLocalSkillImportRequest(w http.ResponseWriter, r *http.Request) {
	runtime, ok := h.runtimeForWorkspaceMember(w, r)
	if !ok {
		return
	}
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.UserID != uuidString(runtime.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	request := h.requests.getImport(chi.URLParam(r, "requestId"), h.now())
	if request == nil || request.RuntimeID != uuidString(runtime.ID) || request.CreatorID != principal.UserID {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, request)
}

type runtimeModelListReport struct {
	Status    string              `json:"status"`
	Models    []runtimeModelEntry `json:"models"`
	Supported *bool               `json:"supported"`
	Error     string              `json:"error"`
	Fallback  bool                `json:"fallback"`
}

func (h *Handler) ReportModelListResult(w http.ResponseWriter, r *http.Request) {
	_, _, runtime, ok := h.daemonRuntimeScope(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	var body runtimeModelListReport
	if err := decodeStrictTaskBody(r, &body); err != nil || (body.Status != "completed" && body.Status != "failed") {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if body.Status == "completed" {
		for _, model := range body.Models {
			if strings.TrimSpace(model.ID) == "" {
				writeCode(w, http.StatusBadRequest, "invalid_argument")
				return
			}
		}
	}
	supported := true
	if body.Supported != nil {
		supported = *body.Supported
	}
	found, _ := h.requests.finishModel(chi.URLParam(r, "requestId"), uuidString(runtime.ID), body.Status,
		strings.TrimSpace(body.Error), body.Models, supported, h.now())
	if !found {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type runtimeLocalSkillListReport struct {
	Status       string                         `json:"status"`
	Skills       []runtimeLocalSkillSummary     `json:"skills"`
	Supported    *bool                          `json:"supported"`
	MCPServers   []runtimeLocalMCPServerSummary `json:"mcp_servers"`
	MCPSupported *bool                          `json:"mcp_supported"`
	Error        string                         `json:"error"`
}

func (h *Handler) ReportLocalSkillListResult(w http.ResponseWriter, r *http.Request) {
	_, _, runtime, ok := h.daemonRuntimeScope(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	var body runtimeLocalSkillListReport
	if err := decodeStrictTaskBody(r, &body); err != nil || (body.Status != "completed" && body.Status != "failed") {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	supported := true
	if body.Supported != nil {
		supported = *body.Supported
	}
	mcpSupported := false
	if body.MCPSupported != nil {
		mcpSupported = *body.MCPSupported
	}
	found, _ := h.requests.finishLocalSkillList(chi.URLParam(r, "requestId"), uuidString(runtime.ID), body.Status,
		strings.TrimSpace(body.Error), body.Skills, supported, body.MCPServers, mcpSupported, h.now())
	if !found {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type reportedRuntimeLocalSkill struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Content     string             `json:"content"`
	SourcePath  string             `json:"source_path"`
	Provider    string             `json:"provider"`
	Files       []skillFileRequest `json:"files"`
}

type runtimeLocalSkillImportReport struct {
	Status string                     `json:"status"`
	Skill  *reportedRuntimeLocalSkill `json:"skill"`
	Error  string                     `json:"error"`
}

func normalizeRuntimeSkillFiles(files []skillFileRequest) ([]skillFileRequest, bool) {
	result := make([]skillFileRequest, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		file.Path = path.Clean(strings.TrimSpace(file.Path))
		if file.Path == "." || strings.HasPrefix(file.Path, "/") || strings.HasPrefix(file.Path, "../") {
			return nil, false
		}
		if _, duplicate := seen[file.Path]; duplicate {
			return nil, false
		}
		seen[file.Path] = struct{}{}
		result = append(result, file)
	}
	return result, true
}

func (h *Handler) ReportLocalSkillImportResult(w http.ResponseWriter, r *http.Request) {
	_, workspaceID, runtime, ok := h.daemonRuntimeScope(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	requestID := chi.URLParam(r, "requestId")
	request, found, terminal := h.requests.beginImportResult(requestID, uuidString(runtime.ID), h.now())
	if !found {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	if terminal {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	var body runtimeLocalSkillImportReport
	if err := decodeStrictTaskBody(r, &body); err != nil || (body.Status != "completed" && body.Status != "failed") {
		h.requests.releaseImport(requestID)
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if body.Status == "failed" {
		h.requests.finishImport(requestID, runtimeRequestFailed, strings.TrimSpace(body.Error), nil, nil, h.now())
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if body.Skill == nil {
		h.requests.releaseImport(requestID)
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	name := strings.TrimSpace(body.Skill.Name)
	if request.Name != nil {
		name = *request.Name
	}
	description := body.Skill.Description
	if request.Description != nil {
		description = *request.Description
	}
	files, valid := normalizeRuntimeSkillFiles(body.Skill.Files)
	if name == "" || len(name) > 200 || !valid {
		h.requests.releaseImport(requestID)
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	creatorID, err := parseUUID(request.CreatorID)
	if err != nil {
		h.requests.finishImport(requestID, runtimeRequestFailed, "invalid stored creator", nil, nil, h.now())
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	config, _ := json.Marshal(map[string]any{"origin": map[string]any{
		"type": "runtime_local", "runtime_id": uuidString(runtime.ID),
		"provider": body.Skill.Provider, "source_path": body.Skill.SourcePath,
	}})

	h.requests.importWrite.Lock()
	defer h.requests.importWrite.Unlock()
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		h.requests.releaseImport(requestID)
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	var skill lwdb.Skill
	eventType := protocol.EventSkillCreated
	if request.Action == "overwrite" {
		targetID, parseErr := parseUUID(request.TargetSkillID)
		if parseErr != nil {
			h.requests.finishImport(requestID, runtimeRequestFailed, "invalid overwrite target", nil, nil, h.now())
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		existing, lookupErr := qtx.GetSkill(r.Context(), lwdb.GetSkillParams{ID: targetID, WorkspaceID: workspaceID})
		if lookupErr != nil || existing.CreatedBy != creatorID {
			h.requests.finishImport(requestID, runtimeRequestFailed, "overwrite target is unavailable", nil, nil, h.now())
			writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		skill, err = qtx.UpdateSkill(r.Context(), lwdb.UpdateSkillParams{
			ID: targetID, WorkspaceID: workspaceID, SetName: true, Name: name,
			SetDescription: true, Description: description, SetContent: true, Content: body.Skill.Content,
			SetConfig: true, Config: config,
		})
		if err == nil {
			_, err = qtx.DeleteSkillFilesBySkill(r.Context(), targetID)
		}
		eventType = protocol.EventSkillUpdated
	} else {
		existingSkills, listErr := qtx.ListSkills(r.Context(), workspaceID)
		if listErr != nil {
			err = listErr
		} else {
			for _, existing := range existingSkills {
				if strings.EqualFold(strings.TrimSpace(existing.Name), name) {
					if request.SupportsConflict {
						conflict := &runtimeLocalSkillImportConflict{
							ExistingSkillID: uuidString(existing.ID), ExistingCreatedBy: uuidString(existing.CreatedBy),
							CanOverwrite: existing.CreatedBy == creatorID,
						}
						h.requests.finishImport(requestID, runtimeRequestConflict, "", nil, conflict, h.now())
					} else {
						h.requests.finishImport(requestID, runtimeRequestFailed, "a skill with this name already exists", nil, nil, h.now())
					}
					writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
					return
				}
			}
			skill, err = qtx.CreateSkill(r.Context(), lwdb.CreateSkillParams{
				WorkspaceID: workspaceID, Name: name, Description: description,
				Content: body.Skill.Content, Config: config, CreatedBy: creatorID,
			})
		}
	}
	if err != nil {
		h.requests.releaseImport(requestID)
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	fileResponses := make([]skillFileResponse, 0, len(files))
	for _, file := range files {
		row, insertErr := qtx.UpsertSkillFile(r.Context(), lwdb.UpsertSkillFileParams{SkillID: skill.ID, Path: file.Path, Content: file.Content})
		if insertErr != nil {
			h.requests.releaseImport(requestID)
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		fileResponses = append(fileResponses, toSkillFileResponse(row))
	}
	if err := tx.Commit(r.Context()); err != nil {
		h.requests.releaseImport(requestID)
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := &runtimeImportedSkillResponse{skillResponse: toSkillResponse(skill), Files: fileResponses}
	h.requests.finishImport(requestID, runtimeRequestCompleted, "", response, nil, h.now())
	h.publish(eventType, uuidString(workspaceID), "member", request.CreatorID, map[string]any{"skill": response})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) runtimeRequestHeartbeatAck(runtimeID string, supportsBatchImport bool) *protocol.DaemonHeartbeatAckPayload {
	now := h.now()
	ack := &protocol.DaemonHeartbeatAckPayload{
		RuntimeID: runtimeID, Status: "ok", ServerCapabilities: []string{protocol.DaemonCapabilityRPCV1},
	}
	if request := h.requests.popModel(runtimeID, now); request != nil {
		ack.PendingModelList = &protocol.DaemonHeartbeatPendingModelList{ID: request.ID}
	}
	if request := h.requests.popLocalSkillList(runtimeID, now); request != nil {
		ack.PendingLocalSkills = &protocol.DaemonHeartbeatPendingLocalSkills{ID: request.ID}
	}
	limit := 1
	if supportsBatchImport {
		limit = maxRuntimeImportBatch
	}
	imports := h.requests.popImports(runtimeID, limit, now)
	if supportsBatchImport && len(imports) > 0 {
		ack.PendingLocalSkillImports = make([]protocol.DaemonHeartbeatPendingLocalSkillImport, 0, len(imports))
		for _, request := range imports {
			ack.PendingLocalSkillImports = append(ack.PendingLocalSkillImports, protocol.DaemonHeartbeatPendingLocalSkillImport{ID: request.ID, SkillKey: request.SkillKey})
		}
	} else if len(imports) > 0 {
		ack.PendingLocalSkillImport = &protocol.DaemonHeartbeatPendingLocalSkillImport{ID: imports[0].ID, SkillKey: imports[0].SkillKey}
	}
	return ack
}
