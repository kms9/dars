package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
)

func lightweightClient(cmd *cobra.Command, workspaceRequired bool) (*cli.APIClient, error) {
	client, err := newAPIClient(cmd)
	if err != nil {
		return nil, err
	}
	if workspaceRequired && client.WorkspaceID == "" {
		if _, err := requireWorkspaceID(cmd); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func lightweightPrint(value any) error {
	return cli.PrintJSON(os.Stdout, value)
}

func lightweightIdempotencyKey(cmd *cobra.Command, scope string) (string, error) {
	if flag := cmd.Flags().Lookup("idempotency-key"); flag != nil {
		value, _ := cmd.Flags().GetString("idempotency-key")
		if value = strings.TrimSpace(value); value != "" {
			return value, nil
		}
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate idempotency key: %w", err)
	}
	return "cli-" + scope + "-" + hex.EncodeToString(random[:]), nil
}

func lightweightPost(cmd *cobra.Command, path, scope string, body, out any) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	key, err := lightweightIdempotencyKey(cmd, scope)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.PostJSONWithHeaders(ctx, path, body, out, map[string]string{"Idempotency-Key": key})
}

func decodeJSONObject(value, flag string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(value), &result); err != nil || result == nil {
		return nil, fmt.Errorf("--%s must be a JSON object", flag)
	}
	return result, nil
}

func decodeJSONArray(value, flag string) ([]any, error) {
	var result []any
	if err := json.Unmarshal([]byte(value), &result); err != nil || result == nil {
		return nil, fmt.Errorf("--%s must be a JSON array", flag)
	}
	return result, nil
}

func newLightweightIssueCommands() []*cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List Lightweight runs", Args: cobra.NoArgs, RunE: runLightweightIssueList}
	list.Flags().Int("limit", 50, "Maximum number of runs (1-100)")
	list.Flags().String("cursor", "", "Opaque next_cursor from the previous page")
	list.Flags().StringSlice("status", nil, "Status filter (repeatable)")
	list.Flags().String("assignee-type", "", "Assignee type: agent or squad")
	list.Flags().String("assignee-id", "", "Assignee UUID")

	get := &cobra.Command{Use: "get <run-id>", Short: "Get a run", Args: exactArgs(1), RunE: runLightweightIssueGet}

	create := &cobra.Command{Use: "create", Short: "Create a run", Args: cobra.NoArgs, RunE: runLightweightIssueCreate}
	addRunMutationFlags(create, true)
	create.Flags().String("idempotency-key", "", "Retry-stable idempotency key (generated when omitted)")

	update := &cobra.Command{Use: "update <run-id>", Short: "Update a run", Args: exactArgs(1), RunE: runLightweightIssueUpdate}
	addRunMutationFlags(update, false)

	status := &cobra.Command{Use: "status <run-id> <status>", Short: "Set run status", Args: exactArgs(2), RunE: runLightweightIssueStatus}
	assign := &cobra.Command{Use: "assign <run-id>", Short: "Assign a run to an agent or squad", Args: exactArgs(1), RunE: runLightweightIssueAssign}
	assign.Flags().String("assignee-type", "", "Assignee type: agent or squad (required)")
	assign.Flags().String("assignee-id", "", "Assignee UUID (required)")
	remove := &cobra.Command{Use: "delete <run-id>", Short: "Delete an idle run", Args: exactArgs(1), RunE: runLightweightIssueDelete}

	comments := &cobra.Command{Use: "comment", Short: "List and append flat run comments"}
	commentList := &cobra.Command{Use: "list <run-id>", Short: "List flat comments", Args: exactArgs(1), RunE: runLightweightCommentList}
	commentList.Flags().Int("limit", 100, "Maximum comments (1-100)")
	commentList.Flags().String("cursor", "", "Opaque next_cursor")
	commentAdd := &cobra.Command{Use: "add <run-id>", Short: "Append a comment", Args: exactArgs(1), RunE: runLightweightCommentAdd}
	commentAdd.Flags().String("content", "", "Plain text or Markdown comment (required)")
	commentAdd.Flags().Bool("content-stdin", false, "Read comment content from stdin")
	commentAdd.Flags().String("content-file", "", "Read comment content from a UTF-8 file inside the current workdir")
	commentAdd.Flags().Bool("allow-external-file", false, "Allow --content-file outside the current workdir")
	commentAdd.Flags().String("idempotency-key", "", "Retry-stable idempotency key (generated when omitted)")
	comments.AddCommand(commentList, commentAdd)

	runs := &cobra.Command{Use: "runs <run-id>", Short: "List task runs", Args: exactArgs(1), RunE: runLightweightTaskRuns}
	runs.Flags().Int("limit", 100, "Maximum task runs (1-100)")
	runs.Flags().String("cursor", "", "Opaque next_cursor")
	messages := &cobra.Command{Use: "run-messages <task-id>", Short: "List task messages", Args: exactArgs(1), RunE: runLightweightTaskMessages}
	messages.Flags().Int("limit", 100, "Maximum messages (1-100)")
	messages.Flags().String("cursor", "", "Opaque next_cursor")
	cancel := &cobra.Command{Use: "cancel-task <task-id>", Short: "Cancel a queued or running task", Args: exactArgs(1), RunE: runLightweightCancelTask}

	return []*cobra.Command{list, get, create, update, status, assign, remove, comments, runs, messages, cancel}
}

func addRunMutationFlags(cmd *cobra.Command, create bool) {
	cmd.Flags().String("title", "", "Run title")
	cmd.Flags().String("description", "", "Run description")
	cmd.Flags().String("status", "", "backlog, todo, in_progress, in_review, done, blocked, or cancelled")
	cmd.Flags().String("assignee-type", "", "Assignee type: agent or squad")
	cmd.Flags().String("assignee-id", "", "Assignee UUID")
	cmd.Flags().String("acceptance-criteria-json", "", "Acceptance criteria JSON array")
	cmd.Flags().String("context-refs-json", "", "Context references JSON array")
	if create {
		_ = cmd.MarkFlagRequired("title")
	}
}

func runMutationBody(cmd *cobra.Command, requireTitle bool) (map[string]any, error) {
	body := map[string]any{}
	for _, flag := range []string{"title", "description", "status", "assignee-type", "assignee-id"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[strings.ReplaceAll(flag, "-", "_")] = value
		}
	}
	if requireTitle && strings.TrimSpace(fmt.Sprint(body["title"])) == "" {
		return nil, fmt.Errorf("--title is required")
	}
	if (body["assignee_type"] == nil) != (body["assignee_id"] == nil) {
		return nil, fmt.Errorf("--assignee-type and --assignee-id must be provided together")
	}
	for _, flag := range []string{"acceptance-criteria-json", "context-refs-json"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			decoded, err := decodeJSONArray(value, flag)
			if err != nil {
				return nil, err
			}
			body[strings.TrimSuffix(strings.ReplaceAll(flag, "-", "_"), "_json")] = decoded
		}
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("no fields supplied")
	}
	return body, nil
}

func runLightweightIssueList(cmd *cobra.Command, _ []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	query := url.Values{}
	limit, _ := cmd.Flags().GetInt("limit")
	query.Set("limit", strconv.Itoa(limit))
	if cursor, _ := cmd.Flags().GetString("cursor"); cursor != "" {
		query.Set("cursor", cursor)
	}
	statuses, _ := cmd.Flags().GetStringSlice("status")
	for _, status := range statuses {
		query.Add("status", status)
	}
	for _, flag := range []string{"assignee-type", "assignee-id"} {
		if value, _ := cmd.Flags().GetString(flag); value != "" {
			query.Set(strings.ReplaceAll(flag, "-", "_"), value)
		}
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.GetJSON(ctx, "/api/issues?"+query.Encode(), &result); err != nil {
		return fmt.Errorf("list runs: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightIssueGet(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.GetJSON(ctx, "/api/issues/"+url.PathEscape(args[0]), &result); err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightIssueCreate(cmd *cobra.Command, _ []string) error {
	body, err := runMutationBody(cmd, true)
	if err != nil {
		return err
	}
	var result map[string]any
	if err := lightweightPost(cmd, "/api/issues", "run", body, &result); err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightIssueUpdate(cmd *cobra.Command, args []string) error {
	body, err := runMutationBody(cmd, false)
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
	if err := client.PutJSON(ctx, "/api/issues/"+url.PathEscape(args[0]), body, &result); err != nil {
		return fmt.Errorf("update run: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightIssueStatus(cmd *cobra.Command, args []string) error {
	if err := validateIssueStatus(args[1]); err != nil {
		return err
	}
	return lightweightPutAndPrint(cmd, "/api/issues/"+url.PathEscape(args[0]), map[string]any{"status": args[1]}, "update run status")
}

func runLightweightIssueAssign(cmd *cobra.Command, args []string) error {
	typeValue, _ := cmd.Flags().GetString("assignee-type")
	id, _ := cmd.Flags().GetString("assignee-id")
	if (typeValue != "agent" && typeValue != "squad") || strings.TrimSpace(id) == "" {
		return fmt.Errorf("--assignee-type (agent or squad) and --assignee-id are required")
	}
	return lightweightPutAndPrint(cmd, "/api/issues/"+url.PathEscape(args[0]), map[string]any{"assignee_type": typeValue, "assignee_id": id}, "assign run")
}

func lightweightPutAndPrint(cmd *cobra.Command, path string, body map[string]any, action string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PutJSON(ctx, path, body, &result); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return lightweightPrint(result)
}

func runLightweightIssueDelete(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	if err := client.DeleteJSON(ctx, "/api/issues/"+url.PathEscape(args[0])); err != nil {
		return fmt.Errorf("delete run: %w", err)
	}
	return nil
}

func pagePath(base string, cmd *cobra.Command) string {
	query := url.Values{}
	if flag := cmd.Flags().Lookup("limit"); flag != nil {
		limit, _ := cmd.Flags().GetInt("limit")
		query.Set("limit", strconv.Itoa(limit))
	}
	if flag := cmd.Flags().Lookup("cursor"); flag != nil {
		if cursor, _ := cmd.Flags().GetString("cursor"); cursor != "" {
			query.Set("cursor", cursor)
		}
	}
	if encoded := query.Encode(); encoded != "" {
		return base + "?" + encoded
	}
	return base
}

func runLightweightCommentList(cmd *cobra.Command, args []string) error {
	return lightweightGetAndPrint(cmd, pagePath("/api/issues/"+url.PathEscape(args[0])+"/comments", cmd), "list comments")
}

func runLightweightCommentAdd(cmd *cobra.Command, args []string) error {
	content, fromStdin, err := resolveTextFlag(cmd, "content")
	if err != nil {
		return err
	}
	if !fromStdin && !cmd.Flags().Changed("content") {
		content, _ = cmd.Flags().GetString("content")
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("--content, --content-stdin, or --content-file is required")
	}
	var result map[string]any
	if err := lightweightPost(cmd, "/api/issues/"+url.PathEscape(args[0])+"/comments", "comment", map[string]any{"content": content}, &result); err != nil {
		return fmt.Errorf("add comment: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightTaskRuns(cmd *cobra.Command, args []string) error {
	return lightweightGetAndPrint(cmd, pagePath("/api/issues/"+url.PathEscape(args[0])+"/task-runs", cmd), "list task runs")
}

func runLightweightTaskMessages(cmd *cobra.Command, args []string) error {
	return lightweightGetAndPrint(cmd, pagePath("/api/tasks/"+url.PathEscape(args[0])+"/messages", cmd), "list task messages")
}

func runLightweightCancelTask(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/tasks/"+url.PathEscape(args[0])+"/cancel", map[string]any{}, &result); err != nil {
		return fmt.Errorf("cancel task: %w", err)
	}
	return lightweightPrint(result)
}

func lightweightGetAndPrint(cmd *cobra.Command, path, action string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result any
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return lightweightPrint(result)
}

func newLightweightChatCommands() []*cobra.Command {
	list := &cobra.Command{Use: "list", Short: "List direct chat sessions", Args: cobra.NoArgs, RunE: runLightweightChatList}
	list.Flags().Int("limit", 100, "Maximum sessions (1-100)")
	list.Flags().String("cursor", "", "Opaque next_cursor")
	create := &cobra.Command{Use: "create", Short: "Create a direct chat session", Args: cobra.NoArgs, RunE: runLightweightChatCreate}
	create.Flags().String("agent-id", "", "Agent UUID (required)")
	create.Flags().String("title", "", "Session title")
	get := &cobra.Command{Use: "get <session-id>", Short: "Get a chat session", Args: exactArgs(1), RunE: runLightweightChatGet}
	update := &cobra.Command{Use: "update <session-id>", Short: "Rename, archive, or restore a chat session", Args: exactArgs(1), RunE: runLightweightChatUpdate}
	update.Flags().String("title", "", "New title")
	update.Flags().String("status", "", "active or archived")
	remove := &cobra.Command{Use: "delete <session-id>", Short: "Delete an archived chat session", Args: exactArgs(1), RunE: runLightweightChatDelete}
	messages := &cobra.Command{Use: "messages", Short: "List and send direct chat messages"}
	messageList := &cobra.Command{Use: "list <session-id>", Short: "List messages", Args: exactArgs(1), RunE: runLightweightChatMessages}
	messageList.Flags().Int("limit", 100, "Maximum messages (1-100)")
	messageList.Flags().String("cursor", "", "Opaque next_cursor")
	send := &cobra.Command{Use: "send <session-id>", Short: "Send a plain-text message", Args: exactArgs(1), RunE: runLightweightChatSend}
	send.Flags().String("content", "", "Message content (required)")
	send.Flags().Bool("content-stdin", false, "Read message content from stdin")
	send.Flags().String("idempotency-key", "", "Retry-stable idempotency key (generated when omitted)")
	messages.AddCommand(messageList, send)
	pending := &cobra.Command{Use: "pending-task <session-id>", Short: "Get the session's current task", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/chat/sessions/"+url.PathEscape(args[0])+"/pending-task", "get pending task")
	}}
	drafts := &cobra.Command{Use: "draft-restores", Short: "List and consume recovered drafts"}
	draftList := &cobra.Command{Use: "list <session-id>", Short: "List recovered drafts", Args: exactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return lightweightGetAndPrint(cmd, "/api/chat/sessions/"+url.PathEscape(args[0])+"/draft-restores", "list draft restores")
	}}
	draftDelete := &cobra.Command{Use: "delete <session-id> <restore-id>", Short: "Consume a recovered draft", Args: exactArgs(2), RunE: runLightweightDraftDelete}
	drafts.AddCommand(draftList, draftDelete)
	cancel := &cobra.Command{Use: "cancel-task <task-id>", Short: "Cancel a direct chat task", Args: exactArgs(1), RunE: runLightweightCancelTask}
	return []*cobra.Command{list, create, get, update, remove, messages, pending, drafts, cancel}
}

func runLightweightChatList(cmd *cobra.Command, _ []string) error {
	return lightweightGetAndPrint(cmd, pagePath("/api/chat/sessions", cmd), "list chat sessions")
}
func runLightweightChatGet(cmd *cobra.Command, args []string) error {
	return lightweightGetAndPrint(cmd, "/api/chat/sessions/"+url.PathEscape(args[0]), "get chat session")
}
func runLightweightChatMessages(cmd *cobra.Command, args []string) error {
	return lightweightGetAndPrint(cmd, pagePath("/api/chat/sessions/"+url.PathEscape(args[0])+"/messages", cmd), "list chat messages")
}

func runLightweightChatCreate(cmd *cobra.Command, _ []string) error {
	agentID, _ := cmd.Flags().GetString("agent-id")
	title, _ := cmd.Flags().GetString("title")
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("--agent-id is required")
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PostJSON(ctx, "/api/chat/sessions", map[string]any{"agent_id": agentID, "title": title}, &result); err != nil {
		return fmt.Errorf("create chat session: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightChatUpdate(cmd *cobra.Command, args []string) error {
	body := map[string]any{}
	for _, flag := range []string{"title", "status"} {
		if cmd.Flags().Changed(flag) {
			value, _ := cmd.Flags().GetString(flag)
			body[flag] = value
		}
	}
	if len(body) == 0 {
		return fmt.Errorf("use --title or --status")
	}
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	if err := client.PatchJSON(ctx, "/api/chat/sessions/"+url.PathEscape(args[0]), body, &result); err != nil {
		return fmt.Errorf("update chat session: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightChatDelete(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSON(ctx, "/api/chat/sessions/"+url.PathEscape(args[0]))
}

func runLightweightChatSend(cmd *cobra.Command, args []string) error {
	content, _, err := resolveTextFlag(cmd, "content")
	if err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("--content or --content-stdin is required")
	}
	var result map[string]any
	if err := lightweightPost(cmd, "/api/chat/sessions/"+url.PathEscape(args[0])+"/messages", "chat", map[string]any{"content": content}, &result); err != nil {
		return fmt.Errorf("send chat message: %w", err)
	}
	return lightweightPrint(result)
}

func runLightweightDraftDelete(cmd *cobra.Command, args []string) error {
	client, err := lightweightClient(cmd, true)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	return client.DeleteJSON(ctx, "/api/chat/sessions/"+url.PathEscape(args[0])+"/draft-restores/"+url.PathEscape(args[1]))
}
