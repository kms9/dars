package main

import (
	"log/slog"
	"net/netip"
	"os"
	"strings"
)

var defaultOrigins = []string{
	"http://localhost:3000",
}

var corsAllowedHeaders = []string{
	"Accept",
	"Authorization",
	"Content-Type",
	"Idempotency-Key",
	"X-Workspace-ID",
	"X-Request-ID",
	"X-CSRF-Token",
	"X-Client-Platform",
	"X-Client-Version",
	"X-Client-OS",
	"X-Client-Capabilities",
}

func allowedOrigins() []string {
	raw := strings.TrimSpace(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN"))
	}
	if raw == "" {
		return defaultOrigins
	}
	origins := splitAndTrim(raw)
	if len(origins) == 0 {
		return defaultOrigins
	}
	return origins
}

func parseTrustedProxies(raw string) []netip.Prefix {
	var prefixes []netip.Prefix
	for _, value := range splitAndTrim(raw) {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			slog.Warn("DARS_TRUSTED_PROXIES: ignoring invalid CIDR", "value", value, "error", err)
			continue
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}

func normalizeServerVersion(value string) string {
	if value == "dev" {
		return ""
	}
	return value
}

func splitAndTrim(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
