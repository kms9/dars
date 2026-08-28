package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kms9/dars/internal/cli"
	"github.com/kms9/dars/internal/daemon"
)

func TestValidateDaemonWorkspaceScope(t *testing.T) {
	if err := validateDaemonWorkspaceScope([]daemon.WorkspaceInfo{{ID: "workspace-a"}}, "workspace-a"); err != nil {
		t.Fatal(err)
	}
	for _, workspaces := range [][]daemon.WorkspaceInfo{
		nil,
		{{ID: "workspace-b"}},
		{{ID: "workspace-a"}, {ID: "workspace-b"}},
	} {
		if err := validateDaemonWorkspaceScope(workspaces, "workspace-a"); err == nil {
			t.Fatalf("workspace scope accepted: %+v", workspaces)
		}
	}
}

func TestDaemonTokenCommandsUseDedicatedWorkspaceRoutes(t *testing.T) {
	previousToken, hadToken := os.LookupEnv("DARS_TOKEN")
	_ = os.Unsetenv("DARS_TOKEN")
	t.Cleanup(func() {
		if hadToken {
			_ = os.Setenv("DARS_TOKEN", previousToken)
		} else {
			_ = os.Unsetenv("DARS_TOKEN")
		}
	})
	t.Setenv("HOME", t.TempDir())
	if err := cli.SaveCLIConfigForProfile(cli.CLIConfig{Token: "dpat_control"}, ""); err != nil {
		t.Fatal(err)
	}
	workspaceID := "10000000-0000-0000-0000-000000000001"
	tokenID := "20000000-0000-0000-0000-000000000001"
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Authorization") != "Bearer dpat_control" || r.Header.Get("X-Workspace-ID") != workspaceID {
			t.Errorf("headers = Authorization %q Workspace %q", r.Header.Get("Authorization"), r.Header.Get("X-Workspace-ID"))
		}
		switch requests {
		case 1:
			if r.Method != http.MethodPost || r.URL.Path != "/api/workspaces/"+workspaceID+"/daemon-tokens" {
				t.Errorf("create route = %s %s", r.Method, r.URL.Path)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body["daemon_id"] != "daemon-a" || body["name"] != "Pi node" {
				t.Errorf("create body = %+v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"` + tokenID + `","token":"ddt_once"}`))
		case 2:
			if r.Method != http.MethodGet || r.URL.Path != "/api/workspaces/"+workspaceID+"/daemon-tokens" {
				t.Errorf("list route = %s %s", r.Method, r.URL.Path)
			}
			_, _ = w.Write([]byte(`[]`))
		case 3:
			if r.Method != http.MethodDelete || r.URL.Path != "/api/workspaces/"+workspaceID+"/daemon-tokens/"+tokenID {
				t.Errorf("revoke route = %s %s", r.Method, r.URL.Path)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %d", requests)
		}
	}))
	defer server.Close()

	create := daemonTokenTestCommand(server.URL, workspaceID)
	create.Flags().String("daemon-id", "daemon-a", "")
	create.Flags().String("name", "Pi node", "")
	create.Flags().String("expires-at", "", "")
	if err := runDaemonTokenCreate(create, nil); err != nil {
		t.Fatal(err)
	}
	if err := runDaemonTokenList(daemonTokenTestCommand(server.URL, workspaceID), nil); err != nil {
		t.Fatal(err)
	}
	if err := runDaemonTokenRevoke(daemonTokenTestCommand(server.URL, workspaceID), []string{tokenID}); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("request count = %d", requests)
	}
}

func TestDaemonAuthPreflightUsesReadOnlyDaemonWorkspaceRoute(t *testing.T) {
	unsetCommandTestEnv(t, "DARS_DAEMON_TOKEN_FILE")
	t.Setenv("DARS_DAEMON_TOKEN", "ddt_preflight")
	workspaceID := "10000000-0000-0000-0000-000000000002"
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodGet || r.URL.Path != "/api/daemon/workspaces" {
			t.Errorf("preflight route = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer ddt_preflight" {
			t.Errorf("preflight auth = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`[{"id":"` + workspaceID + `","name":"Pi"}]`))
	}))
	defer server.Close()
	cmd := daemonTokenTestCommand(server.URL, workspaceID)
	if err := runDaemonAuthPreflight(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("preflight request count = %d", requests)
	}
}

func daemonTokenTestCommand(serverURL, workspaceID string) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("server-url", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("profile", "", "")
	_ = cmd.Flags().Set("server-url", serverURL)
	_ = cmd.Flags().Set("workspace-id", workspaceID)
	return cmd
}

func unsetCommandTestEnv(t *testing.T, name string) {
	t.Helper()
	value, set := os.LookupEnv(name)
	_ = os.Unsetenv(name)
	t.Cleanup(func() {
		if set {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func TestValidateContainerHealthRequiresRunningPiRuntime(t *testing.T) {
	healthy := map[string]any{
		"status": "running",
		"workspaces": []any{map[string]any{
			"id":                "workspace-a",
			"runtimes":          []any{"runtime-pi"},
			"runtime_providers": map[string]any{"runtime-pi": "pi"},
		}},
	}
	if err := validateContainerHealth(healthy, "workspace-a"); err != nil {
		t.Fatal(err)
	}
	tests := []map[string]any{
		{"status": "starting", "workspaces": healthy["workspaces"]},
		{"status": "running", "workspaces": []any{}},
		{"status": "running", "workspaces": []any{map[string]any{
			"id": "workspace-a", "runtimes": []any{"runtime-codex"},
			"runtime_providers": map[string]any{"runtime-codex": "codex"},
		}}},
		{"status": "running", "workspaces": []any{map[string]any{
			"id": "workspace-a", "runtimes": []any{"runtime-pi"},
		}}},
	}
	for _, health := range tests {
		if err := validateContainerHealth(health, "workspace-a"); err == nil {
			t.Fatalf("invalid health accepted: %+v", health)
		}
	}
}
