package mcpgateway

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type staticResolver struct {
	mu      sync.Mutex
	answers map[string][][]netip.Addr
	calls   map[string]int
}

func (resolver *staticResolver) LookupNetIP(_ context.Context, _ string, host string) ([]netip.Addr, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if resolver.calls == nil {
		resolver.calls = make(map[string]int)
	}
	sequences := resolver.answers[host]
	if len(sequences) == 0 {
		return nil, errors.New("fixture DNS miss")
	}
	index := resolver.calls[host]
	resolver.calls[host]++
	if index >= len(sequences) {
		index = len(sequences) - 1
	}
	return append([]netip.Addr(nil), sequences[index]...), nil
}

type recordingDialer struct {
	mu        sync.Mutex
	addresses []string
	peers     []net.Conn
}

func (dialer *recordingDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	client, server := net.Pipe()
	dialer.mu.Lock()
	dialer.addresses = append(dialer.addresses, address)
	dialer.peers = append(dialer.peers, server)
	dialer.mu.Unlock()
	return client, nil
}

func (dialer *recordingDialer) close() {
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	for _, peer := range dialer.peers {
		_ = peer.Close()
	}
}

func TestEgressPolicyEndpointAndAddressClasses(t *testing.T) {
	resolver := &staticResolver{answers: map[string][][]netip.Addr{
		"public.example":  {{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("2606:4700:4700::1111")}},
		"mixed.example":   {{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("127.0.0.1")}},
		"private.example": {{netip.MustParseAddr("10.20.30.40")}},
	}}
	policy := NewEgressPolicy(Config{
		AllowedPorts: []uint16{80, 443, 8443}, PrivateCIDRAllowlist: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16")},
		ConnectTimeout: time.Second, InvocationTimeout: time.Second,
	}, EgressPolicyOptions{Resolver: resolver})

	endpoint, err := policy.ValidateEndpoint(context.Background(), "HTTPS://PUBLIC.EXAMPLE.:8443/mcp?q=1")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.String() != "https://public.example:8443/mcp?q=1" {
		t.Fatalf("canonical endpoint = %q", endpoint)
	}
	if _, err := policy.ValidateEndpoint(context.Background(), "http://private.example/tool"); err != nil {
		t.Fatalf("deployment-allowlisted private target denied: %v", err)
	}

	denied := []string{
		"ftp://public.example/tool",
		"https://user:secret@public.example/tool",
		"https://public.example:22/tool",
		"https://mixed.example/tool",
		"http://127.0.0.1/tool",
		"http://169.254.169.254/latest/meta-data",
		"http://100.100.100.200/metadata",
		"http://192.0.2.10/tool",
		"http://[::1]/tool",
		"http://bad_host.example/tool",
	}
	for _, raw := range denied {
		if _, err := policy.ValidateEndpoint(context.Background(), raw); !errors.Is(err, ErrEgressDenied) {
			t.Fatalf("endpoint %q error = %v, want ErrEgressDenied", raw, err)
		}
	}
}

func TestEgressDialRevalidatesDNSAndPinsApprovedIP(t *testing.T) {
	resolver := &staticResolver{answers: map[string][][]netip.Addr{
		"rebind.example": {
			{netip.MustParseAddr("8.8.8.8")},
			{netip.MustParseAddr("127.0.0.1")},
		},
		"stable.example": {
			{netip.MustParseAddr("1.1.1.1")},
		},
	}}
	dialer := &recordingDialer{}
	t.Cleanup(dialer.close)
	policy := NewEgressPolicy(Config{
		AllowedPorts: []uint16{443}, ConnectTimeout: time.Second, InvocationTimeout: time.Second,
	}, EgressPolicyOptions{Resolver: resolver, Dialer: dialer})
	if _, err := policy.ValidateEndpoint(context.Background(), "https://rebind.example/mcp"); err != nil {
		t.Fatal(err)
	}
	if _, err := policy.DialContext(context.Background(), "tcp", "rebind.example:443"); !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("rebound dial error = %v", err)
	}
	connection, err := policy.DialContext(context.Background(), "tcp", "stable.example:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	dialer.mu.Lock()
	defer dialer.mu.Unlock()
	if len(dialer.addresses) != 1 || dialer.addresses[0] != "1.1.1.1:443" {
		t.Fatalf("dialed addresses = %v", dialer.addresses)
	}
}

func TestEgressGRPCEndpointKeepsOriginalTLSIdentity(t *testing.T) {
	resolver := &staticResolver{answers: map[string][][]netip.Addr{
		"grpc.example": {{netip.MustParseAddr("8.8.8.8")}},
	}}
	policy := NewEgressPolicy(Config{AllowedPorts: []uint16{443, 8443}}, EgressPolicyOptions{Resolver: resolver})
	endpoint, err := policy.ValidateGRPCEndpoint(context.Background(), "grpcs://GRPC.EXAMPLE.:8443")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Target != "grpc.example:8443" || endpoint.ServerName != "grpc.example" || !endpoint.TLS {
		t.Fatalf("gRPC endpoint = %+v", endpoint)
	}
	for _, raw := range []string{
		"https://grpc.example:8443", "grpcs://user:secret@grpc.example:8443", "grpcs://grpc.example:8443/service",
		"grpcs://grpc.example:8443?target=x", "grpc://127.0.0.1:8443",
	} {
		if _, err := policy.ValidateGRPCEndpoint(context.Background(), raw); !errors.Is(err, ErrEgressDenied) {
			t.Fatalf("gRPC endpoint %q error = %v", raw, err)
		}
	}
}

func TestEgressHTTPClientUsesAllowlistedPrivateNetworkAndRejectsRedirect(t *testing.T) {
	privateAddress, ok := privateInterfaceAddress()
	if !ok {
		t.Skip("no private non-loopback interface is available")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		t.Skipf("private interface cannot host a fixture: %v", err)
	}
	cancellationObserved := make(chan struct{})
	var cancelOnce sync.Once
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("fixture-ok"))
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", 64)))
		case "/redirect":
			http.Redirect(w, request, "http://127.0.0.1:1/private", http.StatusFound)
		case "/wait":
			<-request.Context().Done()
			cancelOnce.Do(func() { close(cancellationObserved) })
		default:
			http.NotFound(w, request)
		}
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	resolver := &staticResolver{answers: map[string][][]netip.Addr{
		"fixture.internal": {{privateAddress}},
	}}
	policy := NewEgressPolicy(Config{
		AllowedPorts: []uint16{port}, PrivateCIDRAllowlist: []netip.Prefix{netip.PrefixFrom(privateAddress, privateAddress.BitLen())},
		ConnectTimeout: time.Second, InvocationTimeout: 2 * time.Second, MaxRedirects: 2, MaxResponseBytes: 32,
	}, EgressPolicyOptions{Resolver: resolver})
	client := policy.NewHTTPClient()
	t.Cleanup(client.CloseIdleConnections)
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("NO_PROXY", "")

	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://fixture.internal:"+strconv.Itoa(int(port))+"/ok", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := ReadBoundedResponse(response, 32)
	_ = response.Body.Close()
	if err != nil || string(body) != "fixture-ok" {
		t.Fatalf("body=%q err=%v", body, err)
	}

	largeRequest, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://fixture.internal:"+strconv.Itoa(int(port))+"/large", nil)
	largeResponse, err := client.Do(largeRequest)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ReadBoundedResponse(largeResponse, 32)
	_ = largeResponse.Body.Close()
	if !errors.Is(err, ErrResponseTooBig) {
		t.Fatalf("oversized response error = %v", err)
	}

	redirectRequest, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://fixture.internal:"+strconv.Itoa(int(port))+"/redirect", nil)
	if _, err := client.Do(redirectRequest); !errors.Is(err, ErrEgressDenied) {
		t.Fatalf("restricted redirect error = %v", err)
	}

	timeoutPolicy := NewEgressPolicy(Config{
		AllowedPorts: []uint16{port}, PrivateCIDRAllowlist: []netip.Prefix{netip.PrefixFrom(privateAddress, privateAddress.BitLen())},
		ConnectTimeout: time.Second, InvocationTimeout: 75 * time.Millisecond, MaxRedirects: 1,
	}, EgressPolicyOptions{Resolver: resolver})
	timeoutClient := timeoutPolicy.NewHTTPClient()
	t.Cleanup(timeoutClient.CloseIdleConnections)
	waitRequest, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://fixture.internal:"+strconv.Itoa(int(port))+"/wait", nil)
	if _, err := timeoutClient.Do(waitRequest); err == nil {
		t.Fatal("HTTP invocation timeout was not enforced")
	}
	select {
	case <-cancellationObserved:
	case <-time.After(time.Second):
		t.Fatal("HTTP timeout did not cancel the upstream request")
	}
}

func TestEgressHTTPClientRejectsTLSIdentityMismatchOnPrivateFixture(t *testing.T) {
	privateAddress, ok := privateInterfaceAddress()
	if !ok {
		t.Skip("no private non-loopback interface is available")
	}
	plainListener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		t.Skipf("private interface cannot host a fixture: %v", err)
	}
	seed := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	tlsConfig := seed.TLS.Clone()
	seed.Close()
	tlsListener := tls.NewListener(plainListener, tlsConfig)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })}
	go func() { _ = server.Serve(tlsListener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })
	port := uint16(plainListener.Addr().(*net.TCPAddr).Port)
	resolver := &staticResolver{answers: map[string][][]netip.Addr{"tls-mismatch.internal": {{privateAddress}}}}
	policy := NewEgressPolicy(Config{
		AllowedPorts: []uint16{port}, PrivateCIDRAllowlist: []netip.Prefix{netip.PrefixFrom(privateAddress, privateAddress.BitLen())},
		ConnectTimeout: time.Second, InvocationTimeout: time.Second,
	}, EgressPolicyOptions{Resolver: resolver})
	client := policy.NewHTTPClient()
	t.Cleanup(client.CloseIdleConnections)
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://tls-mismatch.internal:"+strconv.Itoa(int(port)), nil)
	if _, err := client.Do(request); err == nil {
		t.Fatal("TLS identity mismatch was accepted")
	}
}

func privateInterfaceAddress() (netip.Addr, bool) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return netip.Addr{}, false
	}
	for _, networkInterface := range interfaces {
		if networkInterface.Flags&net.FlagUp == 0 || networkInterface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := networkInterface.Addrs()
		if err != nil {
			continue
		}
		for _, raw := range addresses {
			prefix, err := netip.ParsePrefix(raw.String())
			if err == nil && prefix.Addr().Is4() && prefix.Addr().IsPrivate() {
				return prefix.Addr(), true
			}
		}
	}
	return netip.Addr{}, false
}
