package main

import (
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/kms9/dars/internal/lightweightapi"
	"github.com/kms9/dars/internal/mcpgateway"
)

func TestDaemonTokenRoutesAreDedicatedAdminContracts(t *testing.T) {
	expected := map[string]bool{
		"GET /api/workspaces/{workspaceId}/daemon-tokens":              false,
		"POST /api/workspaces/{workspaceId}/daemon-tokens":             true,
		"DELETE /api/workspaces/{workspaceId}/daemon-tokens/{tokenId}": false,
	}
	manifest := lightweightRouteManifest(nil, nil, nil, &lightweightapi.Handler{}, &mcpgateway.ControlPlane{})
	seen := make(map[string]bool, len(expected))
	for _, route := range manifest {
		key := route.Method + " " + route.Path
		mutation, ok := expected[key]
		if !ok {
			continue
		}
		seen[key] = true
		if route.Boundary != boundaryWorkspaceAdmin {
			t.Errorf("%s boundary = %v, want Workspace Admin", key, route.Boundary)
		}
		_, strict := lightweightMutationSchemas[key]
		if strict != mutation {
			t.Errorf("%s strict schema = %v, want %v", key, strict, mutation)
		}
	}
	for key := range expected {
		if !seen[key] {
			t.Errorf("missing Daemon Token route %s", key)
		}
	}
	if len(manifest) != 145 {
		t.Errorf("route manifest count = %d, want 145", len(manifest))
	}
}

func TestLightweightRouterMatchesFrozenManifest(t *testing.T) {
	manifest := lightweightRouteManifest(nil, nil, nil, &lightweightapi.Handler{}, &mcpgateway.ControlPlane{})
	actual := make([]string, 0, len(manifest))
	for _, route := range manifest {
		actual = append(actual, route.Method+" "+route.Path)
	}
	sort.Strings(actual)

	fixturePath := filepath.Join(serverModuleRoot(t), "..", "openspec", "contracts", "routes.txt")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if !slices.Equal(actual, expected) {
		t.Fatalf("route manifest mismatch\nactual:\n%s\nexpected:\n%s", strings.Join(actual, "\n"), strings.Join(expected, "\n"))
	}
}
