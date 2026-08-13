package mcpgateway

import (
	"reflect"
	"testing"
	"time"
)

func TestConfigFromLookupDefaults(t *testing.T) {
	cfg, err := configFromLookup(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "" {
		t.Fatalf("PublicURL = %q, want empty", cfg.PublicURL)
	}
	if !reflect.DeepEqual(cfg.AllowedPorts, []uint16{80, 443}) {
		t.Fatalf("AllowedPorts = %v", cfg.AllowedPorts)
	}
	if cfg.ConnectTimeout != defaultConnectTimeout || cfg.InvocationTimeout != defaultInvocationTimeout || cfg.ChannelIdleTimeout != defaultChannelIdleTimeout {
		t.Fatalf("unexpected timeout defaults: %+v", cfg)
	}
	if cfg.MaxRequestBodyBytes != defaultMaxRequestBodyBytes || cfg.TaskConcurrency != defaultTaskConcurrency {
		t.Fatalf("unexpected limit defaults: %+v", cfg)
	}
}

func TestConfigFromLookupOverrides(t *testing.T) {
	values := map[string]string{
		"DARS_PUBLIC_URL":                 "https://gateway.example.test/base/",
		"DARS_MCP_PRIVATE_CIDR_ALLOWLIST": "10.0.0.0/8, 100.64.0.0/10",
		"DARS_MCP_ALLOWED_PORTS":          "443,8443,443",
		"DARS_MCP_CONNECT_TIMEOUT":        "2s",
		"DARS_MCP_INVOCATION_TIMEOUT":     "45s",
		"DARS_MCP_CHANNEL_IDLE_TIMEOUT":   "90s",
		"DARS_MCP_MAX_REQUEST_BODY_BYTES": "1024",
		"DARS_MCP_MAX_RESPONSE_BYTES":     "2048",
		"DARS_MCP_MAX_ARTIFACT_BYTES":     "4096",
		"DARS_MCP_MAX_REDIRECTS":          "0",
		"DARS_MCP_TASK_CONCURRENCY":       "2",
		"DARS_MCP_SOURCE_CONCURRENCY":     "3",
	}
	cfg, err := configFromLookup(func(name string) string { return values[name] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://gateway.example.test/base" {
		t.Fatalf("PublicURL = %q", cfg.PublicURL)
	}
	if !reflect.DeepEqual(cfg.AllowedPorts, []uint16{443, 8443}) {
		t.Fatalf("AllowedPorts = %v", cfg.AllowedPorts)
	}
	if cfg.ConnectTimeout != 2*time.Second || cfg.InvocationTimeout != 45*time.Second || cfg.ChannelIdleTimeout != 90*time.Second {
		t.Fatalf("unexpected timeouts: %+v", cfg)
	}
	if cfg.MaxRedirects != 0 || cfg.TaskConcurrency != 2 || cfg.SourceConcurrency != 3 {
		t.Fatalf("unexpected limits: %+v", cfg)
	}
	url, err := cfg.BundleURL("tb_01ABCDEFG")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://gateway.example.test/base/bundles/tb_01ABCDEFG/mcp" {
		t.Fatalf("BundleURL = %q", url)
	}
}

func TestConfigFromLookupRejectsInvalidSettings(t *testing.T) {
	tests := map[string]map[string]string{
		"relative public URL": {"DARS_PUBLIC_URL": "gateway.test"},
		"URL credentials":     {"DARS_PUBLIC_URL": "https://user:pass@gateway.test"},
		"invalid CIDR":        {"DARS_MCP_PRIVATE_CIDR_ALLOWLIST": "10.0.0.0/99"},
		"invalid port":        {"DARS_MCP_ALLOWED_PORTS": "0"},
		"zero timeout":        {"DARS_MCP_CONNECT_TIMEOUT": "0s"},
		"zero channel idle":   {"DARS_MCP_CHANNEL_IDLE_TIMEOUT": "0s"},
		"negative body":       {"DARS_MCP_MAX_REQUEST_BODY_BYTES": "-1"},
		"negative redirects":  {"DARS_MCP_MAX_REDIRECTS": "-1"},
		"zero concurrency":    {"DARS_MCP_TASK_CONCURRENCY": "0"},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := configFromLookup(func(key string) string { return values[key] }); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestBundleURLFailsClosedWithoutPublicURL(t *testing.T) {
	if _, err := (Config{}).BundleURL("tb_01ABCDEFG"); err == nil {
		t.Fatal("expected missing public URL error")
	}
}
