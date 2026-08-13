package mcpgateway

import (
	"encoding/json"
	"errors"
	"strings"
)

// ClaimProjector creates the canonical MCP Facade entry placed into the
// existing Claim agent.mcp_config field for a Bundle-pinned Task.
type ClaimProjector struct {
	config    Config
	providers map[string]bool
}

func NewClaimProjector(config Config, providerAllowlist map[string]bool) *ClaimProjector {
	return &ClaimProjector{config: config, providers: copyProviderAllowlist(providerAllowlist)}
}

func (projector *ClaimProjector) ProviderSupported(provider string) bool {
	return projector.providers[strings.ToLower(strings.TrimSpace(provider))]
}

func (projector *ClaimProjector) ProjectClaimMCPConfig(bundleID, provider, taskToken string) (json.RawMessage, error) {
	if !projector.ProviderSupported(provider) {
		return nil, errors.New("runtime provider is not compatible with the MCP Gateway")
	}
	if !strings.HasPrefix(taskToken, "dat_") {
		return nil, errors.New("invalid Task token")
	}
	endpoint, err := projector.config.BundleURL(bundleID)
	if err != nil {
		return nil, err
	}
	document := struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}{
		MCPServers: map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		}{
			"dars": {
				URL: endpoint,
				Headers: map[string]string{
					"Authorization": "Bearer " + taskToken,
				},
			},
		},
	}
	return json.Marshal(document)
}
