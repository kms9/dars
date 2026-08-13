package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
)

var (
	setupCmd = &cobra.Command{
		Use:   "setup",
		Short: "Configure a self-hosted Lightweight runtime",
	}
	setupSelfHostCmd = &cobra.Command{
		Use:   "self-host",
		Short: "Configure the Lightweight Server and Web URLs",
		Args:  cobra.NoArgs,
		RunE:  runSetupSelfHost,
	}
)

func init() {
	setupSelfHostCmd.Flags().String("server-url", "", "Backend server URL (env: DARS_SERVER_URL; default: http://localhost:8080)")
	setupSelfHostCmd.Flags().String("app-url", "", "Frontend app URL (env: DARS_APP_URL; default: http://localhost:3000)")
	setupSelfHostCmd.Flags().Int("port", 8080, "Backend port used when --server-url is omitted")
	setupSelfHostCmd.Flags().Int("frontend-port", 3000, "Frontend port used when --app-url is omitted")
	setupCmd.AddCommand(setupSelfHostCmd)
}

func runSetupSelfHost(cmd *cobra.Command, _ []string) error {
	serverURL := strings.TrimSpace(cli.FlagOrEnv(cmd, "server-url", "DARS_SERVER_URL", ""))
	if serverURL == "" {
		port, _ := cmd.Flags().GetInt("port")
		serverURL = fmt.Sprintf("http://localhost:%d", port)
	}
	serverURL = normalizeAPIBaseURL(serverURL)
	appURL := strings.TrimSpace(cli.FlagOrEnv(cmd, "app-url", "DARS_APP_URL", ""))
	if appURL == "" {
		port, _ := cmd.Flags().GetInt("frontend-port")
		appURL = fmt.Sprintf("http://localhost:%d", port)
	}
	appURL = strings.TrimRight(appURL, "/")

	if !probeServer(serverURL) {
		return fmt.Errorf("Lightweight Server is not reachable at %s/health", serverURL)
	}
	profile := resolveProfile(cmd)
	cfg, _ := cli.LoadCLIConfigForProfile(profile)
	cfg.ServerURL = serverURL
	cfg.AppURL = appURL
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return fmt.Errorf("save CLI config: %w", err)
	}
	path, _ := cli.CLIConfigPathForProfile(profile)
	fmt.Fprintf(cmd.OutOrStdout(), "Configured Lightweight self-hosting.\nServer: %s\nWeb: %s\nConfig: %s\nNext: dars login\n", serverURL, appURL, path)
	return nil
}

func probeServer(baseURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/health", nil)
	if err != nil {
		return false
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}
