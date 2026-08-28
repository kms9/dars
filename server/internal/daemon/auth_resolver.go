package daemon

import (
	"fmt"
	"os"
	"strings"

	"github.com/kms9/dars/internal/cli"
)

type CredentialSource string

const (
	CredentialSourceTokenFile CredentialSource = "token_file"
	CredentialSourceEnv       CredentialSource = "environment"
	CredentialSourceProfile   CredentialSource = "profile"
)

type ResolvedCredential struct {
	Token  string
	Source CredentialSource
}

func ResolveDaemonCredential(profile string) (ResolvedCredential, error) {
	if rawPath, set := os.LookupEnv("DARS_DAEMON_TOKEN_FILE"); set {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			return ResolvedCredential{}, fmt.Errorf("DARS_DAEMON_TOKEN_FILE is set but empty")
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return ResolvedCredential{}, fmt.Errorf("read DARS_DAEMON_TOKEN_FILE: %w", err)
		}
		token := strings.TrimSpace(string(content))
		if !strings.HasPrefix(token, "ddt_") {
			return ResolvedCredential{}, fmt.Errorf("DARS_DAEMON_TOKEN_FILE must contain a ddt_ Daemon Token")
		}
		return ResolvedCredential{Token: token, Source: CredentialSourceTokenFile}, nil
	}
	if rawToken, set := os.LookupEnv("DARS_DAEMON_TOKEN"); set {
		token := strings.TrimSpace(rawToken)
		if !strings.HasPrefix(token, "ddt_") {
			return ResolvedCredential{}, fmt.Errorf("DARS_DAEMON_TOKEN must contain a ddt_ Daemon Token")
		}
		return ResolvedCredential{Token: token, Source: CredentialSourceEnv}, nil
	}
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return ResolvedCredential{}, fmt.Errorf("load CLI config: %w", err)
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		loginHint := "dars login"
		if profile != "" {
			loginHint += " --profile " + profile
		}
		return ResolvedCredential{}, fmt.Errorf("not authenticated: run '%s' first or provide DARS_DAEMON_TOKEN_FILE", loginHint)
	}
	return ResolvedCredential{Token: token, Source: CredentialSourceProfile}, nil
}

func ResolveDaemonWorkspaceID(profile string) (string, error) {
	if workspaceID := strings.TrimSpace(os.Getenv("DARS_WORKSPACE_ID")); workspaceID != "" {
		return workspaceID, nil
	}
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return "", fmt.Errorf("load CLI config: %w", err)
	}
	return strings.TrimSpace(cfg.WorkspaceID), nil
}
