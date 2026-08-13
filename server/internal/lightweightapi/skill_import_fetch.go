package lightweightapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	skillpkg "github.com/kms9/dars/internal/skill"
	"golang.org/x/sync/errgroup"
)

// ErrImportSourceUnavailable marks a transient failure to read the upstream
// source (e.g. the GitHub tree API rate limiting).
var ErrImportSourceUnavailable = errors.New("import source temporarily unavailable")

var clawHubAPIBase = "https://clawhub.ai/api/v1"

const clawHubSearchStatsLimit = 10

type clawhubSearchResponse struct {
	Results []clawhubSearchResult `json:"results"`
}

type clawhubSearchResult struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"displayName"`
	Summary     string `json:"summary"`
	OwnerHandle string `json:"ownerHandle"`
}

type clawhubSkillStats struct {
	InstallsAllTime int64 `json:"installsAllTime"`
	InstallsCurrent int64 `json:"installsCurrent"`
}

type clawhubGetSkillResponse struct {
	Skill         clawhubSkill          `json:"skill"`
	LatestVersion *clawhubLatestVersion `json:"latestVersion"`
}

type clawhubSkill struct {
	Slug        string            `json:"slug"`
	DisplayName string            `json:"displayName"`
	Summary     string            `json:"summary"`
	Tags        map[string]string `json:"tags"`
	Stats       clawhubSkillStats `json:"stats"`
}

type clawhubLatestVersion struct {
	Version string `json:"version"`
}

type clawhubVersionDetailResponse struct {
	Version clawhubVersionDetail `json:"version"`
}

type clawhubVersionDetail struct {
	Version string             `json:"version"`
	Files   []clawhubFileEntry `json:"files"`
}

type clawhubFileEntry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// --- GitHub types (for skills.sh) ---

type githubContentEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"` // "file" or "dir"
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
}

type githubRepoInfo struct {
	DefaultBranch string `json:"default_branch"`
}

type githubTreeResponse struct {
	Tree      []githubTreeEntry `json:"tree"`
	Truncated bool              `json:"truncated"`
}

type githubTreeEntry struct {
	Path string `json:"path"`
	Type string `json:"type"` // "blob" or "tree"
	Size int64  `json:"size"` // blob byte size (absent/0 for tree entries)
}

// fetchGitHubDefaultBranch returns the default branch of a GitHub repository.
// Falls back to "main" if the API call fails.
func fetchGitHubDefaultBranch(ctx context.Context, httpClient *http.Client, owner, repo string) string {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s",
		url.PathEscape(owner), url.PathEscape(repo))
	resp, err := doGitHubAPIGet(ctx, httpClient, apiURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return "main"
	}
	defer resp.Body.Close()

	var info githubRepoInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.DefaultBranch == "" {
		return "main"
	}
	return info.DefaultBranch
}

// --- URL detection ---

// importSource identifies where a URL points.
type importSource int

const (
	sourceClawHub importSource = iota
	sourceSkillsSh
	sourceGitHub
)

// detectImportSource determines the source from a URL.
// Returns the source and a normalized URL (with scheme).
func detectImportSource(raw string) (importSource, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, "", fmt.Errorf("empty URL")
	}

	normalized := raw
	if !strings.HasPrefix(normalized, "http://") && !strings.HasPrefix(normalized, "https://") {
		normalized = "https://" + normalized
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return 0, "", fmt.Errorf("invalid URL: %w", err)
	}

	host := strings.ToLower(parsed.Hostname())
	switch {
	case host == "skills.sh" || host == "www.skills.sh":
		return sourceSkillsSh, normalized, nil
	case host == "clawhub.ai" || host == "www.clawhub.ai":
		return sourceClawHub, normalized, nil
	case host == "github.com" || host == "www.github.com":
		return sourceGitHub, normalized, nil
	default:
		// If no host (bare slug), default to clawhub
		if !strings.Contains(raw, "/") || !strings.Contains(raw, ".") {
			return sourceClawHub, raw, nil
		}
		return 0, "", fmt.Errorf("unsupported source: %s (supported: clawhub.ai, skills.sh, github.com)", host)
	}
}

// --- ClawHub import ---

// parseClawHubSlug extracts the skill slug from a clawhub.ai URL.
func parseClawHubSlug(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	// /{owner}/{slug} — take the last segment as the slug
	if len(parts) == 2 {
		return parts[1], nil
	}
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], nil
	}
	// Bare slug (no path)
	if raw == parsed.Host || parsed.Path == "" || parsed.Path == "/" {
		return "", fmt.Errorf("missing skill slug in URL")
	}
	return "", fmt.Errorf("could not extract skill slug from URL: %s", raw)
}

func searchClawHubSkills(httpClient *http.Client, query string) ([]skillSearchCandidateResponse, error) {
	searchURL := clawHubAPIBase + "/search?q=" + url.QueryEscape(query)
	resp, err := httpClient.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("failed to reach ClawHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ClawHub search returned status %d", resp.StatusCode)
	}

	var searchResp clawhubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse ClawHub search response")
	}

	candidates := make([]skillSearchCandidateResponse, 0, len(searchResp.Results))
	for i, result := range searchResp.Results {
		if result.Slug == "" {
			continue
		}
		candidate := skillSearchCandidateResponse{
			Name:        result.DisplayName,
			URL:         buildClawHubSkillURL(result.OwnerHandle, result.Slug),
			Source:      "clawhub.ai",
			Description: result.Summary,
		}
		if candidate.Name == "" {
			candidate.Name = result.Slug
		}
		if i < clawHubSearchStatsLimit {
			if count, ok := fetchClawHubInstallCount(httpClient, result.Slug); ok {
				candidate.InstallCount = &count
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func buildClawHubSkillURL(ownerHandle, slug string) string {
	if ownerHandle == "" {
		return "https://clawhub.ai/" + url.PathEscape(slug)
	}
	return "https://clawhub.ai/" + url.PathEscape(ownerHandle) + "/" + url.PathEscape(slug)
}

func fetchClawHubInstallCount(httpClient *http.Client, slug string) (int64, bool) {
	detailURL := clawHubAPIBase + "/skills/" + url.PathEscape(slug)
	resp, err := httpClient.Get(detailURL)
	if err != nil {
		slog.Warn("clawhub search: failed to fetch skill details", "slug", slug, "error", err)
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		slog.Warn("clawhub search: skill details returned non-200", "slug", slug, "status", resp.StatusCode)
		return 0, false
	}
	var detail clawhubGetSkillResponse
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		slog.Warn("clawhub search: failed to parse skill details", "slug", slug, "error", err)
		return 0, false
	}
	if detail.Skill.Stats.InstallsAllTime > 0 {
		return detail.Skill.Stats.InstallsAllTime, true
	}
	return detail.Skill.Stats.InstallsCurrent, true
}

func fetchFromClawHub(ctx context.Context, httpClient *http.Client, rawURL string) (*skillpkg.ImportedSkill, error) {
	slug, err := parseClawHubSlug(rawURL)
	if err != nil {
		return nil, err
	}

	apiBase := clawHubAPIBase

	// 1. Fetch skill metadata
	skillReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/skills/"+url.PathEscape(slug), nil)
	if err != nil {
		return nil, err
	}
	skillResp, err := httpClient.Do(skillReq)
	if err != nil {
		return nil, fmt.Errorf("failed to reach ClawHub: %w", err)
	}
	defer skillResp.Body.Close()

	if skillResp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("skill not found on ClawHub: %s", slug)
	}
	if skillResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ClawHub returned status %d", skillResp.StatusCode)
	}

	var chResp clawhubGetSkillResponse
	if err := json.NewDecoder(skillResp.Body).Decode(&chResp); err != nil {
		return nil, fmt.Errorf("failed to parse ClawHub response")
	}
	chSkill := chResp.Skill

	// 2. Determine latest version and fetch file list
	latestVersion := ""
	if v, ok := chSkill.Tags["latest"]; ok {
		latestVersion = v
	} else if chResp.LatestVersion != nil {
		latestVersion = chResp.LatestVersion.Version
	}

	var filePaths []string
	if latestVersion != "" {
		vURL := fmt.Sprintf("%s/skills/%s/versions/%s", apiBase, url.PathEscape(slug), url.PathEscape(latestVersion))
		vReq, verr := http.NewRequestWithContext(ctx, http.MethodGet, vURL, nil)
		if verr != nil {
			return nil, verr
		}
		vResp, err := httpClient.Do(vReq)
		if err == nil {
			defer vResp.Body.Close()
			if vResp.StatusCode == http.StatusOK {
				var vDetail clawhubVersionDetailResponse
				if err := json.NewDecoder(vResp.Body).Decode(&vDetail); err == nil {
					for _, f := range vDetail.Version.Files {
						filePaths = append(filePaths, f.Path)
					}
				}
			}
		}
	}

	// 3. Download each file
	result := &skillpkg.ImportedSkill{
		Name:        chSkill.DisplayName,
		Description: chSkill.Summary,
		Origin: map[string]any{
			"type":       "clawhub",
			"source_url": rawURL,
			"slug":       slug,
		},
	}
	if result.Name == "" {
		result.Name = slug
	}

	for _, fp := range filePaths {
		fileURL := fmt.Sprintf("%s/skills/%s/file?path=%s", apiBase, url.PathEscape(slug), url.QueryEscape(fp))
		if latestVersion != "" {
			fileURL += "&version=" + url.QueryEscape(latestVersion)
		}
		body, err := fetchRawFile(ctx, httpClient, fileURL)
		if err != nil {
			// Cap violations must abort: silently dropping a file would
			// produce an incomplete bundle that looks valid. SKILL.md is
			// load-bearing, so any failure on it is fatal too.
			if skillpkg.IsCapError(err) || fp == "SKILL.md" {
				return nil, fmt.Errorf("clawhub import: %s: %w", fp, err)
			}
			// A cancelled context (overall deadline / client disconnect) is
			// fatal for the same reason: skipping every remaining file would
			// persist a half-populated bundle as a success.
			if ctx.Err() != nil {
				return nil, fmt.Errorf("clawhub import: fetch aborted at %s: %w", fp, ctx.Err())
			}
			slog.Warn("clawhub import: file download failed", "path", fp, "error", err)
			continue
		}
		if fp == "SKILL.md" {
			result.Content = string(body)
			continue
		}
		if err := result.AddFile(fp, string(body)); err != nil {
			return nil, err
		}
	}

	if result.Content == "" {
		return nil, fmt.Errorf("clawhub import: SKILL.md is empty or missing for %s", slug)
	}

	return result, nil
}

// --- skills.sh import ---

// parseSkillsShParts extracts owner, repo, skill-name from a skills.sh URL.
// URL format: https://skills.sh/{owner}/{repo}/{skill-name}
func parseSkillsShParts(raw string) (owner, repo, skillName string, err error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid URL: %w", err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("expected URL format: skills.sh/{owner}/{repo}/{skill-name}, got: %s", parsed.Path)
	}
	return parts[0], parts[1], parts[2], nil
}

func fetchFromSkillsSh(ctx context.Context, httpClient *http.Client, rawURL string) (*skillpkg.ImportedSkill, error) {
	owner, repo, skillName, err := parseSkillsShParts(rawURL)
	if err != nil {
		return nil, err
	}

	// A skills.sh URL maps onto a GitHub repository. Both skill-directory
	// resolution and supporting-file enumeration are driven by a single
	// recursive tree fetch. This collapses what used to be one contents-API
	// call per directory — hundreds of sequential requests for a large mono-repo
	// like api-gateway-skill, which pushed the request past the reverse-proxy
	// gateway timeout (504) — into one call, and lets the import caps be checked
	// from the tree metadata before any file is downloaded.
	defaultBranch := fetchGitHubDefaultBranch(ctx, httpClient, owner, repo)
	rawPrefix := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s",
		url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(defaultBranch))

	tree, truncated, treeErr := fetchGitHubTree(ctx, httpClient, owner, repo, defaultBranch)
	if treeErr != nil {
		// The tree fetch failed (typically GitHub API rate limiting, which also
		// takes down the contents API). Without the tree we cannot safely
		// resolve which directory is the skill: a raw-probe + root-SKILL.md
		// fallback would re-select the repository root whenever its name
		// collides with the slug (the api-gateway-skill case) and the same
		// rate limiting would then leave the crawl empty — persisting the wrong
		// SKILL.md with zero supporting files as a "successful" import. Fail
		// with a retryable error instead so nothing incorrect is saved.
		slog.Warn("skills.sh import: repository tree fetch failed",
			"owner", owner, "repo", repo, "error", treeErr)
		return nil, fmt.Errorf("%w: could not read the %s/%s repository tree (usually GitHub API rate limiting — set GITHUB_TOKEN on the server or retry): %v",
			ErrImportSourceUnavailable, owner, repo, treeErr)
	}

	skillDir, skillMdBody, err := resolveSkillDirFromTree(ctx, httpClient, owner, repo, defaultBranch, rawPrefix, skillName, tree, truncated)
	if err != nil {
		return nil, err
	}

	result := newSkillsShImportedSkill(skillMdBody, skillName, rawURL, owner, repo)

	if truncated {
		// The tree is incomplete, so it can't drive enumeration; use the legacy
		// per-directory crawl scoped to the resolved skill directory.
		if err := addSupportingFilesViaCrawl(ctx, httpClient, result, owner, repo, defaultBranch, skillDir); err != nil {
			return nil, err
		}
		return result, nil
	}

	if err := addSupportingFilesFromTree(ctx, httpClient, result, tree, rawPrefix, skillDir); err != nil {
		return nil, err
	}
	return result, nil
}

// newSkillsShImportedSkill builds the importedSkill shell (name, description,
// SKILL.md content, provenance) from a fetched SKILL.md body.
func newSkillsShImportedSkill(skillMdBody []byte, skillName, rawURL, owner, repo string) *skillpkg.ImportedSkill {
	name, description := skillpkg.ParseSkillFrontmatter(string(skillMdBody))
	if name == "" {
		name = skillName
	}
	return &skillpkg.ImportedSkill{
		Name:        name,
		Description: description,
		Content:     string(skillMdBody),
		Origin: map[string]any{
			"type":       "skills_sh",
			"source_url": rawURL,
			"owner":      owner,
			"repo":       repo,
			"skill":      skillName,
		},
	}
}

// addSupportingFilesViaCrawl is the legacy per-directory enumeration path, used
// as a fallback only for a truncated recursive tree (and by the github.com
// importer). A tree-fetch failure no longer routes here — it returns a retryable
// error instead. It lists skillDir via the contents API, recurses
// subdirectories, and downloads each file. It stays lenient on a genuine listing
// failure (returns with only SKILL.md, matching prior behavior under GitHub API
// rate limiting) but treats a cancelled context as fatal so a mid-crawl deadline
// or disconnect can't persist an incomplete bundle as a success.
func addSupportingFilesViaCrawl(ctx context.Context, httpClient *http.Client, result *skillpkg.ImportedSkill, owner, repo, ref, skillDir string) error {
	apiURL := buildGitHubContentsURL(owner, repo, skillDir, ref)
	dirResp, err := doGitHubAPIGet(ctx, httpClient, apiURL)
	if err != nil || dirResp.StatusCode != http.StatusOK {
		if dirResp != nil {
			dirResp.Body.Close()
		}
		// A cancelled context (overall deadline / client disconnect) must abort
		// rather than being swallowed as "no supporting files", which would
		// persist an incomplete bundle as a success.
		if ctx.Err() != nil {
			return fmt.Errorf("github import: directory listing aborted: %w", ctx.Err())
		}
		return nil
	}
	defer dirResp.Body.Close()

	var entries []githubContentEntry
	if err := json.NewDecoder(dirResp.Body).Decode(&entries); err != nil {
		// A cancelled context surfaces as a decode error when the deadline or a
		// client disconnect lands while the response body is still being read.
		// Treat it as fatal rather than swallowing it — otherwise the import is
		// saved with a valid SKILL.md but zero supporting files.
		if ctx.Err() != nil {
			return fmt.Errorf("github import: directory listing read aborted: %w", ctx.Err())
		}
		slog.Warn("github import: failed to decode top-level directory listing", "url", apiURL, "error", err)
		return nil
	}

	var allFiles []githubContentEntry
	collectGitHubFiles(ctx, httpClient, entries, &allFiles, apiURL)
	// collectGitHubFiles is lenient on a failed subdirectory listing; if the
	// context was cancelled mid-crawl the collected set is incomplete, so abort.
	if ctx.Err() != nil {
		return fmt.Errorf("github import: directory crawl aborted: %w", ctx.Err())
	}

	basePath := ""
	if skillDir != "" {
		basePath = skillDir + "/"
	}
	for _, entry := range allFiles {
		if entry.DownloadURL == "" {
			continue
		}
		body, err := fetchRawFile(ctx, httpClient, entry.DownloadURL)
		if err != nil {
			if skillpkg.IsCapError(err) {
				return fmt.Errorf("github import: %s: %w", entry.Path, err)
			}
			if ctx.Err() != nil {
				return fmt.Errorf("github import: fetch aborted at %s: %w", entry.Path, ctx.Err())
			}
			slog.Warn("github import: file download failed", "path", entry.Path, "error", err)
			continue
		}
		relPath := strings.TrimPrefix(entry.Path, basePath)
		if err := result.AddFile(relPath, string(body)); err != nil {
			return err
		}
	}
	return nil
}

// fetchGitHubTree fetches the full recursive git tree for a ref in a single API
// call. Each returned blob entry carries its path and byte size, which lets the
// importer both resolve the skill directory and enforce the import caps without
// the per-directory contents crawl that previously issued one API call per
// directory — hundreds of sequential requests for a large mono-repo, which is
// what pushed skills.sh imports past the reverse-proxy gateway timeout (504).
func fetchGitHubTree(ctx context.Context, httpClient *http.Client, owner, repo, ref string) (entries []githubTreeEntry, truncated bool, err error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1",
		url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(ref))
	resp, err := doGitHubAPIGet(ctx, httpClient, apiURL)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var tree githubTreeResponse
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, false, err
	}
	return tree.Tree, tree.Truncated, nil
}

// resolveSkillDirFromTree resolves the skill directory and its SKILL.md body
// from an already-fetched repository tree. It prefers the most specific
// frontmatter-verified match (a SKILL.md whose directory matches the skill
// name), only considering the repository root when nothing deeper matches — so
// a repo-root SKILL.md whose name happens to collide with the skill name no
// longer captures the whole repository. When frontmatter matching finds nothing
// it falls back to accepting a conventional skill location by path (preserving
// the pre-tree importer's lenient semantics). When the tree is truncated it
// falls back to a bounded per-prefix listing.
func resolveSkillDirFromTree(ctx context.Context, httpClient *http.Client, owner, repo, defaultBranch, rawPrefix, skillName string, tree []githubTreeEntry, truncated bool) (string, []byte, error) {
	skillPaths := extractSkillMdPaths(tree)
	preferred, remaining := partitionSkillMdPaths(skillName, skillPaths)
	if dir, body, ok := findMatchingSkillDirByFrontmatter(ctx, httpClient, rawPrefix, skillName, preferred); ok {
		return dir, body, nil
	}
	if !truncated {
		if dir, body, ok := findMatchingSkillDirByFrontmatter(ctx, httpClient, rawPrefix, skillName, remaining); ok {
			return dir, body, nil
		}
		// Frontmatter matching found nothing. Fall back to path-based
		// acceptance so a skill living at a conventional location
		// (skills/<name>/SKILL.md, etc.) still imports even when its
		// frontmatter name doesn't byte-match the URL slug — e.g. `name: Foo`
		// for slug `foo`. This restores the pre-tree importer's semantics and
		// matches what the rate-limited legacy fallback already accepts, using
		// only the tree we already have (at most one extra raw fetch, never a
		// directory crawl). Kept after the frontmatter pass so the bare repo
		// root is never eligible here — the collision fix stays intact.
		if dir, body, ok := acceptConventionalSkillDir(ctx, httpClient, rawPrefix, skillName, skillPaths, true); ok {
			return dir, body, nil
		}
		return "", nil, skillMdNotFoundError(owner, repo, skillName)
	}

	slog.Warn("github import: repository tree listing truncated", "owner", owner, "repo", repo, "branch", defaultBranch)
	if dir, body, ok := findSkillDirFromConventionalPrefixes(ctx, httpClient, owner, repo, defaultBranch, rawPrefix, skillName); ok {
		return dir, body, nil
	}
	// Same path-based acceptance as the untruncated branch, so a conventional
	// skill with a display-name frontmatter imports identically regardless of
	// whether GitHub truncated the tree. The tree can't prove absence here, so
	// the helper probes each candidate directly.
	if dir, body, ok := acceptConventionalSkillDir(ctx, httpClient, rawPrefix, skillName, skillPaths, false); ok {
		return dir, body, nil
	}
	return "", nil, fmt.Errorf("repository %s/%s tree is too large to scan exhaustively for skill %s", owner, repo, skillName)
}

// conventionalSkillMdPaths returns the SKILL.md locations the importer accepts
// by path (without a frontmatter name check) for a given skill slug. Order is
// significant: it mirrors the pre-tree importer's probe order.
func conventionalSkillMdPaths(skillName string) []string {
	return []string{
		"skills/" + skillName + "/SKILL.md",
		".claude/skills/" + skillName + "/SKILL.md",
		"plugin/skills/" + skillName + "/SKILL.md",
		skillName + "/SKILL.md",
	}
}

// acceptConventionalSkillDir accepts a skill by its conventional path
// (skills/<name>/SKILL.md, etc.) without requiring a frontmatter name match,
// restoring the pre-tree importer's lenient path-based acceptance. It never
// accepts the bare repo root, so a root SKILL.md whose name collides with the
// slug can only be selected through the frontmatter pass — the collision fix
// stays intact.
//
// When treeComplete is true, the untruncated tree proves which conventional
// paths exist, so absent candidates are skipped without a network call. When
// the tree was truncated it can't prove absence, so each candidate is probed
// directly (a missing candidate simply 404s and is skipped).
func acceptConventionalSkillDir(ctx context.Context, httpClient *http.Client, rawPrefix, skillName string, treeSkillPaths []string, treeComplete bool) (string, []byte, bool) {
	present := make(map[string]struct{}, len(treeSkillPaths))
	for _, p := range treeSkillPaths {
		present[p] = struct{}{}
	}
	for _, candidate := range conventionalSkillMdPaths(skillName) {
		if treeComplete {
			if _, ok := present[candidate]; !ok {
				continue // a complete tree proves this path is absent
			}
		}
		body, err := fetchRawFile(ctx, httpClient, buildRawGitHubURL(rawPrefix, candidate))
		if err != nil {
			// With a complete tree the path was proven present, so a fetch
			// error is unexpected and worth a breadcrumb; with a truncated tree
			// this is just a probe miss.
			if treeComplete {
				slog.Warn("skills.sh import: conventional SKILL.md fetch failed", "path", candidate, "error", err)
			}
			continue
		}
		return skillDirFromSkillFilePath(candidate), body, true
	}
	return "", nil, false
}

// treeDownloadConcurrency bounds how many supporting files are downloaded in
// parallel during tree-based collection. Small enough that one import cannot
// open hundreds of sockets to GitHub at once, large enough to keep the download
// phase well under the overall import deadline for a full (256-file) bundle.
const treeDownloadConcurrency = 8

// addSupportingFilesFromTree enumerates the supporting files under skillDir from
// a single recursive git tree, enforces the per-file / count / total-byte caps
// arithmetically from the tree metadata BEFORE downloading anything (so an
// over-limit skill fails fast with a clear error instead of timing out), then
// downloads the surviving files concurrently and appends them in a stable path
// order. It replaces the legacy per-directory contents crawl (collectGitHubFiles).
func addSupportingFilesFromTree(ctx context.Context, httpClient *http.Client, result *skillpkg.ImportedSkill, tree []githubTreeEntry, rawPrefix, skillDir string) error {
	basePath := ""
	if skillDir != "" {
		basePath = skillDir + "/"
	}

	// Select the eligible supporting-file blobs under skillDir, mirroring the
	// filters the download loop / addFile would otherwise apply: skip the
	// skill's own SKILL.md, LICENSE files, and binary assets (which addFile
	// drops anyway). Keeping the filter here makes the arithmetic cap check
	// below match what actually gets imported.
	type treeFile struct {
		repoPath string
		relPath  string
		size     int64
	}
	var eligible []treeFile
	for _, entry := range tree {
		if entry.Type != "blob" {
			continue
		}
		if basePath != "" && !strings.HasPrefix(entry.Path, basePath) {
			continue
		}
		relPath := strings.TrimPrefix(entry.Path, basePath)
		if relPath == "" {
			continue
		}
		lowerBase := strings.ToLower(filepath.Base(relPath))
		if lowerBase == "skill.md" || lowerBase == "license" || lowerBase == "license.txt" || lowerBase == "license.md" {
			continue
		}
		if skillpkg.IsLikelyBinaryFilePath(relPath) {
			continue
		}
		eligible = append(eligible, treeFile{repoPath: entry.Path, relPath: relPath, size: entry.size()})
	}

	// Stable order so imports are deterministic regardless of download timing.
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].relPath < eligible[j].relPath })

	// Arithmetic cap pre-check on the tree metadata — no downloads required to
	// reject an over-limit bundle.
	if len(eligible) > skillpkg.MaxImportFileCount {
		return fmt.Errorf("%w: import bundle would contain %d files, exceeding the %d file limit", skillpkg.ErrImportCapExceeded, len(eligible), skillpkg.MaxImportFileCount)
	}
	var totalSize int64
	for _, f := range eligible {
		if f.size > skillpkg.MaxImportFileSize {
			return fmt.Errorf("%w: %s is %d bytes, exceeding the %d byte per-file limit", skillpkg.ErrImportCapExceeded, f.relPath, f.size, skillpkg.MaxImportFileSize)
		}
		totalSize += f.size
	}
	if totalSize > skillpkg.MaxImportTotalSize {
		return fmt.Errorf("%w: import bundle is %d bytes, exceeding the %d byte limit", skillpkg.ErrImportCapExceeded, totalSize, skillpkg.MaxImportTotalSize)
	}

	// Download concurrently, then append in the pre-sorted order.
	contents := make([]string, len(eligible))
	fetched := make([]bool, len(eligible))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(treeDownloadConcurrency)
	for i, f := range eligible {
		i, f := i, f
		g.Go(func() error {
			body, err := fetchRawFile(gctx, httpClient, buildRawGitHubURL(rawPrefix, f.repoPath))
			if err != nil {
				if skillpkg.IsCapError(err) {
					return fmt.Errorf("github import: %s: %w", f.repoPath, err)
				}
				// A cancelled context (overall import deadline exceeded, or the
				// client disconnecting) must abort, not be swallowed as a
				// per-file skip: otherwise every remaining download fails the
				// same way, g.Wait returns nil, and the user gets a bundle that
				// looks complete but is silently missing files. Let it surface
				// so importFetchErrorResponse maps the deadline to a readable
				// 504.
				if gctx.Err() != nil {
					return fmt.Errorf("github import: fetch aborted at %s: %w", f.repoPath, gctx.Err())
				}
				// Otherwise match the legacy loop's leniency: a single failed
				// supporting file is logged and skipped, not fatal.
				slog.Warn("github import: file download failed", "path", f.repoPath, "error", err)
				return nil
			}
			contents[i] = string(body)
			fetched[i] = true
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	for i, f := range eligible {
		if !fetched[i] {
			continue
		}
		if err := result.AddFile(f.relPath, contents[i]); err != nil {
			return err
		}
	}
	return nil
}

// size returns the blob byte size, guarding against a negative value from a
// malformed tree response.
func (e githubTreeEntry) size() int64 {
	if e.Size < 0 {
		return 0
	}
	return e.Size
}

// collectGitHubFiles recursively collects file entries from a GitHub directory listing.
func collectGitHubFiles(ctx context.Context, httpClient *http.Client, entries []githubContentEntry, out *[]githubContentEntry, parentURL string) {
	for _, entry := range entries {
		// Stop descending once the context is cancelled; the caller checks
		// ctx.Err() afterwards and aborts rather than importing a partial crawl.
		if ctx.Err() != nil {
			return
		}
		lower := strings.ToLower(entry.Name)
		if lower == "skill.md" || lower == "license" || lower == "license.txt" || lower == "license.md" {
			continue
		}
		if entry.Type == "file" {
			*out = append(*out, entry)
		} else if entry.Type == "dir" {
			// Fetch subdirectory contents
			subURL := entry.URL
			if subURL == "" {
				parsed, err := url.Parse(parentURL)
				if err != nil {
					slog.Warn("github import: invalid parent directory url", "url", parentURL, "error", err)
					continue
				}
				parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/" + entry.Name
				subURL = parsed.String()
			}
			subResp, err := doGitHubAPIGet(ctx, httpClient, subURL)
			if err != nil || subResp.StatusCode != http.StatusOK {
				attrs := []any{"url", subURL}
				if subResp != nil {
					attrs = append(attrs, "status", subResp.StatusCode)
					subResp.Body.Close()
				}
				if err != nil {
					attrs = append(attrs, "error", err)
				}
				slog.Warn("github import: failed to list subdirectory", attrs...)
				continue
			}
			var subEntries []githubContentEntry
			if err := json.NewDecoder(subResp.Body).Decode(&subEntries); err != nil {
				subResp.Body.Close()
				slog.Warn("github import: failed to decode subdirectory listing", "url", subURL, "error", err)
				continue
			}
			subResp.Body.Close()
			collectGitHubFiles(ctx, httpClient, subEntries, out, subURL)
		}
	}
}

func findSkillDirFromConventionalPrefixes(ctx context.Context, httpClient *http.Client, owner, repo, defaultBranch, rawPrefix, skillName string) (string, []byte, bool) {
	prefixes := []string{"skills", ".claude/skills", "plugin/skills"}
	var skillPaths []string
	for _, prefix := range prefixes {
		paths, err := listGitHubSkillMdPaths(ctx, httpClient, owner, repo, prefix, defaultBranch)
		if err != nil {
			slog.Warn("github import: failed to list conventional skill prefix", "prefix", prefix, "error", err)
			continue
		}
		skillPaths = append(skillPaths, paths...)
	}

	preferred, remaining := partitionSkillMdPaths(skillName, skillPaths)
	if dir, body, ok := findMatchingSkillDirByFrontmatter(ctx, httpClient, rawPrefix, skillName, preferred); ok {
		return dir, body, true
	}
	return findMatchingSkillDirByFrontmatter(ctx, httpClient, rawPrefix, skillName, remaining)
}

func listGitHubSkillMdPaths(ctx context.Context, httpClient *http.Client, owner, repo, repoPath, ref string) ([]string, error) {
	apiURL := buildGitHubContentsURL(owner, repo, repoPath, ref)
	resp, err := doGitHubAPIGet(ctx, httpClient, apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var entries []githubContentEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}

	var paths []string
	collectGitHubSkillMdPaths(ctx, httpClient, entries, &paths, apiURL)
	return paths, nil
}

func collectGitHubSkillMdPaths(ctx context.Context, httpClient *http.Client, entries []githubContentEntry, out *[]string, parentURL string) {
	for _, entry := range entries {
		lower := strings.ToLower(entry.Name)
		if entry.Type == "file" {
			if lower == "skill.md" {
				*out = append(*out, entry.Path)
			}
			continue
		}
		if entry.Type != "dir" {
			continue
		}

		subURL := entry.URL
		if subURL == "" {
			parsed, err := url.Parse(parentURL)
			if err != nil {
				slog.Warn("github import: invalid parent directory url", "url", parentURL, "error", err)
				continue
			}
			parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/" + entry.Name
			subURL = parsed.String()
		}

		subResp, err := doGitHubAPIGet(ctx, httpClient, subURL)
		if err != nil || subResp.StatusCode != http.StatusOK {
			attrs := []any{"url", subURL}
			if subResp != nil {
				attrs = append(attrs, "status", subResp.StatusCode)
				subResp.Body.Close()
			}
			if err != nil {
				attrs = append(attrs, "error", err)
			}
			slog.Warn("github import: failed to list skill metadata subdirectory", attrs...)
			continue
		}

		var subEntries []githubContentEntry
		if err := json.NewDecoder(subResp.Body).Decode(&subEntries); err != nil {
			subResp.Body.Close()
			slog.Warn("github import: failed to decode skill metadata subdirectory", "url", subURL, "error", err)
			continue
		}
		subResp.Body.Close()
		collectGitHubSkillMdPaths(ctx, httpClient, subEntries, out, subURL)
	}
}

func extractSkillMdPaths(entries []githubTreeEntry) []string {
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type != "blob" || (!strings.HasSuffix(entry.Path, "/SKILL.md") && entry.Path != "SKILL.md") {
			continue
		}
		paths = append(paths, entry.Path)
	}
	return paths
}

func partitionSkillMdPaths(skillName string, skillPaths []string) (preferred []string, remaining []string) {
	for _, skillPath := range skillPaths {
		if isLikelySkillPathMatch(skillName, skillPath) {
			preferred = append(preferred, skillPath)
			continue
		}
		remaining = append(remaining, skillPath)
	}
	return preferred, remaining
}

func findMatchingSkillDirByFrontmatter(ctx context.Context, httpClient *http.Client, rawPrefix, skillName string, skillPaths []string) (string, []byte, bool) {
	for _, skillPath := range skillPaths {
		body, err := fetchRawFile(ctx, httpClient, buildRawGitHubURL(rawPrefix, skillPath))
		if err != nil {
			slog.Warn("github import: fallback SKILL.md fetch failed", "path", skillPath, "error", err)
			continue
		}
		name, _ := skillpkg.ParseSkillFrontmatter(string(body))
		if name == skillName {
			return skillDirFromSkillFilePath(skillPath), body, true
		}
	}
	return "", nil, false
}

func isLikelySkillPathMatch(skillName, skillPath string) bool {
	dir := strings.ToLower(skillDirFromSkillFilePath(skillPath))
	base := strings.ToLower(filepath.Base(dir))
	for _, hint := range skillNameHints(skillName) {
		if strings.Contains(dir, hint) || strings.Contains(base, hint) || strings.Contains(hint, base) {
			return true
		}
	}
	return false
}

func skillNameHints(skillName string) []string {
	skillName = strings.ToLower(skillName)
	parts := strings.Split(skillName, "-")
	seen := map[string]struct{}{}
	var hints []string

	addHint := func(value string) {
		value = strings.TrimSpace(value)
		if len(value) < 3 {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		hints = append(hints, value)
	}

	addHint(skillName)
	for i := 1; i < len(parts); i++ {
		addHint(strings.Join(parts[i:], "-"))
	}
	for _, part := range parts {
		addHint(part)
	}
	return hints
}

// --- GitHub import ---

// errGitHubAPIBlocked signals that an api.github.com probe was rejected for
// auth/rate-limit reasons (401/403/429) rather than because the resource
// genuinely does not exist. Resolvers treat this as "indeterminate" and may
// fall back to the optimistic URL split rather than aborting the import.
var errGitHubAPIBlocked = errors.New("github API blocked (rate limit or auth)")

// doGitHubAPIGet performs a GET against an api.github.com URL, attaching the
// GITHUB_TOKEN bearer header when the env var is set. Unauthenticated GitHub
// API requests are capped at 60/hour per IP, which is trivially exhausted on
// shared self-hosted servers and surfaces to users as 403 errors during
// skill imports. Setting GITHUB_TOKEN raises the limit to 5000/hour.
func doGitHubAPIGet(ctx context.Context, httpClient *http.Client, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	addGitHubAuthHeader(req)
	return httpClient.Do(req)
}

func addGitHubAuthHeader(req *http.Request) {
	if req == nil {
		return
	}
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// githubSpec captures the parsed components of a github.com URL pointing at a
// skill (or single-skill repository).
type githubSpec struct {
	owner    string
	repo     string
	ref      string // empty → caller resolves the default branch
	skillDir string // relative directory within the repo, "" for the repository root

	// refSegments holds the raw path segments after /tree/ or /blob/ that
	// jointly encode (ref, skillDir). GitHub's web URLs do not delimit the
	// boundary between branch/tag name and in-repo path, so when a ref
	// contains '/' (e.g. "release/v2") segments[0] alone is not the ref.
	// fetchFromGitHub uses resolveGitHubRefAndPath to walk these segments
	// and ask the API which prefix is a real branch/tag/commit. When this
	// slice is empty, ref/skillDir above are authoritative (root URL).
	refSegments []string
	// kind is "tree" or "blob"; "" for root URLs. blob requires the last
	// segment to be SKILL.md, which is already stripped from refSegments.
	kind string
}

// parseGitHubURL extracts the owner, repo, and the raw post-/tree|/blob
// segments from a github.com URL. Supported forms:
//
//	github.com/{owner}/{repo}                                → root, default branch
//	github.com/{owner}/{repo}/tree/{ref}/{path...}           → ref / skill dir
//	github.com/{owner}/{repo}/blob/{ref}/{path.../SKILL.md}  → ref / skill dir
//
// A simple-ref shortcut (segments[0] is the ref, the rest is the path) is
// stored in spec.ref/spec.skillDir; refSegments is also populated so that
// fetchFromGitHub can disambiguate refs containing '/' against the API.
func parseGitHubURL(raw string) (githubSpec, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return githubSpec{}, fmt.Errorf("invalid URL: %w", err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return githubSpec{}, fmt.Errorf("expected URL format: github.com/{owner}/{repo}[/tree/{ref}/{path}], got: %s", parsed.Path)
	}
	spec := githubSpec{owner: parts[0], repo: strings.TrimSuffix(parts[1], ".git")}
	if len(parts) == 2 {
		return spec, nil
	}
	kind := parts[2]
	if kind != "tree" && kind != "blob" {
		return githubSpec{}, fmt.Errorf("unsupported URL form: github.com/%s/%s/%s/... (use /tree/{ref}/... or /blob/{ref}/.../SKILL.md)", spec.owner, spec.repo, kind)
	}
	if len(parts) < 4 || parts[3] == "" {
		return githubSpec{}, fmt.Errorf("missing ref after /%s/", kind)
	}
	spec.kind = kind
	rest := parts[3:]
	if kind == "blob" {
		if !strings.EqualFold(rest[len(rest)-1], "SKILL.md") {
			return githubSpec{}, fmt.Errorf("blob URL must point to a SKILL.md file")
		}
		rest = rest[:len(rest)-1]
		if len(rest) == 0 {
			return githubSpec{}, fmt.Errorf("missing ref after /blob/")
		}
	}
	// Decode URL-escaped segments (e.g. spaces) so paths match the repo's
	// real on-disk layout. Re-escaping happens in buildRawGitHubURL.
	decoded := make([]string, len(rest))
	for i, p := range rest {
		d, err := url.PathUnescape(p)
		if err != nil {
			return githubSpec{}, fmt.Errorf("invalid path segment %q: %w", p, err)
		}
		if d == "" {
			return githubSpec{}, fmt.Errorf("empty path segment in URL")
		}
		decoded[i] = d
	}
	spec.refSegments = decoded
	// Optimistic split: assume the simple case where the ref is one segment.
	// fetchFromGitHub will re-resolve via the API and overwrite both fields
	// when the optimistic guess does not validate (e.g. release/v2 refs).
	spec.ref = decoded[0]
	if len(decoded) > 1 {
		spec.skillDir = strings.Join(decoded[1:], "/")
	}
	return spec, nil
}

// resolveGitHubRefAndPath walks the parsed refSegments and asks the GitHub
// commits API which prefix corresponds to a real branch, tag, or commit.
// This is what makes refs containing '/' (e.g. "release/v2") work correctly:
// the URL github.com/o/r/tree/release/v2/skills/foo is ambiguous between
// (ref=release, path=v2/skills/foo) and (ref=release/v2, path=skills/foo),
// so we probe /repos/{o}/{r}/commits/{candidate} from longest to shortest
// and accept the first one the server confirms exists.
//
// On success spec.ref and spec.skillDir are overwritten with the resolved
// pair. On failure (no candidate resolves) a single error is returned that
// names every candidate that was tried.
func resolveGitHubRefAndPath(ctx context.Context, httpClient *http.Client, spec *githubSpec) error {
	if len(spec.refSegments) == 0 {
		return nil
	}
	// Try longest prefix first so that release/v2 wins over release.
	tried := make([]string, 0, len(spec.refSegments))
	blocked := false
	for n := len(spec.refSegments); n >= 1; n-- {
		candidate := strings.Join(spec.refSegments[:n], "/")
		tried = append(tried, candidate)
		ok, err := githubRefExists(ctx, httpClient, spec.owner, spec.repo, candidate)
		if errors.Is(err, errGitHubAPIBlocked) {
			// 401/403/429 means we can't tell whether the ref exists. Keep
			// trying the remaining (shorter) candidates so we don't punish
			// the common single-segment-ref case for one bad probe.
			blocked = true
			continue
		}
		if err != nil {
			// Network / transport errors should not be silently treated as
			// "ref does not exist" — surface them so the caller can retry.
			return fmt.Errorf("validating ref %q: %w", candidate, err)
		}
		if ok {
			spec.ref = candidate
			if n == len(spec.refSegments) {
				spec.skillDir = ""
			} else {
				spec.skillDir = strings.Join(spec.refSegments[n:], "/")
			}
			return nil
		}
	}
	if blocked {
		// Every probe was either a confirmed 404 or rate-limited and we never
		// got a confirmation. Fall back to the optimistic single-segment
		// split that parseGitHubURL populated. If that's wrong, the
		// subsequent raw-file fetch will surface a clearer "SKILL.md not
		// found" error than failing the whole import on a 403.
		slog.Warn("github import: ref resolution blocked by GitHub API (rate limit or auth); falling back to optimistic single-segment ref. Set GITHUB_TOKEN to enable disambiguation of slash-bearing refs.",
			"owner", spec.owner, "repo", spec.repo, "tried", tried)
		return nil
	}
	return fmt.Errorf("could not resolve ref in github.com/%s/%s URL — tried: %s. Make sure the branch, tag, or commit exists and that the URL is the canonical /tree/{ref}/{path} or /blob/{ref}/{path}/SKILL.md form",
		spec.owner, spec.repo, strings.Join(tried, ", "))
}

// githubRefExists returns true when GitHub recognizes ref as a branch, tag,
// or commit SHA on owner/repo. It uses the commits endpoint because that
// single call accepts all three ref kinds (unlike /branches or /tags which
// only match one). 404 means the ref does not exist; any other non-200
// status is treated as an error so the caller can distinguish "missing"
// from "API down".
func githubRefExists(ctx context.Context, httpClient *http.Client, owner, repo, ref string) (bool, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s",
		url.PathEscape(owner), url.PathEscape(repo), escapeRefPath(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return false, err
	}
	// Per GitHub docs: Accept: application/vnd.github.v3.sha returns just
	// the SHA when the ref resolves, which is the cheapest possible probe.
	req.Header.Set("Accept", "application/vnd.github.v3.sha")
	addGitHubAuthHeader(req)
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound, http.StatusUnprocessableEntity:
		return false, nil
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return false, errGitHubAPIBlocked
	default:
		return false, fmt.Errorf("github API returned status %d for ref %q", resp.StatusCode, ref)
	}
}

func fetchFromGitHub(ctx context.Context, httpClient *http.Client, rawURL string) (*skillpkg.ImportedSkill, error) {
	spec, err := parseGitHubURL(rawURL)
	if err != nil {
		return nil, err
	}
	if len(spec.refSegments) > 0 {
		// Disambiguate slash-bearing refs (release/v2 etc.) against the API
		// before issuing any raw or contents requests.
		if err := resolveGitHubRefAndPath(ctx, httpClient, &spec); err != nil {
			return nil, err
		}
	}
	if spec.ref == "" {
		spec.ref = fetchGitHubDefaultBranch(ctx, httpClient, spec.owner, spec.repo)
	}
	rawPrefix := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s",
		url.PathEscape(spec.owner), url.PathEscape(spec.repo), escapeRefPath(spec.ref))

	skillMdPath := "SKILL.md"
	if spec.skillDir != "" {
		skillMdPath = spec.skillDir + "/SKILL.md"
	}
	skillMdBody, err := fetchRawFile(ctx, httpClient, buildRawGitHubURL(rawPrefix, skillMdPath))
	if err != nil {
		if spec.skillDir == "" {
			return nil, fmt.Errorf("SKILL.md not found at the root of %s/%s@%s. For multi-skill repositories, point to a specific directory using github.com/%s/%s/tree/%s/<skill-dir>",
				spec.owner, spec.repo, spec.ref, spec.owner, spec.repo, spec.ref)
		}
		return nil, fmt.Errorf("SKILL.md not found at %s in %s/%s@%s: %w",
			skillMdPath, spec.owner, spec.repo, spec.ref, err)
	}

	name, description := skillpkg.ParseSkillFrontmatter(string(skillMdBody))
	if name == "" {
		if spec.skillDir != "" {
			name = filepath.Base(spec.skillDir)
		} else {
			name = spec.repo
		}
	}

	result := &skillpkg.ImportedSkill{
		Name:        name,
		Description: description,
		Content:     string(skillMdBody),
		Origin: map[string]any{
			"type":       "github",
			"source_url": rawURL,
			"owner":      spec.owner,
			"repo":       spec.repo,
			"ref":        spec.ref,
			"path":       spec.skillDir,
		},
	}

	// Enumerate supporting files from a single recursive tree, checking the
	// import caps against the tree metadata before downloading anything. Fall
	// back to the per-directory contents crawl when the tree is unavailable or
	// truncated (kept lenient so a rate-limited listing doesn't fail an import
	// that already produced a valid SKILL.md).
	tree, truncated, treeErr := fetchGitHubTree(ctx, httpClient, spec.owner, spec.repo, spec.ref)
	if treeErr == nil && !truncated {
		if err := addSupportingFilesFromTree(ctx, httpClient, result, tree, rawPrefix, spec.skillDir); err != nil {
			return nil, err
		}
		return result, nil
	}
	if err := addSupportingFilesViaCrawl(ctx, httpClient, result, spec.owner, spec.repo, spec.ref, spec.skillDir); err != nil {
		return nil, err
	}
	return result, nil
}

// --- Shared helpers ---

// rawGitHubContentHost serves raw GitHub file content. fetchRawFile attaches the
// GITHUB_TOKEN only for this host: the same function downloads files from
// non-GitHub skill sources (clawhub.ai, skills.sh), and an unconditional auth
// header would leak the token to those third-party hosts.
const rawGitHubContentHost = "raw.githubusercontent.com"

// fetchRawFile downloads a URL and returns the body bytes. Returns an error
// if the response exceeds skillpkg.MaxImportFileSize so we never silently truncate a
// half-downloaded skill file into the workspace.
func fetchRawFile(ctx context.Context, httpClient *http.Client, fileURL string) ([]byte, error) {
	req, err := newRawFileRequest(ctx, fileURL)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, skillpkg.MaxImportFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(body) > skillpkg.MaxImportFileSize {
		return nil, fmt.Errorf("%w: file exceeds %d byte limit", skillpkg.ErrImportCapExceeded, skillpkg.MaxImportFileSize)
	}
	return body, nil
}

// newRawFileRequest builds the GET request for a raw skill file, attaching the
// GitHub auth header only when the URL targets GitHub's raw content host. The
// host gate lives here, separate from the round-trip, so it can be unit tested
// without a live network call — GITHUB_TOKEN must never reach a non-GitHub
// skill host.
func newRawFileRequest(ctx context.Context, fileURL string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(req.URL.Hostname(), rawGitHubContentHost) {
		addGitHubAuthHeader(req)
	}
	return req, nil
}

// escapeRefPath percent-encodes each segment of a git ref individually so
// that slash-bearing refs like "release/v2" are sent to GitHub as
// "release/v2" (path separators preserved) rather than "release%2Fv2"
// (which GitHub does not accept on the commits / raw endpoints).
func escapeRefPath(ref string) string {
	parts := strings.Split(ref, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func buildRawGitHubURL(rawPrefix, repoPath string) string {
	parts := strings.Split(strings.Trim(repoPath, "/"), "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	if len(escaped) == 0 {
		return rawPrefix
	}
	return rawPrefix + "/" + strings.Join(escaped, "/")
}

func buildGitHubContentsURL(owner, repo, repoPath, ref string) string {
	base := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents",
		url.PathEscape(owner), url.PathEscape(repo))
	if repoPath == "" {
		return base + "?ref=" + url.QueryEscape(ref)
	}
	return base + "/" + strings.TrimPrefix(buildRawGitHubURL("", repoPath), "/") + "?ref=" + url.QueryEscape(ref)
}

func skillDirFromSkillFilePath(path string) string {
	if path == "SKILL.md" {
		return ""
	}
	return strings.TrimSuffix(path, "/SKILL.md")
}

func skillMdNotFoundError(owner, repo, skillName string) error {
	return fmt.Errorf("SKILL.md not found in repository %s/%s for skill %s", owner, repo, skillName)
}
