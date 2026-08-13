package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
)

// tryResolveAppURL returns the app URL if configured, or "" if not available.
// Unlike resolveAppURL, it never calls os.Exit.
func tryResolveAppURL(cmd *cobra.Command) string {
	for _, key := range []string{"DARS_APP_URL", "FRONTEND_ORIGIN"} {
		if val := strings.TrimSpace(os.Getenv(key)); val != "" {
			return strings.TrimRight(val, "/")
		}
	}
	profile := resolveProfile(cmd)
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err == nil && cfg.AppURL != "" {
		return strings.TrimRight(cfg.AppURL, "/")
	}
	return ""
}

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with email code or an existing PAT",
	Long:  "Authenticate to the Lightweight Server with an email verification code, then issue and store a scoped PAT.",
	// Up to one positional is accepted so `--token dpat_...`
	// (space form) can recover the token in runAuthLogin even though pflag
	// won't bind it.
	Args: cobra.MaximumNArgs(1),
	RunE: runLogin,
}

// tokenPromptSentinel is the value pflag assigns to `--token` when the flag
// is supplied without an explicit value. runAuthLoginToken treats it as
// "prompt me interactively", preserving the legacy `dars login --token`
// no-value form alongside the documented `--token dpat_...` / `--token dcn_...`
// value form.
//
// The sentinel must be printable: pflag renders NoOptDefVal verbatim in help
// output ("--token string[=\"prompt\"]") and uses "\x00" internally as its
// column-alignment marker, so a NUL-prefixed sentinel corrupts `login -h`.
// Colliding with a user literally typing `--token prompt` is harmless — that
// string is never a valid PAT, and prompting is a reasonable response.
const tokenPromptSentinel = "prompt"

func init() {
	// No backticks in the usage string: pflag's UnquoteUsage treats the first
	// backquoted segment as the flag's value placeholder in help output.
	loginCmd.Flags().String("token", "", "Authenticate using an existing dpat_ personal access token")
	// NoOptDefVal lets `--token` (no value) keep its old prompt-mode behavior
	// while `--token dpat_...` and the `=value` form
	// consume the value normally.
	loginCmd.Flags().Lookup("token").NoOptDefVal = tokenPromptSentinel
	loginCmd.Flags().String("email", "", "Email address for verification-code login")
	loginCmd.Flags().String("code", "", "Six-digit verification code (prompted when omitted)")
}

func runLogin(cmd *cobra.Command, args []string) error {
	var err error
	if cmd.Flags().Changed("token") {
		token, _ := cmd.Flags().GetString("token")
		if token == tokenPromptSentinel && len(args) == 1 {
			token = args[0]
		}
		err = runAuthLoginToken(cmd, token)
	} else {
		err = runLightweightEmailLogin(cmd)
	}
	if err != nil {
		return err
	}

	// Auto-discover and select a default workspace.
	if err := autoWatchWorkspaces(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "\nCould not auto-configure workspaces: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run 'dars workspace list' and 'dars workspace switch <id>' to set up manually.\n")
		return nil
	}

	fmt.Fprintf(os.Stderr, "\n→ Run 'dars daemon start' to start your local agent runtime.\n")
	return nil
}

func runLightweightEmailLogin(cmd *cobra.Command) error {
	email, _ := cmd.Flags().GetString("email")
	email = strings.ToLower(strings.TrimSpace(email))
	reader := bufio.NewReader(cmd.InOrStdin())
	if email == "" {
		fmt.Fprint(cmd.ErrOrStderr(), "Email: ")
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return fmt.Errorf("read email: %w", err)
		}
		email = strings.ToLower(strings.TrimSpace(line))
	}
	if email == "" {
		return fmt.Errorf("email is required")
	}

	serverURL := resolveLoginTokenServerURL(cmd)
	if serverURL == "" {
		return fmt.Errorf("server URL not set: run 'dars setup self-host' first")
	}
	client := cli.NewAPIClient(serverURL, "", "")
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	if err := client.PostJSON(ctx, "/auth/send-code", map[string]string{"email": email}, nil); err != nil {
		return fmt.Errorf("send verification code: %w", err)
	}

	code, _ := cmd.Flags().GetString("code")
	code = strings.TrimSpace(code)
	if code == "" {
		fmt.Fprint(cmd.ErrOrStderr(), "Verification code: ")
		line, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(line) == "" {
			return fmt.Errorf("read verification code: %w", err)
		}
		code = strings.TrimSpace(line)
	}
	if len(code) != 6 {
		return fmt.Errorf("verification code must contain six digits")
	}

	var login struct {
		Token string `json:"token"`
		User  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := client.PostJSON(ctx, "/auth/verify-code", map[string]string{"email": email, "code": code}, &login); err != nil {
		return fmt.Errorf("verify email code: %w", err)
	}
	jwtClient := cli.NewAPIClient(serverURL, "", login.Token)
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	var pat struct {
		Token string `json:"token"`
	}
	if err := jwtClient.PostJSON(ctx, "/api/tokens", map[string]any{"name": "CLI (" + hostname + ")"}, &pat); err != nil {
		return fmt.Errorf("issue CLI access token: %w", err)
	}
	if !strings.HasPrefix(pat.Token, "dpat_") {
		return fmt.Errorf("server returned an invalid personal access token")
	}

	profile := resolveProfile(cmd)
	cfg, _ := cli.LoadCLIConfigForProfile(profile)
	cfg.WorkspaceID = ""
	cfg.Token = pat.Token
	cfg.ServerURL = serverURL
	if appURL := tryResolveAppURL(cmd); appURL != "" {
		cfg.AppURL = appURL
	}
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return fmt.Errorf("save CLI config: %w", err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Authenticated as %s (%s)\nToken saved to config.\n", login.User.Name, login.User.Email)
	return nil
}

func autoWatchWorkspaces(cmd *cobra.Command) error {
	serverURL := resolveServerURL(cmd)
	token := resolveToken(cmd)
	if token == "" {
		return fmt.Errorf("not authenticated")
	}

	client := cli.NewAPIClient(serverURL, "", token)
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var workspaces []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := client.GetJSON(ctx, "/api/workspaces", &workspaces); err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}

	if len(workspaces) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "\nNo workspaces found. Create one with 'dars workspace create --name <name>'.")
		return nil
	}

	profile := resolveProfile(cmd)
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return err
	}

	// Set default workspace if not set.
	if cfg.WorkspaceID == "" {
		cfg.WorkspaceID = workspaces[0].ID
	}

	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return err
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "\nFound %d workspace(s):\n", len(workspaces))
	for _, ws := range workspaces {
		marker := "  "
		if ws.ID == cfg.WorkspaceID {
			marker = "* "
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "%s%s (%s)\n", marker, ws.Name, ws.ID)
	}
	if len(workspaces) > 1 {
		fmt.Fprintln(cmd.ErrOrStderr(), "\nUse 'dars workspace switch <id|slug>' to change the default workspace.")
	}

	return nil
}
