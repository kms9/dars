package mcpgateway

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const openAPIFixtureDocument = `{
  "openapi":"3.0.3",
  "info":{"title":"fixture","version":"1"},
  "paths":{
    "/items/{id}":{
      "post":{
        "operationId":"updateItem",
        "summary":"Update one item",
        "parameters":[
          {"name":"id","in":"path","required":true,"schema":{"type":"string"}},
          {"name":"verbose","in":"query","schema":{"type":"boolean"}},
          {"name":"X-Mode","in":"header","schema":{"type":"string"}}
        ],
        "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}}}},
        "responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"type":"object","properties":{"accepted":{"type":"boolean"}}}}}}}
      }
    }
  }
}`

func TestOpenAPICompileAndInvokeOnControlledPrivateFixture(t *testing.T) {
	privateAddress, ok := privateInterfaceAddress()
	if !ok {
		t.Skip("no private non-loopback interface is available")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		t.Skipf("private interface cannot host a fixture: %v", err)
	}
	var calls atomic.Int32
	var cancellationObserved atomic.Bool
	var crossAuthorityCredentialLeaked atomic.Bool
	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		switch {
		case request.URL.Path == "/schema.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"type":"object","properties":{"remote":{"type":"string"}},"required":["remote"]}`))
			return
		case request.URL.Path == "/redirect":
			http.Redirect(w, request, "http://openapi-other.internal:"+strconv.Itoa(int(port))+"/leak", http.StatusFound)
			return
		case request.URL.Path == "/leak":
			crossAuthorityCredentialLeaked.Store(request.Header.Get("X-API-Key") != "")
			_, _ = w.Write([]byte("ok"))
			return
		case strings.Contains(request.URL.Path, "/wait"):
			<-request.Context().Done()
			cancellationObserved.Store(true)
			return
		case strings.Contains(request.URL.Path, "/large"):
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
			return
		case strings.Contains(request.URL.Path, "/fail"):
			http.Error(w, "fixture failure", http.StatusServiceUnavailable)
			return
		}
		if request.Method != http.MethodPost || request.URL.Query().Get("verbose") != "true" ||
			request.Header.Get("X-Mode") != "safe" || request.Header.Get("X-API-Key") != "sentinel" {
			http.Error(w, "bad request mapping", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(request.Body)
		if string(body) != `{"name":"fixture"}` {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accepted":true}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Shutdown(context.Background()) })

	resolver := &staticResolver{answers: map[string][][]netip.Addr{
		"openapi-fixture.internal": {{privateAddress}}, "openapi-other.internal": {{privateAddress}},
	}}
	config := Config{
		AllowedPorts: []uint16{port}, PrivateCIDRAllowlist: []netip.Prefix{netip.PrefixFrom(privateAddress, privateAddress.BitLen())},
		ConnectTimeout: time.Second, InvocationTimeout: 2 * time.Second,
		MaxRequestBodyBytes: 1 << 20, MaxResponseBytes: 1024, MaxArtifactBytes: 1 << 20, MaxRedirects: 2,
	}
	adapter := NewOpenAPIAdapter(config, NewEgressPolicy(config, EgressPolicyOptions{Resolver: resolver}), nil)
	document, err := adapter.loadDocument(context.Background(), []byte(openAPIFixtureDocument), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	baseURL := "http://openapi-fixture.internal:" + strconv.Itoa(int(port)) + "/api"
	tools, err := compileOpenAPIDocument("http-fixture", baseURL, document)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].PublicName != "http-fixture.updateItem" ||
		!strings.Contains(string(tools[0].InputSchema), `"path"`) || !strings.Contains(string(tools[0].InputSchema), `"body"`) {
		t.Fatalf("compiled OpenAPI tools = %+v", tools)
	}
	var operation openAPIOperationPlan
	if err := json.Unmarshal(tools[0].OperationMetadata, &operation); err != nil {
		t.Fatal(err)
	}
	result, err := adapter.invoke(context.Background(), operation, map[string]any{
		"path": map[string]any{"id": "item/one"}, "query": map[string]any{"verbose": true},
		"header": map[string]any{"X-Mode": "safe"}, "body": map[string]any{"name": "fixture"},
		"endpoint": "http://169.254.169.254/override",
	}, http.Header{"X-Api-Key": []string{"sentinel"}})
	if err != nil || result.IsError {
		t.Fatalf("OpenAPI result=%+v err=%v", result, err)
	}
	structured := result.StructuredContent.(map[string]any)
	responseBody := structured["body"].(map[string]any)
	if structured["status"] != http.StatusOK || responseBody["accepted"] != true {
		t.Fatalf("OpenAPI structured result = %#v", structured)
	}

	remoteRefDocument := strings.Replace(openAPIFixtureDocument,
		`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`,
		`{"$ref":"http://openapi-fixture.internal:`+strconv.Itoa(int(port))+`/schema.json"}`, 1)
	loadedWithRemoteRef, err := adapter.loadDocument(context.Background(), []byte(remoteRefDocument),
		"http://openapi-fixture.internal:"+strconv.Itoa(int(port))+"/openapi.json", nil)
	if err != nil {
		t.Fatalf("approved remote OpenAPI ref failed: %v", err)
	}
	remoteRefTools, err := compileOpenAPIDocument("remote-ref", baseURL, loadedWithRemoteRef)
	if err != nil || len(remoteRefTools) != 1 || !strings.Contains(string(remoteRefTools[0].InputSchema), `"remote"`) {
		t.Fatalf("remote ref tools=%+v err=%v", remoteRefTools, err)
	}

	redirectResponse, err := adapter.do(context.Background(), http.MethodGet,
		"http://openapi-fixture.internal:"+strconv.Itoa(int(port))+"/redirect", nil,
		http.Header{"X-Api-Key": []string{"sentinel"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = redirectResponse.Body.Close()
	if crossAuthorityCredentialLeaked.Load() {
		t.Fatal("Source credential leaked across redirect authority")
	}

	beforeFailure := calls.Load()
	failure, err := adapter.invoke(context.Background(), operation, map[string]any{
		"path": map[string]any{"id": "fail"}, "body": map[string]any{"name": "fixture"},
	}, nil)
	if err != nil || !failure.IsError || calls.Load()-beforeFailure != 1 {
		t.Fatalf("non-idempotent failure result=%+v err=%v calls=%d", failure, err, calls.Load()-beforeFailure)
	}

	waitPlan := openAPIOperationPlan{Method: http.MethodGet, BaseURL: baseURL, Path: "/items/wait"}
	cancelCtx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	_, _ = adapter.invoke(cancelCtx, waitPlan, map[string]any{}, nil)
	deadline := time.Now().Add(time.Second)
	for !cancellationObserved.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !cancellationObserved.Load() {
		t.Fatal("OpenAPI cancellation did not reach upstream")
	}

	largePlan := openAPIOperationPlan{Method: http.MethodGet, BaseURL: baseURL, Path: "/items/large"}
	if _, err := adapter.invoke(context.Background(), largePlan, map[string]any{}, nil); err == nil {
		t.Fatal("oversized OpenAPI response was accepted")
	}
}

func TestOpenAPIAdapterNormalizesSwagger2JSONAndYAML(t *testing.T) {
	adapter := NewOpenAPIAdapter(Config{MaxArtifactBytes: 1 << 20}, nil, nil)
	documents := map[string]string{
		"json": `{"swagger":"2.0","info":{"title":"legacy","version":"1"},"paths":{"/pets":{"get":{"operationId":"listPets","responses":{"200":{"description":"ok"}}}}}}`,
		"yaml": "swagger: \"2.0\"\ninfo:\n  title: legacy\n  version: \"1\"\npaths:\n  /pets:\n    get:\n      operationId: listPets\n      responses:\n        \"200\":\n          description: ok\n",
	}
	for name, content := range documents {
		t.Run(name, func(t *testing.T) {
			document, err := adapter.loadDocument(context.Background(), []byte(content), "", nil)
			if err != nil {
				t.Fatal(err)
			}
			tools, err := compileOpenAPIDocument("legacy", "https://example.com/api", document)
			if err != nil || len(tools) != 1 || tools[0].PublicName != "legacy.listPets" {
				t.Fatalf("Swagger 2.0 tools=%+v err=%v", tools, err)
			}
		})
	}
}

func TestOpenAPIRejectsProtectedHeadersAndRestrictedRemoteRefs(t *testing.T) {
	loaderConfig := Config{AllowedPorts: []uint16{443}, MaxArtifactBytes: 1 << 20}
	loaderAdapter := NewOpenAPIAdapter(loaderConfig, NewEgressPolicy(loaderConfig, EgressPolicyOptions{}), nil)
	protected := strings.Replace(openAPIFixtureDocument, `"X-Mode"`, `"Authorization"`, 1)
	document, err := loaderAdapter.loadDocument(context.Background(), []byte(protected), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compileOpenAPIDocument("protected", "https://8.8.8.8", document); err == nil {
		t.Fatal("OpenAPI Authorization parameter was accepted")
	}

	restricted := `{"openapi":"3.0.3","info":{"title":"x","version":"1"},"paths":{"/x":{"get":{"operationId":"x","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"http://169.254.169.254/schema.json"}}}}}}}}}`
	if _, err := loaderAdapter.loadDocument(context.Background(), []byte(restricted), "https://8.8.8.8/openapi.json", nil); err == nil {
		t.Fatal("metadata OpenAPI reference was accepted")
	}
}
