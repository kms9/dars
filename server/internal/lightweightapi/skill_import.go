package lightweightapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	skillpkg "github.com/kms9/dars/internal/skill"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type importSkillRequest struct {
	URL        string `json:"url"`
	OnConflict string `json:"on_conflict,omitempty"`
}

type skillWithFilesResponse struct {
	skillResponse
	Files []skillFileResponse `json:"files"`
}

type skillImportResult struct {
	Status        string                  `json:"status"`
	Reason        string                  `json:"reason,omitempty"`
	Skill         *skillWithFilesResponse `json:"skill,omitempty"`
	ExistingSkill *existingSkillIdentity  `json:"existing_skill,omitempty"`
}

type existingSkillIdentity struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	CreatedBy    string `json:"created_by,omitempty"`
	CanOverwrite bool   `json:"can_overwrite,omitempty"`
}

type skillFileInput struct {
	Path    string
	Content string
}

const (
	importOnConflictFail      = "fail"
	importOnConflictOverwrite = "overwrite"
	importOnConflictRename    = "rename"
	importOnConflictSkip      = "skip"
)

const maxImportRenameAttempts = 50

const importFetchTimeout = 45 * time.Second

var (
	errSkillOverwriteNotFound     = errors.New("target skill not found")
	errSkillOverwriteForbidden    = errors.New("not permitted to overwrite target skill")
	errSkillOverwriteNameMismatch = errors.New("target skill name does not match the imported skill")
)

func validImportOnConflict(strategy string) bool {
	switch strategy {
	case "", importOnConflictFail, importOnConflictOverwrite, importOnConflictRename, importOnConflictSkip:
		return true
	}
	return false
}

func canOverwriteSkillByImport(userID string, skill lwdb.Skill) bool {
	return skill.CreatedBy.Valid && uuidString(skill.CreatedBy) == userID
}

func existingSkillIdentityFrom(skill lwdb.Skill, userID string) existingSkillIdentity {
	identity := existingSkillIdentity{
		ID:           uuidString(skill.ID),
		Name:         skill.Name,
		CanOverwrite: canOverwriteSkillByImport(userID, skill),
	}
	if skill.CreatedBy.Valid {
		identity.CreatedBy = uuidString(skill.CreatedBy)
	}
	return identity
}

func writeSkillImportDuplicateConflict(w http.ResponseWriter, existing existingSkillIdentity) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":          "a skill with this name already exists",
		"existing_skill": existing,
	})
}

func skillImportConflictReason() string {
	return "a skill with this name already exists; use --on-conflict overwrite to replace it or --on-conflict rename to import a copy"
}

func importFetchErrorResponse(ctx context.Context, err error) (int, string) {
	if skillpkg.IsCapError(err) {
		return http.StatusRequestEntityTooLarge, err.Error()
	}
	if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
		return http.StatusGatewayTimeout, "skill import timed out fetching source files; the skill may be too large or the source too slow"
	}
	if errors.Is(err, ErrImportSourceUnavailable) {
		return http.StatusServiceUnavailable, err.Error()
	}
	return http.StatusBadGateway, err.Error()
}

func (h *Handler) lookupSkillByName(ctx context.Context, workspaceID pgtype.UUID, name string) (lwdb.Skill, bool, error) {
	skill, err := h.q.GetSkillByWorkspaceAndName(ctx, lwdb.GetSkillByWorkspaceAndNameParams{
		WorkspaceID: workspaceID,
		Name:        name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return lwdb.Skill{}, false, nil
		}
		return lwdb.Skill{}, false, err
	}
	return skill, true, nil
}

func (h *Handler) ImportSkill(w http.ResponseWriter, r *http.Request) {
	workspaceID, err := contextWorkspaceUUID(r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.UserID == "" {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	creatorUUID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if isMultipartForm(r) {
		h.importSkillFromArchive(w, r, workspaceID, creatorUUID, principal.UserID)
		return
	}

	var req importSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if !validImportOnConflict(req.OnConflict) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "on_conflict must be one of: fail, overwrite, rename, skip"})
		return
	}
	structuredResult := req.OnConflict != ""
	strategy := req.OnConflict
	if strategy == "" {
		strategy = importOnConflictFail
	}

	source, normalized, err := detectImportSource(req.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(r.Context(), importFetchTimeout)
	defer cancel()

	var imported *skillpkg.ImportedSkill
	switch source {
	case sourceClawHub:
		imported, err = fetchFromClawHub(ctx, httpClient, normalized)
	case sourceSkillsSh:
		imported, err = fetchFromSkillsSh(ctx, httpClient, normalized)
	case sourceGitHub:
		imported, err = fetchFromGitHub(ctx, httpClient, normalized)
	}
	if err != nil {
		status, msg := importFetchErrorResponse(ctx, err)
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}

	h.finishSkillImport(w, r, workspaceID, creatorUUID, principal.UserID, strategy, structuredResult, imported)
}

func (h *Handler) finishSkillImport(w http.ResponseWriter, r *http.Request, workspaceUUID, creatorUUID pgtype.UUID, creatorID, strategy string, structuredResult bool, imported *skillpkg.ImportedSkill) {
	files := make([]skillFileInput, 0, len(imported.Files))
	for _, f := range imported.Files {
		if !skillpkg.ValidateFilePath(f.Path) {
			continue
		}
		files = append(files, skillFileInput{Path: f.Path, Content: f.Content})
	}

	config := map[string]any{}
	if imported.Origin != nil {
		config["origin"] = imported.Origin
	}
	name := skillpkg.SanitizeNullBytes(imported.Name)

	if structuredResult {
		if existing, found, lerr := h.lookupSkillByName(r.Context(), workspaceUUID, name); lerr != nil {
			writeJSON(w, http.StatusInternalServerError, skillImportResult{
				Status: "failed",
				Reason: "failed to check for existing skill: " + lerr.Error(),
			})
			return
		} else if found {
			h.resolveImportSkillConflict(w, r, strategy, workspaceUUID, creatorUUID, creatorID, name, imported, config, files, existing)
			return
		}
	} else {
		if existing, found, lerr := h.lookupSkillByName(r.Context(), workspaceUUID, name); lerr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to check for existing skill"})
			return
		} else if found {
			writeSkillImportDuplicateConflict(w, existingSkillIdentityFrom(existing, creatorID))
			return
		}
	}

	resp, err := h.createImportedSkillWithName(r.Context(), workspaceUUID, creatorUUID, name, imported, config, files)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create skill: " + err.Error()})
		return
	}
	if structuredResult {
		writeJSON(w, http.StatusCreated, skillImportResult{Status: "created", Skill: &resp})
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) resolveImportSkillConflict(
	w http.ResponseWriter, r *http.Request, strategy string,
	workspaceUUID, creatorUUID pgtype.UUID, creatorID, name string,
	imported *skillpkg.ImportedSkill, config map[string]any, files []skillFileInput, existing lwdb.Skill,
) {
	existingInfo := existingSkillIdentityFrom(existing, creatorID)
	switch strategy {
	case importOnConflictSkip:
		writeJSON(w, http.StatusOK, skillImportResult{
			Status:        "skipped",
			Reason:        "a skill with this name already exists",
			ExistingSkill: &existingInfo,
		})
	case importOnConflictOverwrite:
		if !canOverwriteSkillByImport(creatorID, existing) {
			writeJSON(w, http.StatusForbidden, skillImportResult{
				Status:        "failed",
				Reason:        "only the skill creator can overwrite this skill",
				ExistingSkill: &existingInfo,
			})
			return
		}
		resp, err := h.overwriteImportedSkill(r.Context(), workspaceUUID, existing.ID, creatorID, name, imported, config, files)
		if err != nil {
			status, reason := skillImportOverwriteFailure(err)
			writeJSON(w, status, skillImportResult{
				Status:        "failed",
				Reason:        reason,
				ExistingSkill: &existingInfo,
			})
			return
		}
		writeJSON(w, http.StatusOK, skillImportResult{Status: "updated", Skill: &resp})
	case importOnConflictRename:
		resp, err := h.createRenamedImportedSkill(r.Context(), workspaceUUID, creatorUUID, name, imported, config, files)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, skillImportResult{
				Status:        "failed",
				Reason:        "failed to create renamed skill: " + err.Error(),
				ExistingSkill: &existingInfo,
			})
			return
		}
		writeJSON(w, http.StatusCreated, skillImportResult{
			Status:        "created",
			Reason:        "renamed to avoid an existing skill",
			Skill:         &resp,
			ExistingSkill: &existingInfo,
		})
	default:
		writeJSON(w, http.StatusConflict, skillImportResult{
			Status:        "conflict",
			Reason:        skillImportConflictReason(),
			ExistingSkill: &existingInfo,
		})
	}
}

func skillImportOverwriteFailure(err error) (int, string) {
	switch {
	case errors.Is(err, errSkillOverwriteNotFound):
		return http.StatusConflict, "target skill no longer exists"
	case errors.Is(err, errSkillOverwriteForbidden):
		return http.StatusForbidden, "only the skill creator can overwrite this skill"
	case errors.Is(err, errSkillOverwriteNameMismatch):
		return http.StatusConflict, "target skill name no longer matches the imported skill"
	default:
		return http.StatusInternalServerError, "failed to overwrite skill: " + err.Error()
	}
}

func (h *Handler) createImportedSkillWithName(
	ctx context.Context, workspaceID, creatorID pgtype.UUID, name string,
	imported *skillpkg.ImportedSkill, config map[string]any, files []skillFileInput,
) (skillWithFilesResponse, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return skillWithFilesResponse{}, err
	}
	if config == nil {
		configJSON = []byte("{}")
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return skillWithFilesResponse{}, err
	}
	defer tx.Rollback(ctx)
	qtx := h.q.WithTx(tx)

	skill, err := qtx.CreateSkill(ctx, lwdb.CreateSkillParams{
		WorkspaceID: workspaceID,
		Name:        skillpkg.SanitizeNullBytes(name),
		Description: skillpkg.SanitizeNullBytes(imported.Description),
		Content:     skillpkg.SanitizeNullBytes(imported.Content),
		Config:      configJSON,
		CreatedBy:   creatorID,
	})
	if err != nil {
		return skillWithFilesResponse{}, err
	}

	fileResps := make([]skillFileResponse, 0, len(files))
	for _, f := range files {
		if skillpkg.IsReservedContentPath(f.Path) {
			continue
		}
		sf, err := qtx.UpsertSkillFile(ctx, lwdb.UpsertSkillFileParams{
			SkillID: skill.ID,
			Path:    skillpkg.SanitizeNullBytes(f.Path),
			Content: skillpkg.SanitizeNullBytes(f.Content),
		})
		if err != nil {
			return skillWithFilesResponse{}, err
		}
		fileResps = append(fileResps, toSkillFileResponse(sf))
	}
	if err := tx.Commit(ctx); err != nil {
		return skillWithFilesResponse{}, err
	}
	return skillWithFilesResponse{skillResponse: toSkillResponse(skill), Files: fileResps}, nil
}

func (h *Handler) createRenamedImportedSkill(
	ctx context.Context, workspaceID, creatorID pgtype.UUID, baseName string,
	imported *skillpkg.ImportedSkill, config map[string]any, files []skillFileInput,
) (skillWithFilesResponse, error) {
	for suffix := 2; suffix < maxImportRenameAttempts+2; suffix++ {
		candidate := fmt.Sprintf("%s-%d", baseName, suffix)
		if _, found, err := h.lookupSkillByName(ctx, workspaceID, candidate); err != nil {
			return skillWithFilesResponse{}, err
		} else if found {
			continue
		}
		return h.createImportedSkillWithName(ctx, workspaceID, creatorID, candidate, imported, config, files)
	}
	return skillWithFilesResponse{}, fmt.Errorf("failed to find an available renamed skill name after %d attempts", maxImportRenameAttempts)
}

func (h *Handler) overwriteImportedSkill(
	ctx context.Context, workspaceID, targetID pgtype.UUID, userID, expectedName string,
	imported *skillpkg.ImportedSkill, config map[string]any, files []skillFileInput,
) (skillWithFilesResponse, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return skillWithFilesResponse{}, err
	}
	if config == nil {
		configJSON = []byte("{}")
	}

	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return skillWithFilesResponse{}, err
	}
	defer tx.Rollback(ctx)
	qtx := h.q.WithTx(tx)

	existing, err := qtx.LockSkill(ctx, lwdb.LockSkillParams{ID: targetID, WorkspaceID: workspaceID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return skillWithFilesResponse{}, errSkillOverwriteNotFound
		}
		return skillWithFilesResponse{}, err
	}
	if !canOverwriteSkillByImport(userID, existing) {
		return skillWithFilesResponse{}, errSkillOverwriteForbidden
	}
	if expectedName != "" && existing.Name != expectedName {
		return skillWithFilesResponse{}, errSkillOverwriteNameMismatch
	}

	skill, err := qtx.UpdateSkill(ctx, lwdb.UpdateSkillParams{
		ID:             existing.ID,
		WorkspaceID:    workspaceID,
		SetDescription: true,
		Description:    skillpkg.SanitizeNullBytes(imported.Description),
		SetContent:     true,
		Content:        skillpkg.SanitizeNullBytes(imported.Content),
		SetConfig:      true,
		Config:         configJSON,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return skillWithFilesResponse{}, errSkillOverwriteNotFound
		}
		return skillWithFilesResponse{}, err
	}
	if _, err := qtx.DeleteSkillFilesBySkill(ctx, skill.ID); err != nil {
		return skillWithFilesResponse{}, err
	}
	fileResps := make([]skillFileResponse, 0, len(files))
	for _, f := range files {
		if skillpkg.IsReservedContentPath(f.Path) {
			continue
		}
		sf, err := qtx.UpsertSkillFile(ctx, lwdb.UpsertSkillFileParams{
			SkillID: skill.ID,
			Path:    skillpkg.SanitizeNullBytes(f.Path),
			Content: skillpkg.SanitizeNullBytes(f.Content),
		})
		if err != nil {
			return skillWithFilesResponse{}, err
		}
		fileResps = append(fileResps, toSkillFileResponse(sf))
	}
	if err := tx.Commit(ctx); err != nil {
		return skillWithFilesResponse{}, err
	}
	return skillWithFilesResponse{skillResponse: toSkillResponse(skill), Files: fileResps}, nil
}
