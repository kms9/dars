package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
	"github.com/kms9/dars/internal/daemon/execenv"
	"github.com/kms9/dars/internal/util"
)

var (
	issueCmd     = &cobra.Command{Use: "issue", Short: "Work with Lightweight runs"}
	agentCmd     = &cobra.Command{Use: "agent", Short: "Manage Lightweight agents"}
	workspaceCmd = &cobra.Command{Use: "workspace", Short: "Work with Lightweight workspaces"}
	skillCmd     = &cobra.Command{Use: "skill", Short: "Manage Lightweight skills"}
	squadCmd     = &cobra.Command{Use: "squad", Short: "Manage agent-only squads"}
	chatCmd      = &cobra.Command{Use: "chat", Short: "Work with direct agent chat"}
	runtimeCmd   = &cobra.Command{Use: "runtime", Short: "Manage local runtimes and profiles"}
	userCmd      = &cobra.Command{Use: "user", Short: "View or update the current user"}
)

func init() {
	issueCmd.AddCommand(newLightweightIssueCommands()...)
	agentCmd.AddCommand(newLightweightAgentCommands()...)
	workspaceCmd.AddCommand(newLightweightWorkspaceCommands()...)
	skillCmd.AddCommand(newLightweightSkillCommands()...)
	squadCmd.AddCommand(newLightweightSquadCommands()...)
	chatCmd.AddCommand(newLightweightChatCommands()...)
	runtimeCmd.AddCommand(newLightweightRuntimeCommands()...)
	userCmd.AddCommand(newLightweightUserCommands()...)
}

func resolveProfile(cmd *cobra.Command) string {
	value, _ := cmd.Flags().GetString("profile")
	return value
}

func tryResolveServerURL(cmd *cobra.Command) string {
	if value := cli.FlagOrEnv(cmd, "server-url", "DARS_SERVER_URL", ""); value != "" {
		return strings.TrimRight(value, "/")
	}
	cfg, err := cli.LoadCLIConfigForProfile(resolveProfile(cmd))
	if err == nil {
		return strings.TrimRight(cfg.ServerURL, "/")
	}
	return ""
}

func resolveServerURL(cmd *cobra.Command) string {
	if value := tryResolveServerURL(cmd); value != "" {
		return value
	}
	return ""
}

func resolveLoginTokenServerURL(cmd *cobra.Command) string {
	return resolveServerURL(cmd)
}

func inAgentExecutionContext() bool {
	return os.Getenv("DARS_AGENT_ID") != "" || os.Getenv("DARS_TASK_ID") != ""
}

func daemonTaskContextMarkerPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		markerPath := filepath.Join(dir, execenv.TaskContextMarkerRelPath)
		if data, err := os.ReadFile(markerPath); err == nil {
			var marker struct {
				ManagedBy string `json:"managed_by"`
			}
			if json.Unmarshal(data, &marker) == nil && marker.ManagedBy == execenv.TaskContextMarkerManagedBy {
				return markerPath
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func inDaemonManagedExecutionContext() bool {
	return inAgentExecutionContext() || os.Getenv("DARS_DAEMON_PORT") != "" || daemonTaskContextMarkerPath() != ""
}

func resolveToken(cmd *cobra.Command) string {
	if value := strings.TrimSpace(os.Getenv("DARS_TOKEN")); value != "" {
		return value
	}
	if inDaemonManagedExecutionContext() {
		return ""
	}
	cfg, _ := cli.LoadCLIConfigForProfile(resolveProfile(cmd))
	return cfg.Token
}

func resolveWorkspaceID(cmd *cobra.Command) string {
	if value := cli.FlagOrEnv(cmd, "workspace-id", "DARS_WORKSPACE_ID", ""); value != "" {
		return value
	}
	if inDaemonManagedExecutionContext() {
		return ""
	}
	cfg, _ := cli.LoadCLIConfigForProfile(resolveProfile(cmd))
	return cfg.WorkspaceID
}

func requireWorkspaceID(cmd *cobra.Command) (string, error) {
	id := resolveWorkspaceID(cmd)
	if id != "" {
		return id, nil
	}
	if inDaemonManagedExecutionContext() {
		return "", fmt.Errorf("workspace_id is required: DARS_WORKSPACE_ID must be set by the daemon in agent execution context")
	}
	return "", fmt.Errorf("workspace_id is required: use --workspace-id, DARS_WORKSPACE_ID, or 'dars workspace switch <workspace>'")
}

func newAPIClient(cmd *cobra.Command) (*cli.APIClient, error) {
	serverURL := resolveServerURL(cmd)
	if serverURL == "" {
		return nil, fmt.Errorf("server URL not set: use --server-url, DARS_SERVER_URL, or 'dars config set server_url <url>'")
	}
	token := resolveToken(cmd)
	if inDaemonManagedExecutionContext() && !strings.HasPrefix(token, "dat_") {
		return nil, fmt.Errorf("agent execution context requires DARS_TOKEN to be a task-scoped dat_ token")
	}
	return cli.NewAPIClient(serverURL, resolveWorkspaceID(cmd), token), nil
}

func resolveTextFlag(cmd *cobra.Command, flagName string) (string, bool, error) {
	stdinFlag := flagName + "-stdin"
	fileFlag := flagName + "-file"
	useStdin, _ := cmd.Flags().GetBool(stdinFlag)
	inline, _ := cmd.Flags().GetString(flagName)
	filePath, _ := cmd.Flags().GetString(fileFlag)

	sources := 0
	for _, set := range []bool{useStdin, inline != "", filePath != ""} {
		if set {
			sources++
		}
	}
	if sources > 1 {
		return "", false, fmt.Errorf("--%s, --%s, and --%s are mutually exclusive", flagName, stdinFlag, fileFlag)
	}
	if useStdin {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return "", false, fmt.Errorf("read stdin for --%s: %w", stdinFlag, err)
		}
		body := strings.TrimSuffix(string(data), "\n")
		if body == "" {
			return "", false, fmt.Errorf("stdin content for --%s is empty", stdinFlag)
		}
		return body, true, nil
	}
	if filePath != "" {
		if err := ensureFileFlagWithinWorkdir(cmd, fileFlag, flagName, filePath); err != nil {
			return "", false, err
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", false, fmt.Errorf("read file for --%s: %w", fileFlag, err)
		}
		body := strings.TrimSuffix(string(data), "\n")
		if body == "" {
			return "", false, fmt.Errorf("file content for --%s is empty", fileFlag)
		}
		return body, true, nil
	}
	if inline == "" {
		return "", false, nil
	}
	return util.UnescapeBackslashEscapes(inline), true, nil
}

func ensureFileFlagWithinWorkdir(cmd *cobra.Command, fileFlag, flagName, filePath string) error {
	if allow, _ := cmd.Flags().GetBool("allow-external-file"); allow {
		return nil
	}
	within, err := fileWithinWorkingDir(filePath)
	if err != nil {
		return fmt.Errorf("resolve --%s path %q: %w", fileFlag, filePath, err)
	}
	if !within {
		return fmt.Errorf("--%s path %q resolves outside the current working directory; use a task-workdir file such as ./%s.md or pass --allow-external-file", fileFlag, filePath, flagName)
	}
	return nil
}

func fileWithinWorkingDir(filePath string) (bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, err
	}
	base := cwd
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		base = resolved
	}
	abs := filePath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, abs)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	} else if resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(abs)); parentErr == nil {
		abs = filepath.Join(resolvedParent, filepath.Base(abs))
	} else {
		abs = filepath.Clean(abs)
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), nil
}

func validateIssueStatus(status string) error {
	switch status {
	case "backlog", "todo", "in_progress", "in_review", "done", "blocked", "cancelled":
		return nil
	default:
		return fmt.Errorf("invalid status %q: use backlog, todo, in_progress, in_review, done, blocked, or cancelled", status)
	}
}

type workspaceSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func fetchWorkspaces(ctx context.Context, cmd *cobra.Command) ([]workspaceSummary, error) {
	client, err := newAPIClient(cmd)
	if err != nil {
		return nil, err
	}
	client.WorkspaceID = ""
	var workspaces []workspaceSummary
	if err := client.GetJSON(ctx, "/api/workspaces", &workspaces); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return workspaces, nil
}

var uuidRegexp = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func compactUUID(value string) string {
	return strings.ToLower(strings.ReplaceAll(value, "-", ""))
}

func resolveWorkspaceByIDOrSlug(workspaces []workspaceSummary, target string) (workspaceSummary, error) {
	target = strings.TrimSpace(target)
	for _, workspace := range workspaces {
		if strings.EqualFold(workspace.ID, target) || workspace.Slug == target {
			return workspace, nil
		}
	}
	prefix := compactUUID(target)
	if len(prefix) >= 4 {
		matches := make([]workspaceSummary, 0, 1)
		for _, workspace := range workspaces {
			if strings.HasPrefix(compactUUID(workspace.ID), prefix) {
				matches = append(matches, workspace)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
		if len(matches) > 1 {
			return workspaceSummary{}, fmt.Errorf("workspace prefix %q is ambiguous; use the slug or full UUID", target)
		}
	}
	return workspaceSummary{}, fmt.Errorf("workspace %q not found or inaccessible", target)
}

func resolveWorkspaceRef(ctx context.Context, cmd *cobra.Command, input string) (workspaceSummary, error) {
	workspaces, err := fetchWorkspaces(ctx, cmd)
	if err != nil {
		return workspaceSummary{}, err
	}
	return resolveWorkspaceByIDOrSlug(workspaces, input)
}

func resolveWorkspaceArg(cmd *cobra.Command, args []string) (string, error) {
	if len(args) == 0 {
		return resolveWorkspaceID(cmd), nil
	}
	target := strings.TrimSpace(args[0])
	if uuidRegexp.MatchString(target) {
		return target, nil
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	workspace, err := resolveWorkspaceRef(ctx, cmd, target)
	if err != nil {
		return "", err
	}
	return workspace.ID, nil
}

func runWorkspaceSwitch(cmd *cobra.Command, args []string) error {
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	workspace, err := resolveWorkspaceRef(ctx, cmd, args[0])
	if err != nil {
		return err
	}
	profile := resolveProfile(cmd)
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return err
	}
	cfg.WorkspaceID = workspace.ID
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Switched to workspace: %s (%s)\n", workspace.Name, workspace.ID)
	return nil
}

func runRuntimeProfileSetPath(cmd *cobra.Command, args []string) error {
	path, _ := cmd.Flags().GetString("path")
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return fmt.Errorf("--path must be an absolute path")
	}
	profile := resolveProfile(cmd)
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return err
	}
	if cfg.ProfileCommandOverrides == nil {
		cfg.ProfileCommandOverrides = map[string]string{}
	}
	cfg.ProfileCommandOverrides[args[0]] = path
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Pinned runtime profile %s to %s on this machine.\n", args[0], path)
	return nil
}

func runRuntimeProfileUnsetPath(cmd *cobra.Command, args []string) error {
	profile := resolveProfile(cmd)
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return err
	}
	delete(cfg.ProfileCommandOverrides, args[0])
	if len(cfg.ProfileCommandOverrides) == 0 {
		cfg.ProfileCommandOverrides = nil
	}
	return cli.SaveCLIConfigForProfile(cfg, profile)
}
