package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRemoteMCPDiscoverUsesControlledPrivateEgress(t *testing.T) {
	privateAddress, ok := privateInterfaceAddress()
	if !ok {
		t.Skip("no private non-loopback interface is available")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		t.Skipf("private interface cannot host a fixture: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "remote-fixture", Version: "1"}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{},
	})
	var upstreamCalls atomic.Int32
	var cancellationObserved atomic.Bool
	server.AddTool(&mcp.Tool{
		Name: "echo/value", Description: "remote echo",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"echo":{"type":"string"}}}`),
	}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		upstreamCalls.Add(1)
		var arguments map[string]any
		_ = json.Unmarshal(request.Params.Arguments, &arguments)
		value, _ := arguments["value"].(string)
		switch value {
		case "wait":
			<-ctx.Done()
			cancellationObserved.Store(true)
			return nil, ctx.Err()
		case "fail":
			return nil, errors.New("fixture failure")
		case "large":
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat("x", 4096)}}}, nil
		default:
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: value}}, StructuredContent: map[string]any{"echo": value},
			}, nil
		}
	})
	unsupportedServer := mcp.NewServer(&mcp.Implementation{Name: "unsupported-fixture", Version: "1"}, nil)
	unsupportedServer.AddTool(&mcp.Tool{Name: "tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		})
	var requestCount atomic.Int32
	var sawAuth atomic.Bool
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, PropagateRequestCancellation: true,
	})
	unsupportedHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return unsupportedServer }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, PropagateRequestCancellation: true,
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		if request.Header.Get("X-Fixture-Auth") == "sentinel" {
			sawAuth.Store(true)
		}
		if request.URL.Path == "/unsupported" {
			unsupportedHandler.ServeHTTP(w, request)
		} else {
			mcpHandler.ServeHTTP(w, request)
		}
	})
	httpServer := &http.Server{Handler: handler}
	go func() { _ = httpServer.Serve(listener) }()
	t.Cleanup(func() { _ = httpServer.Shutdown(context.Background()) })

	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	resolver := &staticResolver{answers: map[string][][]netip.Addr{"mcp-fixture.internal": {{privateAddress}}}}
	config := Config{
		AllowedPorts: []uint16{port}, PrivateCIDRAllowlist: []netip.Prefix{netip.PrefixFrom(privateAddress, privateAddress.BitLen())},
		ConnectTimeout: time.Second, InvocationTimeout: 2 * time.Second, MaxResponseBytes: 1 << 20, MaxRedirects: 1,
	}
	adapter := NewRemoteMCPAdapter(config, NewEgressPolicy(config, EgressPolicyOptions{Resolver: resolver}), nil)
	endpoint := "http://mcp-fixture.internal:" + strconv.Itoa(int(port)) + "/mcp"
	tools, err := adapter.Discover(context.Background(), nil, lwdb.ToolSource{
		Name: "remote-fixture", Kind: "remote_mcp",
	}, lwdb.ToolSourceRevision{Endpoint: pgtype.Text{String: endpoint, Valid: true}})
	if err != nil {
		t.Fatalf("controlled remote MCP discovery failed: %v", err)
	}
	if len(tools) != 1 || tools[0].PublicName != "remote-fixture.echo_value" || tools[0].UpstreamName != "echo/value" {
		t.Fatalf("discovered tools = %+v", tools)
	}
	if requestCount.Load() == 0 {
		t.Fatal("remote MCP fixture received no request")
	}
	headers, err := parseSourceAuthHeaders([]byte(`{"kind":"headers","headers":{"X-Fixture-Auth":"sentinel"}}`))
	if err != nil {
		t.Fatal(err)
	}
	session, client, err := adapter.connect(context.Background(), endpoint, headers)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.ListTools(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	_ = session.Close()
	client.CloseIdleConnections()
	if !sawAuth.Load() {
		t.Fatal("fixed Source authentication header did not reach upstream")
	}

	callSession, callClient, err := adapter.connect(context.Background(), endpoint, headers)
	if err != nil {
		t.Fatal(err)
	}
	callResult, err := callSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo/value", Arguments: map[string]any{"value": "called"}})
	if err != nil || callResult.StructuredContent.(map[string]any)["echo"] != "called" {
		t.Fatalf("remote MCP call result=%+v err=%v", callResult, err)
	}
	beforeFailure := upstreamCalls.Load()
	_, _ = callSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo/value", Arguments: map[string]any{"value": "fail"}})
	if upstreamCalls.Load()-beforeFailure != 1 {
		t.Fatalf("remote MCP failing call count = %d, want 1", upstreamCalls.Load()-beforeFailure)
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	_, _ = callSession.CallTool(cancelCtx, &mcp.CallToolParams{Name: "echo/value", Arguments: map[string]any{"value": "wait"}})
	deadline := time.Now().Add(time.Second)
	for !cancellationObserved.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !cancellationObserved.Load() {
		t.Fatal("remote MCP cancellation did not reach upstream")
	}
	_ = callSession.Close()
	callClient.CloseIdleConnections()

	smallConfig := config
	smallConfig.MaxResponseBytes = 1024
	smallAdapter := NewRemoteMCPAdapter(smallConfig, NewEgressPolicy(smallConfig, EgressPolicyOptions{Resolver: resolver}), nil)
	largeSession, largeClient, err := smallAdapter.connect(context.Background(), endpoint, headers)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := largeSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo/value", Arguments: map[string]any{"value": "large"}}); err == nil {
		t.Fatalf("oversized remote MCP result error = %v", err)
	}
	_ = largeSession.Close()
	largeClient.CloseIdleConnections()

	if _, err := adapter.Discover(context.Background(), nil, lwdb.ToolSource{Name: "unsupported", Kind: "remote_mcp"},
		lwdb.ToolSourceRevision{Endpoint: pgtype.Text{String: "http://mcp-fixture.internal:" + strconv.Itoa(int(port)) + "/unsupported", Valid: true}}); err == nil {
		t.Fatal("remote MCP server with non-tool logging capability was accepted")
	}
}

func TestRemoteMCPAuthCapabilitiesAndResponseBounds(t *testing.T) {
	headers, err := parseSourceAuthHeaders([]byte(`{"kind":"bearer","token":"secret-token"}`))
	if err != nil || headers.Get("Authorization") != "Bearer secret-token" {
		t.Fatalf("bearer headers=%v err=%v", headers, err)
	}
	invalid := [][]byte{
		[]byte(`{"kind":"bearer","token":"x"} trailing`),
		[]byte(`{"kind":"headers","headers":{"Host":"override"}}`),
		[]byte(`{"kind":"headers","headers":{"X-Test":"ok\r\ninjected"}}`),
		[]byte(`{"kind":"bearer","token":"x","unknown":true}`),
	}
	for _, raw := range invalid {
		if _, err := parseSourceAuthHeaders(raw); err == nil || strings.Contains(err.Error(), "secret-token") {
			t.Fatalf("credential %q error = %v", raw, err)
		}
	}

	if toolsOnlyCapabilities(&mcp.InitializeResult{Capabilities: &mcp.ServerCapabilities{
		Tools: &mcp.ToolCapabilities{}, Prompts: &mcp.PromptCapabilities{},
	}}) {
		t.Fatal("server with prompt callbacks passed tools-only validation")
	}
	if !toolsOnlyCapabilities(&mcp.InitializeResult{Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}}) {
		t.Fatal("tools-only server was rejected")
	}

	exact := &boundedReadCloser{body: io.NopCloser(strings.NewReader("1234")), remaining: 4}
	body, err := io.ReadAll(exact)
	if err != nil || string(body) != "1234" {
		t.Fatalf("exact bound body=%q err=%v", body, err)
	}
	oversized := &boundedReadCloser{body: io.NopCloser(strings.NewReader("12345")), remaining: 4}
	_, err = io.ReadAll(oversized)
	if !errors.Is(err, ErrResponseTooBig) {
		t.Fatalf("oversized body error = %v", err)
	}
}
