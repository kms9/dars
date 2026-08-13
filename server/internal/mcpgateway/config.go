package mcpgateway

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// BundleRoutePattern is the single public Streamable HTTP MCP route.
	BundleRoutePattern = "/bundles/{bundleId}/mcp"
	bundleRoutePrefix  = "/bundles/"
	bundleRouteSuffix  = "/mcp"

	defaultConnectTimeout      = 5 * time.Second
	defaultInvocationTimeout   = 30 * time.Second
	defaultChannelIdleTimeout  = 5 * time.Minute
	defaultMaxRequestBodyBytes = int64(1 << 20)
	defaultMaxResponseBytes    = int64(4 << 20)
	defaultMaxArtifactBytes    = int64(16 << 20)
	defaultMaxRedirects        = 3
	defaultTaskConcurrency     = 4
	defaultSourceConcurrency   = 8
)

// Config contains the process-wide safety limits used by the MCP Gateway.
// It is loaded once by the Server composition root and then treated as immutable.
type Config struct {
	PublicURL            string
	PrivateCIDRAllowlist []netip.Prefix
	AllowedPorts         []uint16
	ConnectTimeout       time.Duration
	InvocationTimeout    time.Duration
	ChannelIdleTimeout   time.Duration
	MaxRequestBodyBytes  int64
	MaxResponseBytes     int64
	MaxArtifactBytes     int64
	MaxRedirects         int
	TaskConcurrency      int
	SourceConcurrency    int
}

// ConfigFromEnv reads and validates Gateway configuration without enabling or
// disabling the Gateway. An empty public URL is accepted at process startup so
// installations without Bundle Tasks keep working; Bundle URL projection fails
// closed until a valid public URL is configured.
func ConfigFromEnv() (Config, error) {
	return configFromLookup(os.Getenv)
}

func configFromLookup(lookup func(string) string) (Config, error) {
	publicURL, err := normalizePublicURL(lookup("DARS_PUBLIC_URL"))
	if err != nil {
		return Config{}, err
	}
	privateCIDRs, err := parseCIDRs(lookup("DARS_MCP_PRIVATE_CIDR_ALLOWLIST"))
	if err != nil {
		return Config{}, err
	}
	ports, err := parsePorts(lookup("DARS_MCP_ALLOWED_PORTS"))
	if err != nil {
		return Config{}, err
	}
	connectTimeout, err := durationSetting(lookup, "DARS_MCP_CONNECT_TIMEOUT", defaultConnectTimeout)
	if err != nil {
		return Config{}, err
	}
	invocationTimeout, err := durationSetting(lookup, "DARS_MCP_INVOCATION_TIMEOUT", defaultInvocationTimeout)
	if err != nil {
		return Config{}, err
	}
	channelIdleTimeout, err := durationSetting(lookup, "DARS_MCP_CHANNEL_IDLE_TIMEOUT", defaultChannelIdleTimeout)
	if err != nil {
		return Config{}, err
	}
	maxRequestBodyBytes, err := positiveInt64Setting(lookup, "DARS_MCP_MAX_REQUEST_BODY_BYTES", defaultMaxRequestBodyBytes)
	if err != nil {
		return Config{}, err
	}
	maxResponseBytes, err := positiveInt64Setting(lookup, "DARS_MCP_MAX_RESPONSE_BYTES", defaultMaxResponseBytes)
	if err != nil {
		return Config{}, err
	}
	maxArtifactBytes, err := positiveInt64Setting(lookup, "DARS_MCP_MAX_ARTIFACT_BYTES", defaultMaxArtifactBytes)
	if err != nil {
		return Config{}, err
	}
	maxRedirects, err := nonNegativeIntSetting(lookup, "DARS_MCP_MAX_REDIRECTS", defaultMaxRedirects)
	if err != nil {
		return Config{}, err
	}
	taskConcurrency, err := positiveIntSetting(lookup, "DARS_MCP_TASK_CONCURRENCY", defaultTaskConcurrency)
	if err != nil {
		return Config{}, err
	}
	sourceConcurrency, err := positiveIntSetting(lookup, "DARS_MCP_SOURCE_CONCURRENCY", defaultSourceConcurrency)
	if err != nil {
		return Config{}, err
	}

	return Config{
		PublicURL:            publicURL,
		PrivateCIDRAllowlist: privateCIDRs,
		AllowedPorts:         ports,
		ConnectTimeout:       connectTimeout,
		InvocationTimeout:    invocationTimeout,
		ChannelIdleTimeout:   channelIdleTimeout,
		MaxRequestBodyBytes:  maxRequestBodyBytes,
		MaxResponseBytes:     maxResponseBytes,
		MaxArtifactBytes:     maxArtifactBytes,
		MaxRedirects:         maxRedirects,
		TaskConcurrency:      taskConcurrency,
		SourceConcurrency:    sourceConcurrency,
	}, nil
}

// BundleURL returns the canonical public URL for an opaque Bundle ID.
func (c Config) BundleURL(bundleID string) (string, error) {
	if c.PublicURL == "" {
		return "", fmt.Errorf("DARS_PUBLIC_URL is required for MCP Bundle projection")
	}
	if !ValidBundleID(bundleID) {
		return "", fmt.Errorf("invalid Bundle ID")
	}
	return c.PublicURL + bundleRoutePrefix + url.PathEscape(bundleID) + bundleRouteSuffix, nil
}

func normalizePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return "", fmt.Errorf("DARS_PUBLIC_URL must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("DARS_PUBLIC_URL must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("DARS_PUBLIC_URL must not contain userinfo, query, or fragment")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func parseCIDRs(raw string) ([]netip.Prefix, error) {
	parts := splitCSV(raw)
	result := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, fmt.Errorf("DARS_MCP_PRIVATE_CIDR_ALLOWLIST contains invalid CIDR %q", part)
		}
		result = append(result, prefix.Masked())
	}
	return result, nil
}

func parsePorts(raw string) ([]uint16, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return []uint16{80, 443}, nil
	}
	result := make([]uint16, 0, len(parts))
	seen := make(map[uint16]struct{}, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseUint(part, 10, 16)
		if err != nil || value == 0 {
			return nil, fmt.Errorf("DARS_MCP_ALLOWED_PORTS contains invalid port %q", part)
		}
		port := uint16(value)
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		result = append(result, port)
	}
	return result, nil
}

func splitCSV(raw string) []string {
	var result []string
	for _, part := range strings.Split(raw, ",") {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func durationSetting(lookup func(string) string, name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(lookup(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return value, nil
}

func positiveInt64Setting(lookup func(string) string, name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(lookup(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func positiveIntSetting(lookup func(string) string, name string, fallback int) (int, error) {
	value, err := positiveInt64Setting(lookup, name, int64(fallback))
	if err != nil {
		return 0, err
	}
	if int64(int(value)) != value {
		return 0, fmt.Errorf("%s exceeds the platform integer range", name)
	}
	return int(value), nil
}

func nonNegativeIntSetting(lookup func(string) string, name string, fallback int) (int, error) {
	raw := strings.TrimSpace(lookup(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return int(value), nil
}
