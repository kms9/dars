package mcpgateway

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaimProjectorProducesGatewayOnlyDocument(t *testing.T) {
	projector := NewClaimProjector(Config{PublicURL: "https://dars.example/base"}, map[string]bool{"codex": true})
	document, err := projector.ProjectClaimMCPConfig("tb_01ABCDEFG", "CODEX", "dat_current-token")
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.MCPServers) != 1 {
		t.Fatalf("mcpServers = %+v, want exactly one entry", decoded.MCPServers)
	}
	server := decoded.MCPServers["dars"]
	if server.URL != "https://dars.example/base/bundles/tb_01ABCDEFG/mcp" {
		t.Fatalf("url = %q", server.URL)
	}
	if server.Headers["Authorization"] != "Bearer dat_current-token" {
		t.Fatalf("authorization = %q", server.Headers["Authorization"])
	}
	if strings.Contains(string(document), "existing") {
		t.Fatalf("projection leaked another MCP entry: %s", document)
	}
}

func TestClaimProjectorFailsClosed(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		providers map[string]bool
		bundleID  string
		provider  string
		token     string
	}{
		{name: "missing public URL", config: Config{}, providers: map[string]bool{"codex": true}, bundleID: "tb_01ABCDEFG", provider: "codex", token: "dat_token"},
		{name: "unsupported provider", config: Config{PublicURL: "https://dars.example"}, providers: map[string]bool{}, bundleID: "tb_01ABCDEFG", provider: "codex", token: "dat_token"},
		{name: "invalid token", config: Config{PublicURL: "https://dars.example"}, providers: map[string]bool{"codex": true}, bundleID: "tb_01ABCDEFG", provider: "codex", token: "not-a-task-token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projector := NewClaimProjector(test.config, test.providers)
			if _, err := projector.ProjectClaimMCPConfig(test.bundleID, test.provider, test.token); err == nil {
				t.Fatal("projection succeeded, want fail-closed error")
			}
		})
	}
}

func TestShippingProviderAllowlistContainsOnlyE3ProvenProviders(t *testing.T) {
	providers := ProviderCompatibilityAllowlist()
	if len(providers) != 1 || !providers["codex"] {
		t.Fatalf("shipping providers = %+v, want only the E3-proven codex runtime", providers)
	}
	providers["unproven"] = true
	if shipping := ProviderCompatibilityAllowlist(); len(shipping) != 1 || !shipping["codex"] {
		t.Fatal("caller mutated the code-owned Provider allowlist")
	}
}
