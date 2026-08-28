package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kms9/dars/internal/cli"
)

func TestResolveDaemonCredentialPrecedenceAndFailClosedFile(t *testing.T) {
	unsetTestEnv(t, "DARS_DAEMON_TOKEN_FILE")
	unsetTestEnv(t, "DARS_DAEMON_TOKEN")
	tokenFile := filepath.Join(t.TempDir(), "daemon-token")
	if err := os.WriteFile(tokenFile, []byte("ddt_file_value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARS_DAEMON_TOKEN_FILE", tokenFile)
	t.Setenv("DARS_DAEMON_TOKEN", "ddt_environment_value")
	credential, err := ResolveDaemonCredential("")
	if err != nil {
		t.Fatal(err)
	}
	if credential.Token != "ddt_file_value" || credential.Source != CredentialSourceTokenFile {
		t.Fatalf("credential = %+v", credential)
	}

	if err := os.WriteFile(tokenFile, []byte("not-a-daemon-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ResolveDaemonCredential("")
	if err == nil || !strings.Contains(err.Error(), "must contain a ddt_") {
		t.Fatalf("invalid authoritative file error = %v", err)
	}
	if strings.Contains(err.Error(), "not-a-daemon-token") || strings.Contains(err.Error(), "ddt_environment_value") {
		t.Fatalf("credential leaked in error: %v", err)
	}
}

func TestResolveDaemonCredentialEnvironmentAndProfileFallback(t *testing.T) {
	unsetTestEnv(t, "DARS_DAEMON_TOKEN_FILE")
	unsetTestEnv(t, "DARS_DAEMON_TOKEN")
	t.Setenv("HOME", t.TempDir())
	if err := cli.SaveCLIConfigForProfile(cli.CLIConfig{
		Token: "dpat_profile_value", WorkspaceID: "profile-workspace",
	}, ""); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DARS_DAEMON_TOKEN", "ddt_environment_value")
	credential, err := ResolveDaemonCredential("")
	if err != nil {
		t.Fatal(err)
	}
	if credential.Token != "ddt_environment_value" || credential.Source != CredentialSourceEnv {
		t.Fatalf("environment credential = %+v", credential)
	}

	t.Setenv("DARS_DAEMON_TOKEN", "dpat_wrong_type")
	if _, err := ResolveDaemonCredential(""); err == nil {
		t.Fatal("invalid environment credential fell back to profile")
	}
	if err := os.Unsetenv("DARS_DAEMON_TOKEN"); err != nil {
		t.Fatal(err)
	}
	credential, err = ResolveDaemonCredential("")
	if err != nil {
		t.Fatal(err)
	}
	if credential.Token != "dpat_profile_value" || credential.Source != CredentialSourceProfile {
		t.Fatalf("profile credential = %+v", credential)
	}
}

func TestResolveDaemonWorkspaceIDEnvironmentOverridesProfile(t *testing.T) {
	unsetTestEnv(t, "DARS_WORKSPACE_ID")
	t.Setenv("HOME", t.TempDir())
	if err := cli.SaveCLIConfigForProfile(cli.CLIConfig{WorkspaceID: "profile-workspace"}, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DARS_WORKSPACE_ID", "environment-workspace")
	workspaceID, err := ResolveDaemonWorkspaceID("")
	if err != nil {
		t.Fatal(err)
	}
	if workspaceID != "environment-workspace" {
		t.Fatalf("workspace ID = %q", workspaceID)
	}
}

func TestClientSetDaemonTokenClearsStalePairingToken(t *testing.T) {
	client := NewClient("http://127.0.0.1")
	client.SetToken("dpat_human")
	client.SetToken("ddt_machine")
	if got := client.PairingToken(); got != "ddt_machine" {
		t.Fatalf("PairingToken = %q, want current Daemon Token", got)
	}
}

func unsetTestEnv(t *testing.T, name string) {
	t.Helper()
	value, set := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if set {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}
