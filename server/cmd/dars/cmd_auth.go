package main

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
)

const (
	defaultCloudServerURL = "https://api.dars.ai"
	defaultCloudAppURL    = "https://dars.ai"
	callbackHostFlag      = "callback-host"
)

var (
	authCmd       = &cobra.Command{Use: "auth", Short: "Inspect or clear Lightweight authentication"}
	authStatusCmd = &cobra.Command{Use: "status", Short: "Show current authentication status", Args: cobra.NoArgs, RunE: runAuthStatus}
	authLogoutCmd = &cobra.Command{Use: "logout", Short: "Remove the stored authentication token", Args: cobra.NoArgs, RunE: runAuthLogout}
)

func init() {
	authCmd.AddCommand(authStatusCmd, authLogoutCmd)
}

func validateLoginTokenPrefix(token string) error {
	if strings.HasPrefix(token, "dpat_") {
		return nil
	}
	return fmt.Errorf("invalid token format: must start with dpat_")
}

func normalizeAPIBaseURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return raw
	}
	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	case "http", "https":
	default:
		return raw
	}
	if parsed.Path == "/ws" {
		parsed.Path = ""
	}
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

func runAuthLoginToken(cmd *cobra.Command, providedToken string) error {
	if providedToken == tokenPromptSentinel {
		providedToken = ""
	}
	token := strings.TrimSpace(providedToken)
	if token == "" {
		fmt.Fprint(cmd.ErrOrStderr(), "Enter your personal access token: ")
		scanner := bufio.NewScanner(cmd.InOrStdin())
		if !scanner.Scan() {
			return fmt.Errorf("token is required")
		}
		token = strings.TrimSpace(scanner.Text())
	}
	if err := validateLoginTokenPrefix(token); err != nil {
		return err
	}
	serverURL := resolveLoginTokenServerURL(cmd)
	if serverURL == "" {
		return fmt.Errorf("server URL not set: run 'dars setup self-host' first")
	}
	client := cli.NewAPIClient(serverURL, "", token)
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var me struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := client.GetJSON(ctx, "/api/me", &me); err != nil {
		return cli.WithUserMessage("Could not sign in with that token; verify that it is valid and unexpired.", err)
	}
	profile := resolveProfile(cmd)
	cfg, _ := cli.LoadCLIConfigForProfile(profile)
	cfg.ServerURL = serverURL
	cfg.WorkspaceID = ""
	cfg.Token = token
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return fmt.Errorf("save CLI config: %w", err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Authenticated as %s (%s)\nToken saved to config.\n", me.Name, me.Email)
	return nil
}

func runAuthStatus(cmd *cobra.Command, _ []string) error {
	token := resolveToken(cmd)
	if token == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "Not authenticated. Run 'dars login' to authenticate.")
		return nil
	}
	serverURL := resolveServerURL(cmd)
	if serverURL == "" {
		return fmt.Errorf("server URL not set")
	}
	client := cli.NewAPIClient(serverURL, "", token)
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var me struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := client.GetJSON(ctx, "/api/me", &me); err != nil {
		return fmt.Errorf("validate token: %w", err)
	}
	prefix := token
	if len(prefix) > 12 {
		prefix = prefix[:12] + "..."
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Server: %s\nUser: %s (%s)\nToken: %s\n", serverURL, me.Name, me.Email, prefix)
	return nil
}

func runAuthLogout(cmd *cobra.Command, _ []string) error {
	profile := resolveProfile(cmd)
	cfg, _ := cli.LoadCLIConfigForProfile(profile)
	if cfg.Token == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "Not authenticated.")
		return nil
	}
	cfg.Token = ""
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return fmt.Errorf("save CLI config: %w", err)
	}
	fmt.Fprintln(cmd.ErrOrStderr(), "Token removed. You are now logged out.")
	return nil
}
