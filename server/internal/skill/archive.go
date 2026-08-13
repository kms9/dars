package skill

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// ParseSkillArchive decompresses an uploaded skill archive (.skill / .zip) into
// an ImportedSkill. A .skill file is a standard zip whose entries sit either at
// the archive root (SKILL.md, scripts/...) or nested under a single top-level
// directory (my-skill/SKILL.md, my-skill/scripts/...) — the layout produced by
// Anthropic's package_skill. Both are accepted by rooting on the shallowest
// SKILL.md found.
//
// Safety: every entry is validated against traversal / absolute paths
// (zip-slip), the reserved SKILL.md supporting path is dropped, per-file size is
// bounded while reading (so a lying zip header can't blow up memory), and
// AddFile enforces the per-bundle byte and file-count caps.
func ParseSkillArchive(data []byte, filename string) (*ImportedSkill, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("uploaded file is not a valid .skill/.zip archive")
	}

	// Locate the skill root: the directory of the shallowest SKILL.md.
	var skillMd *zip.File
	rootPrefix := ""
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		clean := path.Clean(f.Name)
		if !strings.EqualFold(path.Base(clean), ContentFilename) {
			continue
		}
		if !ValidateFilePath(clean) {
			continue
		}
		prefix := archiveEntryPrefix(clean)
		if skillMd == nil || len(prefix) < len(rootPrefix) {
			skillMd = f
			rootPrefix = prefix
		}
	}
	if skillMd == nil {
		return nil, fmt.Errorf("archive does not contain a SKILL.md")
	}

	content, err := readZipFile(skillMd, MaxImportFileSize)
	if err != nil {
		return nil, fmt.Errorf("read SKILL.md: %w", err)
	}

	name, description := ParseSkillFrontmatter(content)
	if name == "" {
		name = skillNameFromArchive(rootPrefix, filename)
	}
	if name == "" {
		return nil, fmt.Errorf("could not determine the skill name: SKILL.md has no name field and the archive is unnamed")
	}

	imported := &ImportedSkill{
		Name:        name,
		Description: description,
		Content:     content,
	}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		clean := path.Clean(f.Name)
		// Only files under the resolved skill root belong to this skill.
		if rootPrefix != "" && !strings.HasPrefix(clean, rootPrefix) {
			continue
		}
		rel := strings.TrimPrefix(clean, rootPrefix)
		if rel == "" {
			continue
		}
		// A SKILL.md at any depth is never a supporting file.
		if strings.EqualFold(path.Base(rel), ContentFilename) {
			continue
		}
		if isIgnoredArchiveEntry(rel) {
			continue
		}
		// zip-slip / absolute-path guard.
		if !ValidateFilePath(rel) {
			continue
		}
		fileContent, ferr := readZipFile(f, MaxImportFileSize)
		if ferr != nil {
			// An oversize or unreadable individual asset is skipped rather than
			// failing the whole import, matching the local-runtime importer.
			continue
		}
		if err := imported.AddFile(rel, fileContent); err != nil {
			return nil, err
		}
	}

	sort.Slice(imported.Files, func(i, j int) bool {
		return imported.Files[i].Path < imported.Files[j].Path
	})
	return imported, nil
}

func archiveEntryPrefix(cleanName string) string {
	dir := path.Dir(cleanName)
	if dir == "." || dir == "/" {
		return ""
	}
	return dir + "/"
}

func skillNameFromArchive(rootPrefix, filename string) string {
	if rootPrefix != "" {
		base := path.Base(strings.TrimSuffix(rootPrefix, "/"))
		if base != "." && base != "/" && base != ".." {
			return base
		}
	}
	clean := strings.ReplaceAll(filename, "\\", "/")
	base := path.Base(clean)
	if ext := path.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return strings.TrimSpace(base)
}

func isIgnoredArchiveEntry(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "__MACOSX" || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	switch strings.ToLower(path.Base(rel)) {
	case "license", "license.md", "license.txt":
		return true
	}
	return false
}

func readZipFile(f *zip.File, maxSize int64) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(io.LimitReader(rc, maxSize+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxSize {
		return "", fmt.Errorf("file %q exceeds %d bytes", f.Name, maxSize)
	}
	return string(data), nil
}
