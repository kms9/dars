package main

import (
	"testing"

	"github.com/kms9/dars/internal/lightweightapi"
	"github.com/kms9/dars/internal/mcpgateway"
)

func TestToolControlRoutesAreAdminOnlyAndStrict(t *testing.T) {
	expected := map[string]bool{
		"GET /api/tool-sources":                       false,
		"POST /api/tool-sources":                      true,
		"POST /api/tool-sources/import":               true,
		"GET /api/tool-sources/{sourceId}":            false,
		"PUT /api/tool-sources/{sourceId}":            true,
		"DELETE /api/tool-sources/{sourceId}":         false,
		"POST /api/tool-sources/{sourceId}/validate":  true,
		"POST /api/tool-sources/{sourceId}/enable":    true,
		"POST /api/tool-sources/{sourceId}/disable":   true,
		"GET /api/tool-sources/{sourceId}/tools":      false,
		"POST /api/tool-sources/{sourceId}/artifacts": true,
		"GET /api/tool-bundles/{bundleId}":            false,
		"POST /api/tool-bundles/{bundleId}/revoke":    true,
		"GET /api/agents/{agentId}/tool-bundle":       false,
		"PUT /api/agents/{agentId}/tool-bundle":       true,
		"DELETE /api/agents/{agentId}/tool-bundle":    false,
	}
	manifest := lightweightRouteManifest(nil, nil, nil, &lightweightapi.Handler{}, &mcpgateway.ControlPlane{})
	seen := make(map[string]struct{}, len(expected))
	for _, route := range manifest {
		if route.Path == mcpgateway.BundleRoutePattern {
			t.Fatal("MCP data plane must not be part of the REST control-plane manifest")
		}
		key := route.Method + " " + route.Path
		mutation, ok := expected[key]
		if !ok {
			continue
		}
		seen[key] = struct{}{}
		if route.Boundary != boundaryWorkspaceAdmin {
			t.Errorf("%s boundary = %v, want Workspace Admin", key, route.Boundary)
		}
		_, hasSchema := lightweightMutationSchemas[key]
		if mutation != hasSchema {
			t.Errorf("%s strict schema = %v, want %v", key, hasSchema, mutation)
		}
	}
	for key := range expected {
		if _, ok := seen[key]; !ok {
			t.Errorf("missing control-plane route %s", key)
		}
	}
}
