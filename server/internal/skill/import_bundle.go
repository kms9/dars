package skill

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// Per-import bundle limits. These mirror the local-runtime importer so that
// URL/archive imports cannot smuggle in payloads that the rest of the stack
// would reject.
const (
	MaxImportFileSize  = 1 << 20 // 1 MiB per file
	MaxImportTotalSize = 8 << 20 // 8 MiB per import bundle (sum of supporting files)
	MaxImportFileCount = 256     // max number of supporting files
)

// ErrImportCapExceeded marks an error caused by a per-file or per-bundle cap.
// Such errors must abort the import — silently dropping a file would otherwise
// produce an incomplete skill that looks valid to the user.
var ErrImportCapExceeded = errors.New("import cap exceeded")

// IsCapError reports whether err is (or wraps) ErrImportCapExceeded.
func IsCapError(err error) bool {
	return errors.Is(err, ErrImportCapExceeded)
}

// ImportedSkill holds the data extracted from an external source or archive.
type ImportedSkill struct {
	Name        string
	Description string
	Content     string // SKILL.md body
	Files       []ImportedFile
	BundleSize  int            // running sum of file content bytes for cap enforcement
	Origin      map[string]any // written into skill.config.origin
}

// ImportedFile is one supporting file in an imported skill bundle.
type ImportedFile struct {
	Path    string
	Content string
}

// SanitizeNullBytes makes a string safe for a PostgreSQL TEXT column.
//
// Two failure modes covered:
//   - Embedded NUL (0x00) — PG rejects with SQLSTATE 22021. Removed.
//   - Other invalid-UTF-8 byte sequences. strings.ToValidUTF8 drops them.
func SanitizeNullBytes(s string) string {
	return strings.ToValidUTF8(strings.ReplaceAll(s, "\x00", ""), "")
}

// ValidateFilePath checks that a file path is safe (no traversal, no absolute paths).
func ValidateFilePath(p string) bool {
	if p == "" {
		return false
	}
	if filepath.IsAbs(p) {
		return false
	}
	cleaned := filepath.Clean(p)
	if strings.HasPrefix(cleaned, "..") {
		return false
	}
	return true
}

// AddFile appends a supporting file while enforcing the per-bundle caps. It
// returns an error when either the file count or aggregate byte budget would
// be exceeded so the caller fails the import instead of silently truncating.
//
// Binary files (images, fonts, archives) are silently skipped: their bytes
// can't survive a PG TEXT column (SQLSTATE 22021), and they're reference
// assets the agent never reads as text anyway.
func (s *ImportedSkill) AddFile(path, content string) error {
	if IsLikelyBinaryFilePath(path) {
		slog.Info("skill import: skipping binary file", "path", path, "size", len(content))
		return nil
	}
	if len(s.Files) >= MaxImportFileCount {
		return fmt.Errorf("%w: import bundle exceeds %d file limit", ErrImportCapExceeded, MaxImportFileCount)
	}
	if s.BundleSize+len(content) > MaxImportTotalSize {
		return fmt.Errorf("%w: import bundle exceeds %d byte limit", ErrImportCapExceeded, MaxImportTotalSize)
	}
	s.BundleSize += len(content)
	s.Files = append(s.Files, ImportedFile{Path: path, Content: content})
	return nil
}

// IsLikelyBinaryFilePath reports whether the file's extension indicates a
// non-text payload. Conservative blacklist — extensions not on the list
// are assumed text and pass through.
func IsLikelyBinaryFilePath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case
		// images
		".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff", ".ico", ".heic",
		// fonts
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		// archives
		".zip", ".gz", ".tar", ".bz2", ".7z", ".rar",
		// documents (binary office)
		".pdf", ".docx", ".xlsx", ".pptx", ".doc", ".xls", ".ppt",
		// media
		".mp3", ".mp4", ".wav", ".avi", ".mov", ".webm", ".m4a", ".flac",
		// compiled / executable
		".exe", ".dll", ".so", ".dylib", ".class", ".jar", ".wasm",
		// db / cache
		".db", ".sqlite", ".sqlite3", ".pyc":
		return true
	}
	return false
}
