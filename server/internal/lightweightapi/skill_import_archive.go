package lightweightapi

import (
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	skillpkg "github.com/kms9/dars/internal/skill"
)

// maxImportArchiveUploadSize bounds the compressed upload accepted by the
// archive import path. The decompressed bundle is still held to the existing
// per-file / total / file-count caps; this outer cap just stops a client from
// streaming an unbounded compressed body before those decompression limits apply.
const maxImportArchiveUploadSize = 16 << 20 // 16 MiB

func isMultipartForm(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data")
}

func (h *Handler) importSkillFromArchive(w http.ResponseWriter, r *http.Request, workspaceUUID, creatorUUID pgtype.UUID, creatorID string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportArchiveUploadSize)
	if err := r.ParseMultipartForm(maxImportArchiveUploadSize); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart upload or file exceeds the size limit"})
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	onConflict := r.FormValue("on_conflict")
	if !validImportOnConflict(onConflict) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "on_conflict must be one of: fail, overwrite, rename, skip"})
		return
	}
	strategy := onConflict
	if strategy == "" {
		strategy = importOnConflictFail
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `a skill archive file is required (form field "file")`})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read uploaded file"})
		return
	}

	filename := ""
	if header != nil {
		filename = header.Filename
	}
	imported, err := skillpkg.ParseSkillArchive(data, filename)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if imported.Origin == nil {
		imported.Origin = map[string]any{"type": "archive"}
	}

	h.finishSkillImport(w, r, workspaceUUID, creatorUUID, creatorID, strategy, true, imported)
}
