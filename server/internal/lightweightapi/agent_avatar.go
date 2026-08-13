package lightweightapi

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	avatarMaxCompressedBytes = 5 * 1024 * 1024
	avatarMaxDimension       = 1024
)

var allowedAvatarContentTypes = map[string]string{
	"image/jpeg": "image/jpeg",
	"image/jpg":  "image/jpeg",
	"image/png":  "image/png",
}

func (h *Handler) UploadAgentAvatar(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgent(w, r)
	if !ok {
		return
	}
	if !h.canManage(r, agent.OwnerID) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	h.uploadEntityAvatar(w, r, agent.WorkspaceID, avatarEntityAgent, agent.ID, agent.AvatarUrl)
}

func (h *Handler) UploadSquadAvatar(w http.ResponseWriter, r *http.Request) {
	squad, ok := h.loadSquad(w, r)
	if !ok {
		return
	}
	if !h.canManageSquad(r, squad) {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	h.uploadEntityAvatar(w, r, squad.WorkspaceID, avatarEntitySquad, squad.ID, squad.AvatarUrl)
}

type avatarEntityKind string

const (
	avatarEntityAgent avatarEntityKind = "agent"
	avatarEntitySquad avatarEntityKind = "squad"
)

func (h *Handler) uploadEntityAvatar(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID, kind avatarEntityKind, entityID pgtype.UUID, previousURL pgtype.Text) {
	contentType, body, err := decodeAvatarUpload(r)
	if err != nil {
		if errors.Is(err, errAvatarPayloadTooLarge) {
			writeCode(w, http.StatusRequestEntityTooLarge, "avatar_invalid")
			return
		}
		writeCode(w, http.StatusBadRequest, "avatar_invalid")
		return
	}
	avatarID := uuid.NewString()
	mediaURL := avatarMediaURL(avatarID)
	store := h.avatarStore()
	if err := store.Put(r.Context(), avatarID, contentType, body); err != nil {
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
	var updatedAvatarURL pgtype.Text
	switch kind {
	case avatarEntityAgent:
		agent, err := qtx.UpdateAgentAvatar(r.Context(), lwdb.UpdateAgentAvatarParams{
			ID: entityID, WorkspaceID: workspaceID, AvatarUrl: pgtype.Text{String: mediaURL, Valid: true},
		})
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		updatedAvatarURL = agent.AvatarUrl
	case avatarEntitySquad:
		squad, err := qtx.UpdateSquadAvatar(r.Context(), lwdb.UpdateSquadAvatarParams{
			ID: entityID, WorkspaceID: workspaceID, AvatarUrl: pgtype.Text{String: mediaURL, Valid: true},
		})
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		updatedAvatarURL = squad.AvatarUrl
	default:
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if previousURL.Valid && previousURL.String != "" && previousURL.String != mediaURL {
		_ = h.cleanupAvatarObjects(r.Context(), []string{previousURL.String})
	}
	principal, _ := principalFromContext(r.Context())
	switch kind {
	case avatarEntityAgent:
		h.publish(protocol.EventAgentUpdated, uuidString(workspaceID), "member", principal.UserID, map[string]any{
			"agent_id": uuidString(entityID), "avatar_url": textPointer(updatedAvatarURL),
		})
	case avatarEntitySquad:
		squad, err := h.q.GetSquad(r.Context(), lwdb.GetSquadParams{ID: entityID, WorkspaceID: workspaceID})
		if err == nil {
			dto, err := h.squadDTO(r, squad)
			if err == nil {
				h.publish(protocol.EventSquadUpdated, uuidString(workspaceID), "member", principal.UserID, map[string]any{"squad": dto})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"avatar_url": mediaURL})
}

func (h *Handler) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	avatarID := strings.TrimSpace(chi.URLParam(r, "avatarId"))
	if avatarID == "" || strings.Contains(avatarID, "/") {
		http.NotFound(w, r)
		return
	}
	mediaURL := avatarMediaURL(avatarID)
	if !h.avatarURLIsBound(r.Context(), mediaURL) {
		http.NotFound(w, r)
		return
	}
	store := h.avatarStore()
	contentType, body, err := store.Get(r.Context(), avatarID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) avatarURLIsBound(ctx context.Context, mediaURL string) bool {
	text := pgtype.Text{String: mediaURL, Valid: true}
	if _, err := h.q.GetAgentByAvatarURL(ctx, text); err == nil {
		return true
	}
	if _, err := h.q.GetSquadByAvatarURL(ctx, text); err == nil {
		return true
	}
	return false
}

func avatarMediaURL(avatarID string) string {
	return "/media/avatars/" + avatarID
}

var errAvatarPayloadTooLarge = errors.New("avatar payload too large")

func decodeAvatarUpload(r *http.Request) (string, []byte, error) {
	if err := r.ParseMultipartForm(avatarMaxCompressedBytes + 1024); err != nil {
		return "", nil, err
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		file, header, err = r.FormFile("avatar")
	}
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	limited := io.LimitReader(file, avatarMaxCompressedBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", nil, err
	}
	if len(raw) > avatarMaxCompressedBytes {
		return "", nil, errAvatarPayloadTooLarge
	}
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if mapped, ok := allowedAvatarContentTypes[contentType]; ok {
		contentType = mapped
	} else {
		switch ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(header.Filename), ".")); ext {
		case "jpg", "jpeg":
			contentType = "image/jpeg"
		case "png":
			contentType = "image/png"
		default:
			return "", nil, errors.New("unsupported avatar type")
		}
	}
	return processAvatarBytes(contentType, raw)
}

func processAvatarBytes(contentType string, raw []byte) (string, []byte, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > avatarMaxDimension || cfg.Height > avatarMaxDimension {
		return "", nil, errors.New("avatar dimensions out of range")
	}
	if format == "gif" {
		return "", nil, errors.New("animated avatars are not supported")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", nil, err
	}
	bounds := img.Bounds()
	if bounds.Dx() > avatarMaxDimension || bounds.Dy() > avatarMaxDimension {
		return "", nil, errors.New("avatar dimensions out of range")
	}
	var encoded bytes.Buffer
	switch contentType {
	case "image/jpeg":
		if err := jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 90}); err != nil {
			return "", nil, err
		}
		return "image/jpeg", encoded.Bytes(), nil
	case "image/png":
		if err := png.Encode(&encoded, img); err != nil {
			return "", nil, err
		}
		return "image/png", encoded.Bytes(), nil
	default:
		return "", nil, errors.New("unsupported avatar type")
	}
}
