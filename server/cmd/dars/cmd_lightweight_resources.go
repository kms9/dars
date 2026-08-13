package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
)

func newLightweightAgentCommands() []*cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List agents", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return lightweightGetAndPrint(cmd, "/api/agents", "list agents")
	}}
	get := &cobra.Command{Use: "get <agent-id>", Short: "Get an agent", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/agents/"+url.PathEscape(args[0]), "get agent")
	}}
	create := &cobra.Command{Use: "create", Short: "Create an agent", Args: cobra.NoArgs, RunE: runLightweightAgentCreate}
	addLightweightAgentMutationFlags(create, true)
	update := &cobra.Command{Use: "update <agent-id>", Short: "Update an agent", Args: exactArgs(1), RunE: runLightweightAgentUpdate}
	addLightweightAgentMutationFlags(update, false)
	archive := &cobra.Command{Use: "archive <agent-id>", Short: "Archive an idle agent", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightAgentLifecycle(cmd, args[0], "archive")
	}}
	restore := &cobra.Command{Use: "restore <agent-id>", Short: "Restore an archived agent", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightAgentLifecycle(cmd, args[0], "restore")
	}}

	env := &cobra.Command{Use: "env", Short: "Reveal or replace encrypted agent environment variables"}
	envGet := &cobra.Command{Use: "get <agent-id>", Short: "Reveal custom_env (audited)", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/agents/"+url.PathEscape(args[0])+"/env", "reveal agent env")
	}}
	envSet := &cobra.Command{Use: "set <agent-id>", Short: "Replace custom_env (audited)", Args: exactArgs(1), RunE: runLightweightAgentEnvSet}
	envSet.Flags().String("custom-env-json", "", "Replacement JSON object; prefer stdin for real secrets")
	envSet.Flags().Bool("custom-env-stdin", false, "Read replacement JSON object from stdin")
	env.AddCommand(envGet, envSet)

	skills := &cobra.Command{Use: "skills", Short: "List or replace agent skill bindings"}
	skillList := &cobra.Command{Use: "list <agent-id>", Short: "List bound skills", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/agents/"+url.PathEscape(args[0])+"/skills", "list agent skills")
	}}
	skillSet := &cobra.Command{Use: "set <agent-id>", Short: "Replace the complete skill binding set", Args: exactArgs(1), RunE: runLightweightAgentSkillsSet}
	skillSet.Flags().String("skills-json", "", "JSON array of {skill_id,enabled} bindings (required)")
	skills.AddCommand(skillList, skillSet)

	return []*cobra.Command{list, get, create, update, archive, restore, env, skills}
}

func addLightweightAgentMutationFlags(cmd *cobra.Command, create bool) {
	for _, field := range []string{"name", "description", "instructions", "runtime-id", "model", "thinking-level", "service-tier", "permission-mode"} {
		cmd.Flags().String(field, "", strings.ReplaceAll(field, "-", " "))
	}
	cmd.Flags().String("runtime-config-json", "", "Runtime config JSON object")
	cmd.Flags().String("custom-args-json", "", "Custom arguments JSON array")
	cmd.Flags().String("mcp-config-json", "", "MCP configuration JSON object; secret material")
	cmd.Flags().String("disabled-runtime-skills-json", "", "Disabled runtime skills JSON array")
	cmd.Flags().String("invocation-targets-json", "", "Invocation targets JSON array")
	cmd.Flags().Int32("max-concurrent-tasks", 0, "Maximum concurrent tasks")
	cmd.Flags().String("body-json", "", "Complete strict mutation JSON object; mutually exclusive with field flags")
	if create {
		_ = cmd.MarkFlagRequired("name")
		_ = cmd.MarkFlagRequired("runtime-id")
	}
}

func lightweightAgentBody(cmd *cobra.Command, create bool) (map[string]any, error) {
	if raw, _ := cmd.Flags().GetString("body-json"); strings.TrimSpace(raw) != "" {
		return decodeJSONObject(raw, "body-json")
	}
	body := map[string]any{}
	for _, flag := range []string{"name", "description", "instructions", "runtime-id", "model", "thinking-level", "service-tier", "permission-mode"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[strings.ReplaceAll(flag, "-", "_")] = value
		}
	}
	for _, spec := range []struct{ flag, key, kind string }{
		{"runtime-config-json", "runtime_config", "object"},
		{"custom-args-json", "custom_args", "array"},
		{"mcp-config-json", "mcp_config", "object"},
		{"disabled-runtime-skills-json", "disabled_runtime_skills", "array"},
		{"invocation-targets-json", "invocation_targets", "array"},
	} {
		if !cmd.Flags().Changed(spec.flag) {
			continue
		}
		raw, _ := cmd.Flags().GetString(spec.flag)
		if spec.kind == "object" {
			value, err := decodeJSONObject(raw, spec.flag)
			if err != nil {
				return nil, err
			}
			body[spec.key] = value
		} else {
			value, err := decodeJSONArray(raw, spec.flag)
			if err != nil {
				return nil, err
			}
			body[spec.key] = value
		}
	}
	if cmd.Flags().Changed("max-concurrent-tasks") {
		value, _ := cmd.Flags().GetInt32("max-concurrent-tasks")
		body["max_concurrent_tasks"] = value
	}
	if create && (strings.TrimSpace(fmt.Sprint(body["name"])) == "" || strings.TrimSpace(fmt.Sprint(body["runtime_id"])) == "") {
		return nil, fmt.Errorf("--name and --runtime-id are required")
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields supplied")
	}
	return body, nil
}

func runLightweightAgentCreate(cmd *cobra.Command, _ []string) error {
	body, err := lightweightAgentBody(cmd, true)
	if err != nil {
		return err
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/agents", body, &result); err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightAgentUpdate(cmd *cobra.Command, args []string) error {
	body, err := lightweightAgentBody(cmd, false)
	if err != nil {
		return err
	}
	return lightweightPutAndPrint(cmd, "/api/agents/"+url.PathEscape(args[0]), body, "update agent")
}

func lightweightAgentLifecycle(cmd *cobra.Command, id, action string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/agents/"+url.PathEscape(id)+"/"+action, map[string]any{}, &result); err != nil {
		return fmt.Errorf("%s agent: %w", action, err)
	}
	return lightweightPrint(result)
}

func runLightweightAgentEnvSet(cmd *cobra.Command, args []string) error {
	inline, _ := cmd.Flags().GetString("custom-env-json")
	useStdin, _ := cmd.Flags().GetBool("custom-env-stdin")
	if useStdin && strings.TrimSpace(inline) != "" {
		return fmt.Errorf("--custom-env-json and --custom-env-stdin are mutually exclusive")
	}
	if useStdin {
		data, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		inline = string(data)
	}
	customEnv, err := decodeJSONObject(inline, "custom-env-json")
	if err != nil {
		return err
	}
	return lightweightPutAndPrint(cmd, "/api/agents/"+url.PathEscape(args[0])+"/env", map[string]any{"custom_env": customEnv}, "update agent env")
}

func runLightweightAgentSkillsSet(cmd *cobra.Command, args []string) error {
	raw, _ := cmd.Flags().GetString("skills-json")
	skills, err := decodeJSONArray(raw, "skills-json")
	if err != nil {
		return err
	}
	return lightweightPutAndPrint(cmd, "/api/agents/"+url.PathEscape(args[0])+"/skills", map[string]any{"skills": skills}, "set agent skills")
}

func newLightweightSkillCommands() []*cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List workspace skills", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return lightweightGetAndPrint(cmd, "/api/skills", "list skills")
	}}
	get := &cobra.Command{Use: "get <skill-id>", Short: "Get a skill", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/skills/"+url.PathEscape(args[0]), "get skill")
	}}
	create := &cobra.Command{Use: "create", Short: "Create a skill", Args: cobra.NoArgs, RunE: runLightweightSkillCreate}
	addLightweightSkillFlags(create, true)
	update := &cobra.Command{Use: "update <skill-id>", Short: "Update a skill", Args: exactArgs(1), RunE: runLightweightSkillUpdate}
	addLightweightSkillFlags(update, false)
	remove := &cobra.Command{Use: "delete <skill-id>", Short: "Delete an unbound skill", Args: exactArgs(1), RunE: runLightweightSkillDelete}
	files := &cobra.Command{Use: "files", Short: "List, replace, or delete skill files"}
	fileList := &cobra.Command{Use: "list <skill-id>", Short: "List skill files", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/skills/"+url.PathEscape(args[0])+"/files", "list skill files")
	}}
	replace := &cobra.Command{Use: "replace <skill-id>", Short: "Atomically replace the complete file set", Args: exactArgs(1), RunE: runLightweightSkillFilesReplace}
	replace.Flags().String("files-json", "", "JSON array of {path,content} files")
	replace.Flags().String("path", "", "Single relative file path")
	replace.Flags().String("content", "", "Single file content")
	fileDelete := &cobra.Command{Use: "delete <skill-id> <file-id>", Short: "Delete a skill file", Args: exactArgs(2), RunE: runLightweightSkillFileDelete}
	files.AddCommand(fileList, replace, fileDelete)

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import a skill from a URL or local .skill/.zip archive",
		Args:  cobra.NoArgs,
		RunE:  runLightweightSkillImport,
	}
	importCmd.Flags().String("url", "", "URL to import from (clawhub.ai, skills.sh, or github.com). Mutually exclusive with --file.")
	importCmd.Flags().String("file", "", "Path to a local skill archive (.skill or .zip). Mutually exclusive with --url.")
	importCmd.Flags().String("on-conflict", "fail", "Conflict strategy: fail, overwrite, rename, or skip")

	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search ClawHub for importable skills",
		Args:  exactArgs(1),
		RunE:  runLightweightSkillSearch,
	}

	return []*cobra.Command{list, get, create, update, remove, importCmd, searchCmd, files}
}

func runLightweightSkillImport(cmd *cobra.Command, _ []string) error {
	importURL, _ := cmd.Flags().GetString("url")
	importFile, _ := cmd.Flags().GetString("file")
	switch {
	case importURL == "" && importFile == "":
		return fmt.Errorf("either --url or --file is required")
	case importURL != "" && importFile != "":
		return fmt.Errorf("--url and --file are mutually exclusive")
	}
	onConflict, _ := cmd.Flags().GetString("on-conflict")
	if !validLightweightSkillImportConflict(onConflict) {
		return fmt.Errorf("--on-conflict must be one of: fail, overwrite, rename, skip")
	}

	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
	defer cancel()

	var result map[string]any
	if importFile != "" {
		fileData, readErr := os.ReadFile(importFile)
		if readErr != nil {
			return fmt.Errorf("read skill archive: %w", readErr)
		}
		if err := client.ImportSkillFile(ctx, fileData, filepath.Base(importFile), onConflict, &result); err != nil {
			if handled := handleLightweightSkillImportError(err); handled != nil {
				return handled
			}
			return fmt.Errorf("import skill: %w", err)
		}
		return printLightweightSkillImportResult(result)
	}

	body := map[string]any{"url": importURL, "on_conflict": onConflict}
	if err := client.PostJSON(ctx, "/api/skills/import", body, &result); err != nil {
		if handled := handleLightweightSkillImportError(err); handled != nil {
			return handled
		}
		return fmt.Errorf("import skill: %w", err)
	}
	return printLightweightSkillImportResult(result)
}

func runLightweightSkillSearch(cmd *cobra.Command, args []string) error {
	query := strings.TrimSpace(args[0])
	if query == "" {
		return fmt.Errorf("query is required")
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(60*time.Second))
	defer cancel()
	var results []map[string]any
	path := "/api/skills/search?q=" + url.QueryEscape(query)
	if err := client.GetJSON(ctx, path, &results); err != nil {
		return fmt.Errorf("search skills: %w", err)
	}
	return lightweightPrint(results)
}

func validLightweightSkillImportConflict(strategy string) bool {
	switch strategy {
	case "fail", "overwrite", "rename", "skip":
		return true
	default:
		return false
	}
}

func handleLightweightSkillImportError(err error) error {
	var httpErr *cli.HTTPError
	if !errors.As(err, &httpErr) || strings.TrimSpace(httpErr.Body) == "" {
		return nil
	}
	var body map[string]any
	if json.Unmarshal([]byte(httpErr.Body), &body) != nil {
		return nil
	}
	if _, ok := body["status"]; !ok {
		if _, hasExisting := body["existing_skill"]; !hasExisting {
			return nil
		}
		body = map[string]any{
			"status":         "conflict",
			"reason":         "a skill with this name already exists; use --on-conflict overwrite or rename",
			"existing_skill": body["existing_skill"],
		}
	}
	_ = printLightweightSkillImportResult(body)
	reason, _ := body["reason"].(string)
	if reason == "" {
		reason, _ = body["error"].(string)
	}
	if reason == "" {
		reason = "skill import conflict"
	}
	return errors.New(reason)
}

func printLightweightSkillImportResult(result map[string]any) error {
	return lightweightPrint(result)
}

func addLightweightSkillFlags(cmd *cobra.Command, create bool) {
	cmd.Flags().String("name", "", "Skill name")
	cmd.Flags().String("description", "", "Skill description")
	cmd.Flags().String("content", "", "SKILL.md content")
	cmd.Flags().String("config-json", "", "Skill config JSON object")
	if create {
		_ = cmd.MarkFlagRequired("name")
	}
}

func lightweightSkillBody(cmd *cobra.Command, create bool) (map[string]any, error) {
	body := map[string]any{}
	for _, flag := range []string{"name", "description", "content"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[flag] = value
		}
	}
	if cmd.Flags().Changed("config-json") {
		raw, _ := cmd.Flags().GetString("config-json")
		value, err := decodeJSONObject(raw, "config-json")
		if err != nil {
			return nil, err
		}
		body["config"] = value
	}
	if create && strings.TrimSpace(fmt.Sprint(body["name"])) == "" {
		return nil, fmt.Errorf("--name is required")
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields supplied")
	}
	return body, nil
}

func runLightweightSkillCreate(cmd *cobra.Command, _ []string) error {
	body, err := lightweightSkillBody(cmd, true)
	if err != nil {
		return err
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/skills", body, &result); err != nil {
		return fmt.Errorf("create skill: %w", err)
	}
	return lightweightPrint(result)
}
func runLightweightSkillUpdate(cmd *cobra.Command, args []string) error {
	body, err := lightweightSkillBody(cmd, false)
	if err != nil {
		return err
	}
	return lightweightPutAndPrint(cmd, "/api/skills/"+url.PathEscape(args[0]), body, "update skill")
}
func runLightweightSkillDelete(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSON(ctx, "/api/skills/"+url.PathEscape(args[0]))
}

func runLightweightSkillFilesReplace(cmd *cobra.Command, args []string) error {
	raw, _ := cmd.Flags().GetString("files-json")
	path, _ := cmd.Flags().GetString("path")
	content, _ := cmd.Flags().GetString("content")
	var files []any
	if strings.TrimSpace(raw) != "" {
		decoded, err := decodeJSONArray(raw, "files-json")
		if err != nil {
			return err
		}
		files = decoded
	} else if strings.TrimSpace(path) != "" {
		files = []any{map[string]any{"path": path, "content": content}}
	} else {
		return fmt.Errorf("use --files-json or --path with --content")
	}
	return lightweightPutAndPrint(cmd, "/api/skills/"+url.PathEscape(args[0])+"/files", map[string]any{"files": files}, "replace skill files")
}

func runLightweightSkillFileDelete(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSON(ctx, "/api/skills/"+url.PathEscape(args[0])+"/files/"+url.PathEscape(args[1]))
}

func decodeJSONValue(raw, flag string) (any, error) {
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, fmt.Errorf("--%s must be valid JSON", flag)
	}
	return value, nil
}

func newLightweightWorkspaceCommands() []*cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List workspaces", Args: cobra.NoArgs, RunE: runLightweightWorkspaceList}
	create := &cobra.Command{Use: "create", Short: "Create an isolated Lightweight workspace", Args: cobra.NoArgs, RunE: runLightweightWorkspaceCreate}
	create.Flags().String("name", "", "Workspace name (required)")
	create.Flags().String("slug", "", "Optional stable slug")
	get := &cobra.Command{Use: "get [workspace-id|slug|prefix]", Short: "Get a workspace", Args: cobra.MaximumNArgs(1), RunE: runLightweightWorkspaceGet}
	update := &cobra.Command{Use: "update [workspace-id|slug|prefix]", Short: "Update workspace settings", Args: cobra.MaximumNArgs(1), RunE: runLightweightWorkspaceUpdate}
	update.Flags().String("name", "", "New name")
	update.Flags().String("description", "", "New description")
	update.Flags().String("context", "", "New context")
	update.Flags().String("settings-json", "", "Replacement settings JSON")
	update.Flags().String("repos-json", "", "Replacement repositories JSON")
	remove := &cobra.Command{Use: "delete [workspace-id|slug|prefix]", Short: "Delete an idle workspace (owner only)", Args: cobra.MaximumNArgs(1), RunE: runLightweightWorkspaceDelete}
	members := &cobra.Command{Use: "member", Short: "Read-only workspace members"}
	memberList := &cobra.Command{Use: "list [workspace-id|slug|prefix]", Short: "List read-only workspace members", Args: cobra.MaximumNArgs(1), RunE: runLightweightWorkspaceMembers}
	members.AddCommand(memberList)
	switchCmd := &cobra.Command{Use: "switch <workspace-id|slug|prefix>", Short: "Set the default workspace", Args: exactArgs(1), RunE: runWorkspaceSwitch}
	return []*cobra.Command{list, create, get, update, remove, members, switchCmd}
}

func runLightweightWorkspaceList(cmd *cobra.Command, _ []string) error {
	client, err := lightweightClient(cmd, false)
	if err != nil {
		return err
	}
	client.WorkspaceID = ""
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result []map[string]any
	if err := client.GetJSON(ctx, "/api/workspaces", &result); err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightWorkspaceCreate(cmd *cobra.Command, _ []string) error {
	name, _ := cmd.Flags().GetString("name")
	slug, _ := cmd.Flags().GetString("slug")
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("--name is required")
	}
	body := map[string]any{"name": name}
	if strings.TrimSpace(slug) != "" {
		body["slug"] = slug
	}
	client, err := lightweightClient(cmd, false)
	if err != nil {
		return err
	}
	client.WorkspaceID = ""
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/workspaces", body, &result); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	return lightweightPrint(result)
}

func lightweightWorkspaceScope(cmd *cobra.Command, args []string) (*cli.APIClient, string, error) {
	id, err := resolveWorkspaceArg(cmd, args)
	if err != nil {
		return nil, "", err
	}
	if id == "" {
		return nil, "", fmt.Errorf("workspace ID is required")
	}
	client, err := lightweightClient(cmd, false)
	if err != nil {
		return nil, "", err
	}
	client.WorkspaceID = id
	return client, id, nil
}

func runLightweightWorkspaceGet(cmd *cobra.Command, args []string) error {
	client, id, err := lightweightWorkspaceScope(cmd, args)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.GetJSON(ctx, "/api/workspaces/"+url.PathEscape(id), &result); err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightWorkspaceUpdate(cmd *cobra.Command, args []string) error {
	body := map[string]any{}
	for _, flag := range []string{"name", "description", "context"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[flag] = value
		}
	}
	for _, spec := range []struct{ flag, key string }{{"settings-json", "settings"}, {"repos-json", "repos"}} {
		if cmd.Flags().Changed(spec.flag) {
			raw, _ := cmd.Flags().GetString(spec.flag)
			value, err := decodeJSONValue(raw, spec.flag)
			if err != nil {
				return err
			}
			body[spec.key] = value
		}
	}
	if len(body) == 0 {
		return fmt.Errorf("no fields supplied")
	}
	client, id, err := lightweightWorkspaceScope(cmd, args)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PutJSON(ctx, "/api/workspaces/"+url.PathEscape(id), body, &result); err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightWorkspaceDelete(cmd *cobra.Command, args []string) error {
	client, id, err := lightweightWorkspaceScope(cmd, args)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSON(ctx, "/api/workspaces/"+url.PathEscape(id))
}

func runLightweightWorkspaceMembers(cmd *cobra.Command, args []string) error {
	client, id, err := lightweightWorkspaceScope(cmd, args)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result []map[string]any
	if err := client.GetJSON(ctx, "/api/workspaces/"+url.PathEscape(id)+"/members", &result); err != nil {
		return fmt.Errorf("list workspace members: %w", err)
	}
	return lightweightPrint(result)
}

func newLightweightRuntimeCommands() []*cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List local runtimes", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return lightweightGetAndPrint(cmd, "/api/runtimes", "list runtimes")
	}}
	rename := &cobra.Command{Use: "rename <runtime-id> <name>", Short: "Set a runtime display name", Args: exactArgs(2), RunE: runLightweightRuntimeRename}
	remove := &cobra.Command{Use: "delete <runtime-id>", Short: "Delete an offline runtime", Args: exactArgs(1), RunE: runLightweightRuntimeDelete}
	remove.Flags().StringSlice("expected-active-agent-ids", nil, "Confirm the exact active agent set and unbind it before deletion")
	profiles := &cobra.Command{Use: "profile", Short: "Manage runtime profiles"}
	profileList := &cobra.Command{Use: "list", Short: "List runtime profiles", Args: cobra.NoArgs, RunE: runLightweightProfileList}
	profileGet := &cobra.Command{Use: "get <profile-id>", Short: "Get a runtime profile", Args: exactArgs(1), RunE: runLightweightProfileGet}
	profileCreate := &cobra.Command{Use: "create", Short: "Create a runtime profile", Args: cobra.NoArgs, RunE: runLightweightProfileCreate}
	addLightweightProfileFlags(profileCreate, true)
	profileUpdate := &cobra.Command{Use: "update <profile-id>", Short: "Update a runtime profile", Args: exactArgs(1), RunE: runLightweightProfileUpdate}
	addLightweightProfileFlags(profileUpdate, false)
	profileDelete := &cobra.Command{Use: "delete <profile-id>", Short: "Delete an unused runtime profile", Args: exactArgs(1), RunE: runLightweightProfileDelete}
	setPath := &cobra.Command{Use: "set-path <profile-id>", Short: "Pin a local executable path", Args: exactArgs(1), RunE: runRuntimeProfileSetPath}
	setPath.Flags().String("path", "", "Absolute executable path (required)")
	unsetPath := &cobra.Command{Use: "unset-path <profile-id>", Short: "Remove a local executable override", Args: exactArgs(1), RunE: runRuntimeProfileUnsetPath}
	profiles.AddCommand(profileList, profileGet, profileCreate, profileUpdate, profileDelete, setPath, unsetPath)
	return []*cobra.Command{list, rename, remove, profiles}
}

func runLightweightRuntimeRename(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PatchJSON(ctx, "/api/runtimes/"+url.PathEscape(args[0]), map[string]any{"custom_name": args[1]}, &result); err != nil {
		return fmt.Errorf("rename runtime: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightRuntimeDelete(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	ids, _ := cmd.Flags().GetStringSlice("expected-active-agent-ids")
	if len(ids) == 0 {
		return client.DeleteJSON(ctx, "/api/runtimes/"+url.PathEscape(args[0]))
	}
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/runtimes/"+url.PathEscape(args[0])+"/unbind-agents-and-delete", map[string]any{"expected_active_agent_ids": ids}, &result); err != nil {
		return fmt.Errorf("unbind agents and delete runtime: %w", err)
	}
	return lightweightPrint(result)
}

func addLightweightProfileFlags(cmd *cobra.Command, create bool) {
	cmd.Flags().String("display-name", "", "Display name")
	cmd.Flags().String("protocol-family", "", "Protocol family")
	cmd.Flags().String("command-name", "", "Executable command")
	cmd.Flags().String("description", "", "Description")
	cmd.Flags().String("fixed-args-json", "", "Fixed arguments JSON array")
	cmd.Flags().Bool("enabled", true, "Whether the profile is enabled")
	if create {
		_ = cmd.MarkFlagRequired("display-name")
		_ = cmd.MarkFlagRequired("protocol-family")
		_ = cmd.MarkFlagRequired("command-name")
	}
}

func lightweightProfileBody(cmd *cobra.Command, create bool) (map[string]any, error) {
	body := map[string]any{}
	for _, flag := range []string{"display-name", "protocol-family", "command-name", "description"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[strings.ReplaceAll(flag, "-", "_")] = value
		}
	}
	if cmd.Flags().Changed("fixed-args-json") {
		raw, _ := cmd.Flags().GetString("fixed-args-json")
		value, err := decodeJSONArray(raw, "fixed-args-json")
		if err != nil {
			return nil, err
		}
		body["fixed_args"] = value
	}
	if cmd.Flags().Changed("enabled") {
		value, _ := cmd.Flags().GetBool("enabled")
		body["enabled"] = value
	}
	if create && (strings.TrimSpace(fmt.Sprint(body["display_name"])) == "" || strings.TrimSpace(fmt.Sprint(body["protocol_family"])) == "" || strings.TrimSpace(fmt.Sprint(body["command_name"])) == "") {
		return nil, fmt.Errorf("--display-name, --protocol-family and --command-name are required")
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields supplied")
	}
	return body, nil
}

func lightweightProfilesPath(cmd *cobra.Command) (string, error) {
	id, err := requireWorkspaceID(cmd)
	if err != nil {
		return "", err
	}
	return "/api/workspaces/" + url.PathEscape(id) + "/runtime-profiles", nil
}
func runLightweightProfileList(cmd *cobra.Command, _ []string) error {
	path, err := lightweightProfilesPath(cmd)
	if err != nil {
		return err
	}
	return lightweightGetAndPrint(cmd, path, "list runtime profiles")
}
func runLightweightProfileGet(cmd *cobra.Command, args []string) error {
	path, err := lightweightProfilesPath(cmd)
	if err != nil {
		return err
	}
	return lightweightGetAndPrint(cmd, path+"/"+url.PathEscape(args[0]), "get runtime profile")
}
func runLightweightProfileCreate(cmd *cobra.Command, _ []string) error {
	body, err := lightweightProfileBody(cmd, true)
	if err != nil {
		return err
	}
	path, err := lightweightProfilesPath(cmd)
	if err != nil {
		return err
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, path, body, &result); err != nil {
		return fmt.Errorf("create runtime profile: %w", err)
	}
	return lightweightPrint(result)
}
func runLightweightProfileUpdate(cmd *cobra.Command, args []string) error {
	body, err := lightweightProfileBody(cmd, false)
	if err != nil {
		return err
	}
	path, err := lightweightProfilesPath(cmd)
	if err != nil {
		return err
	}
	return lightweightPutAndPrint(cmd, path+"/"+url.PathEscape(args[0]), body, "update runtime profile")
}
func runLightweightProfileDelete(cmd *cobra.Command, args []string) error {
	path, err := lightweightProfilesPath(cmd)
	if err != nil {
		return err
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSON(ctx, path+"/"+url.PathEscape(args[0]))
}

func newLightweightSquadCommands() []*cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List agent-only squads", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return lightweightGetAndPrint(cmd, "/api/squads", "list squads")
	}}
	get := &cobra.Command{Use: "get <squad-id>", Short: "Get a squad", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/squads/"+url.PathEscape(args[0]), "get squad")
	}}
	create := &cobra.Command{Use: "create", Short: "Create a squad with exactly one leader", Args: cobra.NoArgs, RunE: runLightweightSquadCreate}
	addLightweightSquadFlags(create, true)
	update := &cobra.Command{Use: "update <squad-id>", Short: "Update a squad or atomically replace its leader", Args: exactArgs(1), RunE: runLightweightSquadUpdate}
	addLightweightSquadFlags(update, false)
	archive := &cobra.Command{Use: "archive <squad-id>", Short: "Archive an idle squad", Args: exactArgs(1), RunE: runLightweightSquadArchive}
	evaluate := &cobra.Command{Use: "evaluate <run-id>", Short: "Record the current Leader Task decision (Task Token only)", Args: exactArgs(1), RunE: runLightweightSquadEvaluate}
	evaluate.Flags().String("outcome", "", "action, no_action, or failed (required)")
	evaluate.Flags().String("reason", "", "Decision reason (redacted and capped server-side)")
	members := &cobra.Command{Use: "member", Short: "Manage squad agents and roles"}
	memberList := &cobra.Command{Use: "list <squad-id>", Short: "List squad agents", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/squads/"+url.PathEscape(args[0])+"/members", "list squad members")
	}}
	memberStatus := &cobra.Command{Use: "status <squad-id>", Short: "List live agent status", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/squads/"+url.PathEscape(args[0])+"/members/status", "list squad member status")
	}}
	memberAdd := &cobra.Command{Use: "add <squad-id>", Short: "Add an agent", Args: exactArgs(1), RunE: runLightweightSquadMemberAdd}
	addSquadMemberFlags(memberAdd)
	memberRole := &cobra.Command{Use: "set-role <squad-id>", Short: "Change role or atomically select the leader", Args: exactArgs(1), RunE: runLightweightSquadMemberRole}
	addSquadMemberFlags(memberRole)
	memberRemove := &cobra.Command{Use: "remove <squad-id>", Short: "Remove a non-leader agent", Args: exactArgs(1), RunE: runLightweightSquadMemberRemove}
	memberRemove.Flags().String("agent-id", "", "Agent UUID (required)")
	members.AddCommand(memberList, memberStatus, memberAdd, memberRole, memberRemove)
	return []*cobra.Command{list, get, create, update, archive, evaluate, members}
}

func addLightweightSquadFlags(cmd *cobra.Command, create bool) {
	cmd.Flags().String("name", "", "Squad name")
	cmd.Flags().String("description", "", "Description")
	cmd.Flags().String("instructions", "", "Leader coordination instructions")
	cmd.Flags().String("leader-id", "", "Leader agent UUID")
	if create {
		_ = cmd.MarkFlagRequired("name")
		_ = cmd.MarkFlagRequired("leader-id")
	}
}

func lightweightSquadBody(cmd *cobra.Command, create bool) (map[string]any, error) {
	body := map[string]any{}
	for _, flag := range []string{"name", "description", "instructions", "leader-id"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[strings.ReplaceAll(flag, "-", "_")] = value
		}
	}
	if create && (strings.TrimSpace(fmt.Sprint(body["name"])) == "" || strings.TrimSpace(fmt.Sprint(body["leader_id"])) == "") {
		return nil, fmt.Errorf("--name and --leader-id are required")
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields supplied")
	}
	return body, nil
}

func runLightweightSquadCreate(cmd *cobra.Command, _ []string) error {
	body, err := lightweightSquadBody(cmd, true)
	if err != nil {
		return err
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/squads", body, &result); err != nil {
		return fmt.Errorf("create squad: %w", err)
	}
	return lightweightPrint(result)
}
func runLightweightSquadUpdate(cmd *cobra.Command, args []string) error {
	body, err := lightweightSquadBody(cmd, false)
	if err != nil {
		return err
	}
	return lightweightPutAndPrint(cmd, "/api/squads/"+url.PathEscape(args[0]), body, "update squad")
}
func runLightweightSquadArchive(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSON(ctx, "/api/squads/"+url.PathEscape(args[0]))
}

func runLightweightSquadEvaluate(cmd *cobra.Command, args []string) error {
	outcome, _ := cmd.Flags().GetString("outcome")
	if outcome != "action" && outcome != "no_action" && outcome != "failed" {
		return fmt.Errorf("--outcome must be action, no_action, or failed")
	}
	reason, _ := cmd.Flags().GetString("reason")
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+url.PathEscape(args[0])+"/squad-evaluated", map[string]any{"outcome": outcome, "reason": reason}, &result); err != nil {
		return fmt.Errorf("record squad leader evaluation: %w", err)
	}
	return lightweightPrint(result)
}

func addSquadMemberFlags(cmd *cobra.Command) {
	cmd.Flags().String("agent-id", "", "Agent UUID (required)")
	cmd.Flags().String("role", "member", "member or leader")
}
func lightweightSquadMemberBody(cmd *cobra.Command) (map[string]any, error) {
	agentID, _ := cmd.Flags().GetString("agent-id")
	role, _ := cmd.Flags().GetString("role")
	if strings.TrimSpace(agentID) == "" || (role != "member" && role != "leader") {
		return nil, fmt.Errorf("--agent-id and --role (member or leader) are required")
	}
	return map[string]any{"agent_id": agentID, "role": role}, nil
}
func runLightweightSquadMemberAdd(cmd *cobra.Command, args []string) error {
	body, err := lightweightSquadMemberBody(cmd)
	if err != nil {
		return err
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/squads/"+url.PathEscape(args[0])+"/members", body, &result); err != nil {
		return fmt.Errorf("add squad member: %w", err)
	}
	return lightweightPrint(result)
}
func runLightweightSquadMemberRole(cmd *cobra.Command, args []string) error {
	body, err := lightweightSquadMemberBody(cmd)
	if err != nil {
		return err
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PatchJSON(ctx, "/api/squads/"+url.PathEscape(args[0])+"/members/role", body, &result); err != nil {
		return fmt.Errorf("set squad member role: %w", err)
	}
	return lightweightPrint(result)
}
func runLightweightSquadMemberRemove(cmd *cobra.Command, args []string) error {
	agentID, _ := cmd.Flags().GetString("agent-id")
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("--agent-id is required")
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSONWithBody(ctx, "/api/squads/"+url.PathEscape(args[0])+"/members", map[string]any{"agent_id": agentID})
}

func newLightweightUserCommands() []*cobra.Command {
	get := &cobra.Command{Use: "get", Short: "Show the current user", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := lightweightClient(cmd, false)
		if err != nil {
			return err
		}
		client.WorkspaceID = ""
		ctx, cancel := cli.APIContext(context.Background())
		defer cancel()
		var result map[string]any
		if err := client.GetJSON(ctx, "/api/me", &result); err != nil {
			return fmt.Errorf("get current user: %w", err)
		}
		return lightweightPrint(result)
	}}
	update := &cobra.Command{Use: "update", Short: "Update name, language, or timezone", Args: cobra.NoArgs, RunE: runLightweightUserUpdate}
	update.Flags().String("name", "", "Display name")
	update.Flags().String("language", "", "en, zh-Hans, ko, or ja")
	update.Flags().String("timezone", "", "IANA timezone; use --clear-timezone to clear")
	update.Flags().Bool("clear-timezone", false, "Clear timezone")
	return []*cobra.Command{get, update}
}

func runLightweightUserUpdate(cmd *cobra.Command, _ []string) error {
	body := map[string]any{}
	for _, flag := range []string{"name", "language", "timezone"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[flag] = value
		}
	}
	clear, _ := cmd.Flags().GetBool("clear-timezone")
	if clear {
		if cmd.Flags().Changed("timezone") {
			return fmt.Errorf("--timezone and --clear-timezone are mutually exclusive")
		}
		body["timezone"] = nil
	}
	if len(body) == 0 {
		return fmt.Errorf("no fields supplied")
	}
	client, err := lightweightClient(cmd, false)
	if err != nil {
		return err
	}
	client.WorkspaceID = ""
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PatchJSON(ctx, "/api/me", body, &result); err != nil {
		return fmt.Errorf("update current user: %w", err)
	}
	return lightweightPrint(result)
}
