package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
)

var daemonTokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage Workspace Daemon Tokens",
}

func init() {
	create := &cobra.Command{Use: "create", Short: "Create or rotate a Daemon Token", Args: cobra.NoArgs, RunE: runDaemonTokenCreate}
	create.Flags().String("daemon-id", "", "Stable daemon ID (required)")
	create.Flags().String("name", "", "Human-readable token name (defaults to daemon ID)")
	create.Flags().String("expires-at", "", "Optional RFC3339 expiry, at most 365 days")
	list := &cobra.Command{Use: "list", Short: "List Daemon Token metadata", Args: cobra.NoArgs, RunE: runDaemonTokenList}
	revoke := &cobra.Command{Use: "revoke <token-id>", Short: "Revoke a Daemon Token", Args: exactArgs(1), RunE: runDaemonTokenRevoke}
	daemonTokenCmd.AddCommand(create, list, revoke)
	daemonCmd.AddCommand(daemonTokenCmd)
}

func runDaemonTokenCreate(cmd *cobra.Command, _ []string) error {
	daemonID, _ := cmd.Flags().GetString("daemon-id")
	daemonID = strings.TrimSpace(daemonID)
	if daemonID == "" {
		return fmt.Errorf("--daemon-id is required")
	}
	name, _ := cmd.Flags().GetString("name")
	name = strings.TrimSpace(name)
	if name == "" {
		name = daemonID
	}
	body := map[string]string{"daemon_id": daemonID, "name": name}
	if expiresAt, _ := cmd.Flags().GetString("expires-at"); strings.TrimSpace(expiresAt) != "" {
		body["expires_at"] = strings.TrimSpace(expiresAt)
	}
	client, workspaceID, err := lightweightWorkspaceScope(cmd, nil)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result map[string]any
	path := "/api/workspaces/" + url.PathEscape(workspaceID) + "/daemon-tokens"
	if err := client.PostJSON(ctx, path, body, &result); err != nil {
		return fmt.Errorf("create Daemon Token: %w", err)
	}
	return lightweightPrint(result)
}

func runDaemonTokenList(cmd *cobra.Command, _ []string) error {
	client, workspaceID, err := lightweightWorkspaceScope(cmd, nil)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var result []map[string]any
	path := "/api/workspaces/" + url.PathEscape(workspaceID) + "/daemon-tokens"
	if err := client.GetJSON(ctx, path, &result); err != nil {
		return fmt.Errorf("list Daemon Tokens: %w", err)
	}
	return lightweightPrint(result)
}

func runDaemonTokenRevoke(cmd *cobra.Command, args []string) error {
	client, workspaceID, err := lightweightWorkspaceScope(cmd, nil)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	path := "/api/workspaces/" + url.PathEscape(workspaceID) + "/daemon-tokens/" + url.PathEscape(args[0])
	if err := client.DeleteJSON(ctx, path); err != nil {
		return fmt.Errorf("revoke Daemon Token: %w", err)
	}
	return nil
}
