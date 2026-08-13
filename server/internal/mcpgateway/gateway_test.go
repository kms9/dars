package mcpgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGatewayPreservesMCPTransportResponses(t *testing.T) {
	gateway := newTestGateway(t, 1024)

	request := httptest.NewRequest(http.MethodGet, "/bundles/tb_01ABCDEFG/mcp", nil)
	request = WithBundleID(request, "tb_01ABCDEFG")
	recorder := httptest.NewRecorder()
	gateway.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405; body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"error":{"code"`) {
		t.Fatalf("MCP response was wrapped in REST envelope: %s", recorder.Body.String())
	}

	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	request = httptest.NewRequest(http.MethodPost, "/bundles/tb_01ABCDEFG/mcp", strings.NewReader(initialize))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request = WithBundleID(request, "tb_01ABCDEFG")
	recorder = httptest.NewRecorder()
	gateway.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want MCP stream", contentType)
	}
	if strings.Contains(recorder.Body.String(), `"logging"`) || !strings.Contains(recorder.Body.String(), `"tools"`) {
		t.Fatalf("capabilities are not tool-only: %s", recorder.Body.String())
	}
}

func TestGatewayEnforcesBodyLimitAndUniformAuthorization(t *testing.T) {
	gateway := newTestGateway(t, 32)
	request := httptest.NewRequest(http.MethodPost, "/bundles/tb_01ABCDEFG/mcp", strings.NewReader(strings.Repeat("x", 64)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request = WithBundleID(request, "tb_01ABCDEFG")
	recorder := httptest.NewRecorder()
	gateway.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status = %d, want 413; body=%s", recorder.Code, recorder.Body.String())
	}

	denied, err := New(testConfig(1024), Dependencies{
		IdentityResolver: testIdentityResolver(),
		Store:            StoreFunc(func(context.Context, BundleAccess) error { return ErrUnauthorized }),
		Projector:        emptyProjector(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, bundleID := range []string{"invalid", "tb_missing123"} {
		request := httptest.NewRequest(http.MethodPost, "/bundles/"+bundleID+"/mcp", strings.NewReader(`{}`))
		request = WithBundleID(request, bundleID)
		recorder := httptest.NewRecorder()
		denied.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != "Unauthorized\n" {
			t.Fatalf("bundle %q response = %d %q", bundleID, recorder.Code, recorder.Body.String())
		}
	}
}

func TestGatewayCloseRejectsNewRequests(t *testing.T) {
	gateway := newTestGateway(t, 1024)
	if err := gateway.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/bundles/tb_01ABCDEFG/mcp", nil)
	request = WithBundleID(request, "tb_01ABCDEFG")
	recorder := httptest.NewRecorder()
	gateway.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestGatewayCloseCancelsInflightCallsBeforeClosingResources(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	resource := &trackingGatewayResource{}
	projector := ProjectorFunc(func(context.Context, BundleAccess) ([]ToolRegistration, error) {
		return []ToolRegistration{{
			Tool: &mcp.Tool{Name: "wait", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)},
			Handler: func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				close(started)
				<-ctx.Done()
				close(finished)
				return nil, ctx.Err()
			},
		}}, nil
	})
	gateway, err := New(testConfig(1024), Dependencies{
		IdentityResolver: testIdentityResolver(),
		Store:            StoreFunc(func(context.Context, BundleAccess) error { return nil }),
		Projector:        projector,
		Resources:        []GatewayResource{resource},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gateway.ServeHTTP(w, WithBundleID(r, "tb_01ABCDEFG"))
	}))
	defer server.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "shutdown-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint: server.URL, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		_, _ = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "wait", Arguments: map[string]any{}})
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("tool call did not start")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gateway.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("in-flight tool call did not observe Gateway cancellation")
	}
	select {
	case <-callDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled MCP call did not terminate")
	}
	if resource.closed.Load() != 1 {
		t.Fatalf("resource close count = %d, want 1", resource.closed.Load())
	}
	if err := gateway.Close(context.Background()); err != nil || resource.closed.Load() != 1 {
		t.Fatalf("idempotent close err=%v count=%d", err, resource.closed.Load())
	}
}

type trackingGatewayResource struct {
	closed atomic.Int32
}

func (resource *trackingGatewayResource) Close() error {
	resource.closed.Add(1)
	return nil
}

func newTestGateway(t *testing.T, bodyLimit int64) *Gateway {
	t.Helper()
	gateway, err := New(testConfig(bodyLimit), Dependencies{
		IdentityResolver: testIdentityResolver(),
		Store:            StoreFunc(func(context.Context, BundleAccess) error { return nil }),
		Projector:        emptyProjector(),
		Version:          "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gateway.Close(context.Background()) })
	return gateway
}

func testConfig(bodyLimit int64) Config {
	return Config{MaxRequestBodyBytes: bodyLimit}
}

func testIdentityResolver() IdentityResolver {
	return IdentityResolverFunc(func(context.Context) (Identity, bool) {
		return Identity{UserID: "user", WorkspaceID: "workspace", AgentID: "agent", TaskID: "task", ToolBundleID: "tb_01ABCDEFG"}, true
	})
}

func emptyProjector() Projector {
	return ProjectorFunc(func(context.Context, BundleAccess) ([]ToolRegistration, error) {
		return nil, nil
	})
}
