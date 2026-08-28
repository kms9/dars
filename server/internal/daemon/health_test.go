package daemon

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthResponseKeepsRuntimeIDsAndAddsProviders(t *testing.T) {
	d := New(Config{DaemonID: "daemon-a"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.workspaces["workspace-a"] = newWorkspaceState("workspace-a", []string{"runtime-pi"}, "", nil, nil)
	d.runtimeIndex["runtime-pi"] = Runtime{ID: "runtime-pi", Provider: "pi"}
	d.ready.Store(true)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()
	d.healthHandler(time.Now()).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d", recorder.Code)
	}
	var response HealthResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "running" || len(response.Workspaces) != 1 {
		t.Fatalf("health response = %+v", response)
	}
	workspace := response.Workspaces[0]
	if len(workspace.Runtimes) != 1 || workspace.Runtimes[0] != "runtime-pi" {
		t.Fatalf("runtime IDs changed: %+v", workspace.Runtimes)
	}
	if workspace.RuntimeProviders["runtime-pi"] != "pi" {
		t.Fatalf("runtime providers = %+v", workspace.RuntimeProviders)
	}
}
