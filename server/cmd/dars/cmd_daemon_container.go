package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
	"github.com/kms9/dars/internal/daemon"
)

var daemonAuthPreflightCmd = &cobra.Command{
	Use:    "auth-preflight",
	Short:  "Validate container Daemon Token and workspace scope",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runDaemonAuthPreflight,
}

var daemonContainerHealthCmd = &cobra.Command{
	Use:    "container-health",
	Short:  "Check that the configured Pi runtime is registered and ready",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE:   runDaemonContainerHealth,
}

func init() {
	daemonCmd.AddCommand(daemonAuthPreflightCmd, daemonContainerHealthCmd)
}

func runDaemonAuthPreflight(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	credential, err := daemon.ResolveDaemonCredential(profile)
	if err != nil {
		return fmt.Errorf("daemon auth preflight: %w", err)
	}
	if credential.Source == daemon.CredentialSourceProfile || !strings.HasPrefix(credential.Token, "ddt_") {
		return fmt.Errorf("daemon auth preflight requires DARS_DAEMON_TOKEN_FILE or DARS_DAEMON_TOKEN with a ddt_ token")
	}
	workspaceID := resolveWorkspaceID(cmd)
	if workspaceID == "" {
		return fmt.Errorf("daemon auth preflight requires DARS_WORKSPACE_ID")
	}
	rawURL := tryResolveServerURL(cmd)
	if rawURL == "" {
		return fmt.Errorf("daemon auth preflight requires DARS_SERVER_URL")
	}
	baseURL, err := daemon.NormalizeServerBaseURL(rawURL)
	if err != nil {
		return fmt.Errorf("daemon auth preflight: invalid DARS_SERVER_URL: %w", err)
	}
	client := daemon.NewClient(baseURL)
	client.SetToken(credential.Token)
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	workspaces, err := client.ListWorkspaces(ctx)
	if err != nil {
		return fmt.Errorf("daemon auth preflight: %w", err)
	}
	if err := validateDaemonWorkspaceScope(workspaces, workspaceID); err != nil {
		return fmt.Errorf("daemon auth preflight: %w", err)
	}
	return cli.PrintJSON(cmd.OutOrStdout(), map[string]string{
		"status": "ok", "workspace_id": workspaceID, "credential_source": string(credential.Source),
	})
}

func validateDaemonWorkspaceScope(workspaces []daemon.WorkspaceInfo, workspaceID string) error {
	if len(workspaces) != 1 || strings.TrimSpace(workspaces[0].ID) != strings.TrimSpace(workspaceID) {
		return fmt.Errorf("Daemon Token workspace scope does not match DARS_WORKSPACE_ID")
	}
	return nil
}

func runDaemonContainerHealth(cmd *cobra.Command, _ []string) error {
	workspaceID := resolveWorkspaceID(cmd)
	if workspaceID == "" {
		return fmt.Errorf("container health requires DARS_WORKSPACE_ID")
	}
	health := checkDaemonHealthOnPort(cmd.Context(), healthPortForProfile(resolveProfile(cmd)))
	return validateContainerHealth(health, workspaceID)
}

func validateContainerHealth(health map[string]any, workspaceID string) error {
	if status, _ := health["status"].(string); status != "running" {
		return fmt.Errorf("daemon is not running")
	}
	workspaces, ok := health["workspaces"].([]any)
	if !ok {
		return fmt.Errorf("daemon health has no workspaces")
	}
	for _, raw := range workspaces {
		workspace, ok := raw.(map[string]any)
		if !ok || workspace["id"] != workspaceID {
			continue
		}
		runtimes, _ := workspace["runtimes"].([]any)
		providers, _ := workspace["runtime_providers"].(map[string]any)
		for _, rawID := range runtimes {
			runtimeID, _ := rawID.(string)
			provider, _ := providers[runtimeID].(string)
			if strings.EqualFold(provider, "pi") {
				return nil
			}
		}
		return fmt.Errorf("workspace has no registered Pi runtime")
	}
	return fmt.Errorf("configured workspace is not registered")
}
