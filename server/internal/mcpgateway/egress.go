package mcpgateway

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrEgressDenied   = errors.New("egress policy denied the target")
	ErrEgressResolve  = errors.New("egress target resolution failed")
	ErrEgressConnect  = errors.New("egress target connection failed")
	ErrResponseTooBig = errors.New("upstream response exceeded the configured limit")
)

type netIPResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type contextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// EgressPolicyOptions supports deterministic policy tests while production
// uses the process resolver and a bounded net.Dialer.
type EgressPolicyOptions struct {
	Resolver netIPResolver
	Dialer   contextDialer
}

// EgressPolicy is the shared connect-time authority for HTTP, gRPC and remote
// MCP traffic. Validation results are advisory only: DialContext resolves and
// checks every address again immediately before each connection.
type EgressPolicy struct {
	config   Config
	resolver netIPResolver
	dialer   contextDialer
}

// GRPCEndpoint is a validated gRPC target. Target retains the original host
// name so TLS verifies that identity even though DialContext pins an IP.
type GRPCEndpoint struct {
	Target     string
	ServerName string
	TLS        bool
}

func NewEgressPolicy(config Config, options EgressPolicyOptions) *EgressPolicy {
	resolver := options.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := options.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: config.ConnectTimeout, KeepAlive: 30 * time.Second}
	}
	return &EgressPolicy{config: config, resolver: resolver, dialer: dialer}
}

// ValidateEndpoint canonicalizes an HTTP(S) endpoint and verifies its current
// DNS results. Callers must still use DialContext for connect-time authority.
func (policy *EgressPolicy) ValidateEndpoint(ctx context.Context, raw string) (*url.URL, error) {
	endpoint, err := policy.canonicalHTTPURL(raw)
	if err != nil {
		return nil, err
	}
	if _, err := policy.resolveApproved(ctx, endpoint.Hostname()); err != nil {
		return nil, err
	}
	return endpoint, nil
}

func (policy *EgressPolicy) ValidateURL(ctx context.Context, candidate *url.URL) error {
	if candidate == nil {
		return ErrEgressDenied
	}
	endpoint, err := policy.canonicalHTTPURL(candidate.String())
	if err != nil {
		return err
	}
	_, err = policy.resolveApproved(ctx, endpoint.Hostname())
	return err
}

func (policy *EgressPolicy) ValidateGRPCEndpoint(ctx context.Context, raw string) (GRPCEndpoint, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return GRPCEndpoint{}, ErrEgressDenied
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "grpc" && parsed.Scheme != "grpcs" {
		return GRPCEndpoint{}, ErrEgressDenied
	}
	host, err := canonicalEgressHost(parsed.Hostname())
	if err != nil {
		return GRPCEndpoint{}, err
	}
	port := uint16(80)
	if parsed.Scheme == "grpcs" {
		port = 443
	}
	if rawPort := parsed.Port(); rawPort != "" {
		value, parseErr := strconv.ParseUint(rawPort, 10, 16)
		if parseErr != nil || value == 0 {
			return GRPCEndpoint{}, ErrEgressDenied
		}
		port = uint16(value)
	}
	if !policy.portAllowed(port) {
		return GRPCEndpoint{}, ErrEgressDenied
	}
	if _, err := policy.resolveApproved(ctx, host); err != nil {
		return GRPCEndpoint{}, err
	}
	return GRPCEndpoint{
		Target: net.JoinHostPort(host, strconv.Itoa(int(port))), ServerName: host, TLS: parsed.Scheme == "grpcs",
	}, nil
}

func (policy *EgressPolicy) canonicalHTTPURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" {
		return nil, ErrEgressDenied
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, ErrEgressDenied
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return nil, ErrEgressDenied
	}
	host, err := canonicalEgressHost(parsed.Hostname())
	if err != nil {
		return nil, err
	}
	port, explicitPort, err := endpointPort(parsed)
	if err != nil || !policy.portAllowed(port) {
		return nil, ErrEgressDenied
	}
	if explicitPort {
		parsed.Host = net.JoinHostPort(host, strconv.Itoa(int(port)))
	} else if strings.Contains(host, ":") {
		parsed.Host = "[" + host + "]"
	} else {
		parsed.Host = host
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed, nil
}

func endpointPort(endpoint *url.URL) (uint16, bool, error) {
	raw := endpoint.Port()
	if raw == "" {
		switch endpoint.Scheme {
		case "http":
			return 80, false, nil
		case "https":
			return 443, false, nil
		default:
			return 0, false, ErrEgressDenied
		}
	}
	value, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || value == 0 {
		return 0, true, ErrEgressDenied
	}
	return uint16(value), true, nil
}

func canonicalEgressHost(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if host == "" || strings.Contains(host, "%") || len(host) > 253 {
		return "", ErrEgressDenied
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Unmap().String(), nil
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrEgressDenied
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return "", ErrEgressDenied
		}
	}
	return host, nil
}

func (policy *EgressPolicy) portAllowed(port uint16) bool {
	return slices.Contains(policy.config.AllowedPorts, port)
}

func (policy *EgressPolicy) resolveApproved(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if err := policy.approveAddress(address); err != nil {
			return nil, err
		}
		return []netip.Addr{address}, nil
	}
	addresses, err := policy.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return nil, ErrEgressResolve
	}
	unique := make(map[netip.Addr]struct{}, len(addresses))
	approved := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if _, duplicate := unique[address]; duplicate {
			continue
		}
		unique[address] = struct{}{}
		if err := policy.approveAddress(address); err != nil {
			return nil, err
		}
		approved = append(approved, address)
	}
	slices.SortFunc(approved, func(left, right netip.Addr) int { return left.Compare(right) })
	return approved, nil
}

var (
	cgnatPrefix       = netip.MustParsePrefix("100.64.0.0/10")
	documentationNets = []netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	benchmarkNets = []netip.Prefix{
		netip.MustParsePrefix("198.18.0.0/15"),
	}
	metadataAddresses = map[netip.Addr]struct{}{
		netip.MustParseAddr("169.254.169.254"): {},
		netip.MustParseAddr("100.100.100.200"): {},
		netip.MustParseAddr("168.63.129.16"):   {},
		netip.MustParseAddr("fd00:ec2::254"):   {},
	}
)

func (policy *EgressPolicy) approveAddress(address netip.Addr) error {
	if !address.IsValid() || address.IsUnspecified() || address.IsLoopback() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return ErrEgressDenied
	}
	if _, metadata := metadataAddresses[address]; metadata {
		return ErrEgressDenied
	}
	if prefixContains(documentationNets, address) || prefixContains(benchmarkNets, address) {
		return ErrEgressDenied
	}
	privateOrVPC := address.IsPrivate() || cgnatPrefix.Contains(address)
	if privateOrVPC {
		if prefixContains(policy.config.PrivateCIDRAllowlist, address) {
			return nil
		}
		return ErrEgressDenied
	}
	if !address.IsGlobalUnicast() {
		return ErrEgressDenied
	}
	return nil
}

func prefixContains(prefixes []netip.Prefix, address netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// DialContext re-resolves and pins the actual connection to an approved IP.
// It is suitable for both http.Transport and grpc.WithContextDialer.
func (policy *EgressPolicy) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, ErrEgressDenied
	}
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrEgressDenied
	}
	host, err = canonicalEgressHost(host)
	if err != nil {
		return nil, err
	}
	parsedPort, err := strconv.ParseUint(rawPort, 10, 16)
	if err != nil || parsedPort == 0 || !policy.portAllowed(uint16(parsedPort)) {
		return nil, ErrEgressDenied
	}
	addresses, err := policy.resolveApproved(ctx, host)
	if err != nil {
		return nil, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, policy.config.ConnectTimeout)
	defer cancel()
	for _, approved := range addresses {
		connection, dialErr := policy.dialer.DialContext(dialCtx, network, net.JoinHostPort(approved.String(), rawPort))
		if dialErr == nil {
			return connection, nil
		}
		if dialCtx.Err() != nil {
			break
		}
	}
	return nil, ErrEgressConnect
}

// EgressHTTPClient disables environment proxies and revalidates every initial
// URL, redirect and transport connection.
type EgressHTTPClient struct {
	policy    *EgressPolicy
	client    *http.Client
	transport *http.Transport
}

func (policy *EgressPolicy) NewHTTPClient() *EgressHTTPClient {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           policy.DialContext,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		ResponseHeaderTimeout: policy.config.InvocationTimeout,
		TLSHandshakeTimeout:   policy.config.ConnectTimeout,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	wrapper := &EgressHTTPClient{policy: policy, transport: transport}
	wrapper.client = &http.Client{
		Transport: transport,
		Timeout:   policy.config.InvocationTimeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) > policy.config.MaxRedirects {
				return ErrEgressDenied
			}
			if err := policy.ValidateURL(request.Context(), request.URL); err != nil {
				return err
			}
			if len(via) > 0 && !sameURLAuthority(via[len(via)-1].URL, request.URL) {
				request.Header.Del("Authorization")
				request.Header.Del("Cookie")
				request.Header.Del("Proxy-Authorization")
			}
			return nil
		},
	}
	return wrapper
}

func sameURLAuthority(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}

func (client *EgressHTTPClient) Do(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, ErrEgressDenied
	}
	if err := client.policy.ValidateURL(request.Context(), request.URL); err != nil {
		return nil, err
	}
	return client.client.Do(request)
}

func (client *EgressHTTPClient) CloseIdleConnections() {
	client.transport.CloseIdleConnections()
}

func (client *EgressHTTPClient) standardClient() *http.Client {
	return client.client
}

func ReadBoundedResponse(response *http.Response, maxBytes int64) ([]byte, error) {
	if response == nil || response.Body == nil || maxBytes <= 0 {
		return nil, fmt.Errorf("invalid bounded response")
	}
	limited := io.LimitReader(response.Body, maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, ErrResponseTooBig
	}
	return body, nil
}
