package service

import "github.com/kms9/dars/pkg/skillbundle"

// AgentSkillData is the execution bundle shared by the Lightweight claim and
// skill-bundle resolve paths.
type AgentSkillData struct {
	ID          string               `json:"id"`
	Source      string               `json:"source,omitempty"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Hash        string               `json:"hash,omitempty"`
	SizeBytes   int64                `json:"size_bytes,omitempty"`
	Content     string               `json:"content"`
	Files       []AgentSkillFileData `json:"files,omitempty"`
}

type AgentSkillFileData struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type AgentSkillRefData struct {
	ID          string                  `json:"id"`
	Source      string                  `json:"source"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	Hash        string                  `json:"hash"`
	SizeBytes   int64                   `json:"size_bytes"`
	FileCount   int                     `json:"file_count"`
	Files       []AgentSkillFileRefData `json:"files,omitempty"`
}

type AgentSkillFileRefData struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

// BuildAgentSkillBundles normalizes source/ID and computes a stable content
// manifest for both workspace and built-in skills.
func BuildAgentSkillBundles(skills []AgentSkillData) ([]AgentSkillData, []AgentSkillRefData) {
	bundles := make([]AgentSkillData, 0, len(skills))
	refs := make([]AgentSkillRefData, 0, len(skills))
	for _, skill := range skills {
		if skill.Source == "" {
			if skill.ID == "" {
				skill.Source = skillbundle.SourceBuiltin
			} else {
				skill.Source = skillbundle.SourceWorkspace
			}
		}
		if skill.ID == "" && skill.Source == skillbundle.SourceBuiltin {
			skill.ID = "builtin:" + skill.Name
		}

		files := make([]skillbundle.File, 0, len(skill.Files))
		for _, file := range skill.Files {
			files = append(files, skillbundle.File{Path: file.Path, Content: file.Content})
		}
		manifest := skillbundle.BuildManifest(skillbundle.Skill{
			ID: skill.ID, Source: skill.Source, Name: skill.Name,
			Description: skill.Description, Content: skill.Content, Files: files,
		})
		skill.Hash = manifest.Hash
		skill.SizeBytes = manifest.SizeBytes
		fileRefs := make(map[string]skillbundle.FileRef, len(manifest.Files))
		for _, file := range manifest.Files {
			fileRefs[file.Path] = file
		}
		for i := range skill.Files {
			if ref, ok := fileRefs[skill.Files[i].Path]; ok {
				skill.Files[i].SHA256 = ref.SHA256
				skill.Files[i].SizeBytes = ref.SizeBytes
			}
		}
		bundles = append(bundles, skill)

		refFiles := make([]AgentSkillFileRefData, 0, len(manifest.Files))
		for _, file := range manifest.Files {
			refFiles = append(refFiles, AgentSkillFileRefData{Path: file.Path, SHA256: file.SHA256, SizeBytes: file.SizeBytes})
		}
		refs = append(refs, AgentSkillRefData{
			ID: skill.ID, Source: skill.Source, Name: skill.Name, Description: skill.Description,
			Hash: manifest.Hash, SizeBytes: manifest.SizeBytes, FileCount: manifest.FileCount, Files: refFiles,
		})
	}
	return bundles, refs
}
