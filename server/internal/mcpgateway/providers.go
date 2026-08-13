package mcpgateway

// ProviderCompatibilityAllowlist is code-owned. A provider may be added only
// after unchanged-Daemon E3 proves it preserves the Bundle URL and
// Authorization header through a real Runtime tools/list and tools/call flow.
func ProviderCompatibilityAllowlist() map[string]bool {
	return map[string]bool{
		"codex": true,
	}
}
