package mcpgateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kms9/dars/internal/auth"
	"github.com/kms9/dars/internal/lightweightapi"
	"github.com/kms9/dars/internal/util"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

const adapterE3AuthSecret = "adapter-e3-auth-secret"

func TestDatabaseRegistryControlledAdapterE3OnFreshCheckDatabase(t *testing.T) {
	pool := openControlPlaneCheckDatabase(t)
	privateAddress, ok := privateInterfaceAddress()
	if !ok {
		t.Skip("no private non-loopback interface is available")
	}

	descriptors, err := NewDescriptorRegistry(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	embeddedArtifact, ok := descriptors.EmbeddedArtifact("dars_gateway_v1")
	if !ok {
		t.Fatal("embedded descriptor is unavailable")
	}
	embeddedCompiled, err := descriptors.CompileArtifact(context.Background(), embeddedArtifact, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	embeddedMethod := embeddedCompiled.methods["/dars.gateway.v1.GatewayUtility/Echo"]
	uploadedArtifact := descriptorTestArtifact("text/x-proto", []byte(unaryFixtureProto))
	uploadedCompiled, err := descriptors.CompileArtifact(context.Background(), uploadedArtifact, json.RawMessage(`{"root_file":"fixture.proto"}`))
	if err != nil {
		t.Fatal(err)
	}
	uploadedMethod := uploadedCompiled.methods["/fixture.v1.EchoService/Echo"]
	if embeddedMethod == nil || uploadedMethod == nil {
		t.Fatal("controlled gRPC fixture descriptors are incomplete")
	}

	httpListener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		t.Skipf("private interface cannot host HTTP fixtures: %v", err)
	}
	grpcListener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		_ = httpListener.Close()
		t.Skipf("private interface cannot host gRPC fixtures: %v", err)
	}

	var remoteCalls atomic.Int32
	var remoteAuthSeen atomic.Bool
	remoteServer := mcp.NewServer(&mcp.Implementation{Name: "adapter-e3-remote", Version: "1"}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{},
	})
	remoteServer.AddTool(&mcp.Tool{
		Name: "echo/value", Description: "controlled E3 echo",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false}`),
		OutputSchema: json.RawMessage(`{"type":"object","properties":{"echo":{"type":"string"}},"required":["echo"],"additionalProperties":false}`),
	}, func(_ context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		remoteCalls.Add(1)
		var arguments map[string]any
		_ = json.Unmarshal(request.Params.Arguments, &arguments)
		value, _ := arguments["value"].(string)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: value}}, StructuredContent: map[string]any{"echo": value},
		}, nil
	})
	remoteHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return remoteServer }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, PropagateRequestCancellation: true,
	})

	var openAPIDocumentFetches atomic.Int32
	var openAPICalls atomic.Int32
	var openAPIAuthSeen atomic.Bool
	httpServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		authorized := request.Header.Get("X-Fixture-Auth") == adapterE3AuthSecret
		switch {
		case request.URL.Path == "/mcp":
			if authorized {
				remoteAuthSeen.Store(true)
			}
			remoteHandler.ServeHTTP(w, request)
		case request.URL.Path == "/openapi.json":
			openAPIDocumentFetches.Add(1)
			if authorized {
				openAPIAuthSeen.Store(true)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(openAPIFixtureDocument))
		case strings.HasPrefix(request.URL.Path, "/api/items/"):
			openAPICalls.Add(1)
			if authorized {
				openAPIAuthSeen.Store(true)
			}
			body, _ := io.ReadAll(request.Body)
			if !authorized || request.Method != http.MethodPost || request.URL.Query().Get("verbose") != "true" ||
				request.Header.Get("X-Mode") != "safe" || string(body) != `{"name":"adapter-e3-payload-secret"}` {
				http.Error(w, "invalid controlled fixture request", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accepted":true}`))
		default:
			http.NotFound(w, request)
		}
	})}
	go func() { _ = httpServer.Serve(httpListener) }()
	t.Cleanup(func() { _ = httpServer.Shutdown(context.Background()) })

	var embeddedCalls atomic.Int32
	var uploadedCalls atomic.Int32
	var embeddedAuthSeen atomic.Bool
	var uploadedAuthSeen atomic.Bool
	grpcServer := grpc.NewServer()
	grpcServer.RegisterService(&grpc.ServiceDesc{
		ServiceName: "dars.gateway.v1.GatewayUtility", HandlerType: (*dynamicEchoService)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "Echo", Handler: adapterE3GRPCHandler(
			"/dars.gateway.v1.GatewayUtility/Echo", embeddedMethod, &embeddedCalls, &embeddedAuthSeen,
		)}},
	}, struct{}{})
	grpcServer.RegisterService(&grpc.ServiceDesc{
		ServiceName: "fixture.v1.EchoService", HandlerType: (*dynamicEchoService)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "Echo", Handler: adapterE3GRPCHandler(
			"/fixture.v1.EchoService/Echo", uploadedMethod, &uploadedCalls, &uploadedAuthSeen,
		)}},
	}, struct{}{})
	go func() { _ = grpcServer.Serve(grpcListener) }()
	t.Cleanup(grpcServer.Stop)

	httpPort := uint16(httpListener.Addr().(*net.TCPAddr).Port)
	grpcPort := uint16(grpcListener.Addr().(*net.TCPAddr).Port)
	resolver := &staticResolver{answers: map[string][][]netip.Addr{
		"adapter-fixture.internal": {{privateAddress}},
		"grpc-fixture.internal":    {{privateAddress}},
	}}
	config := Config{
		AllowedPorts:         []uint16{httpPort, grpcPort},
		PrivateCIDRAllowlist: []netip.Prefix{netip.PrefixFrom(privateAddress, privateAddress.BitLen())},
		ConnectTimeout:       time.Second,
		InvocationTimeout:    2 * time.Second,
		ChannelIdleTimeout:   50 * time.Millisecond,
		MaxRequestBodyBytes:  1 << 20,
		MaxResponseBytes:     1 << 20,
		MaxArtifactBytes:     1 << 20,
		MaxRedirects:         1,
		TaskConcurrency:      4,
		SourceConcurrency:    4,
	}
	egress := NewEgressPolicy(config, EgressPolicyOptions{Resolver: resolver})
	codec, err := NewSourceSecretCodec("adapter-e3", map[string][]byte{"adapter-e3": bytes.Repeat([]byte{37}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	remoteAdapter := NewRemoteMCPAdapter(config, egress, codec)
	grpcAdapter := NewGRPCAdapter(config, egress, descriptors, codec)
	t.Cleanup(func() { _ = grpcAdapter.Close() })
	openAPIAdapter := NewOpenAPIAdapter(config, egress, codec)
	control := NewControlPlane(pool, ControlPlaneOptions{
		SecretCodec: codec, ProviderAllowlist: map[string]bool{"codex": true}, MaxArtifactBytes: config.MaxArtifactBytes,
		RemoteMCP: remoteAdapter, GRPC: grpcAdapter, OpenAPI: openAPIAdapter,
	})

	base := createRegistryFixture(t, pool)
	httpEndpoint := "http://adapter-fixture.internal:" + strconv.Itoa(int(httpPort))
	grpcEndpoint := "grpc://grpc-fixture.internal:" + strconv.Itoa(int(grpcPort))
	authDocument := `{"kind":"headers","headers":{"X-Fixture-Auth":"` + adapterE3AuthSecret + `"}}`

	remoteSource, remoteTools := createReadyAdapterE3Source(t, control, base.workspaceID, base.userID, fmt.Sprintf(`{
		"name":"remote-e3","kind":"remote_mcp","endpoint":%q,"transport_config":{},"auth":%s
	}`, httpEndpoint+"/mcp", authDocument))
	embeddedSource, embeddedTools := createReadyAdapterE3Source(t, control, base.workspaceID, base.userID, fmt.Sprintf(`{
		"name":"embedded-e3","kind":"grpc","endpoint":%q,
		"transport_config":{"builtin_descriptor":"dars_gateway_v1"},"auth":%s
	}`, grpcEndpoint, authDocument))
	uploadedSource, uploadedTools := createUploadedProtoE3Source(t, control, base.workspaceID, base.userID, grpcEndpoint, authDocument)
	openAPISource, openAPITools := createReadyAdapterE3Source(t, control, base.workspaceID, base.userID, fmt.Sprintf(`{
		"name":"openapi-e3","kind":"openapi","endpoint":%q,
		"transport_config":{"document_url":%q},"auth":%s
	}`, httpEndpoint+"/api", httpEndpoint+"/openapi.json", authDocument))

	allTools := [][]toolDTO{remoteTools, embeddedTools, uploadedTools, openAPITools}
	selectionItems := make([]map[string]string, 0, 4)
	for _, tools := range allTools {
		if len(tools) != 1 {
			t.Fatalf("adapter Source discovered %d tools, want 1", len(tools))
		}
		selectionItems = append(selectionItems, map[string]string{
			"tool_definition_id": tools[0].ID,
			"exported_name":      tools[0].PublicName,
		})
	}
	encodedIDs, _ := json.Marshal(map[string]any{"items": selectionItems})
	bundle := publishBundleForTest(t, control, base.agentID, string(encodedIDs), base.workspaceID, base.userID, http.StatusCreated)
	if len(bundle.Items) != 4 {
		t.Fatalf("adapter Bundle item count = %d", len(bundle.Items))
	}

	taskID := uuid.NewString()
	token := "dat_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO agent_task_queue (id, workspace_id, agent_id, runtime_id, issue_id, status, tool_bundle_id)
		VALUES ($1, $2, $3, $4, $5, 'dispatched', $6)
	`, taskID, base.workspaceID, base.agentID, base.runtimeID, base.issueID, bundle.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, tool_bundle_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + interval '1 hour')
	`, auth.HashToken(token), taskID, base.agentID, base.workspaceID, base.userID, bundle.ID); err != nil {
		t.Fatal(err)
	}

	registry := NewDatabaseRegistry(pool, DatabaseRegistryOptions{
		Config: config, ProviderAllowlist: map[string]bool{"codex": true},
		RemoteMCP: remoteAdapter, GRPC: grpcAdapter, OpenAPI: openAPIAdapter,
	})
	core := lightweightapi.New(pool, nil, lightweightapi.Config{})
	gatewayServer := serveAuthenticatedGateway(t, core, newDatabaseGateway(t, config, registry))
	session := connectMCPClient(t, gatewayServer.URL, bundle.ID, token)
	defer session.Close()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatalf("adapter Bundle tools=%v err=%v", toolNames(tools.Tools), err)
	}
	callAdapterE3Tool(t, session, remoteTools[0].PublicName, map[string]any{"value": "adapter-e3-payload-secret"})
	callAdapterE3Tool(t, session, embeddedTools[0].PublicName, map[string]any{"value": "adapter-e3-payload-secret"})
	callAdapterE3Tool(t, session, uploadedTools[0].PublicName, map[string]any{"message": "adapter-e3-payload-secret", "count": "3"})
	callAdapterE3Tool(t, session, openAPITools[0].PublicName, map[string]any{
		"path": map[string]any{"id": "adapter-e3"}, "query": map[string]any{"verbose": true},
		"header": map[string]any{"X-Mode": "safe"}, "body": map[string]any{"name": "adapter-e3-payload-secret"},
	})

	if remoteCalls.Load() != 1 || embeddedCalls.Load() != 1 || uploadedCalls.Load() != 1 || openAPICalls.Load() != 1 {
		t.Fatalf("fixture call correlation remote=%d embedded=%d uploaded=%d openapi=%d",
			remoteCalls.Load(), embeddedCalls.Load(), uploadedCalls.Load(), openAPICalls.Load())
	}
	if !remoteAuthSeen.Load() || !embeddedAuthSeen.Load() || !uploadedAuthSeen.Load() || !openAPIAuthSeen.Load() || openAPIDocumentFetches.Load() != 1 {
		t.Fatalf("fixture auth/document correlation remote=%t embedded=%t uploaded=%t openapi=%t documents=%d",
			remoteAuthSeen.Load(), embeddedAuthSeen.Load(), uploadedAuthSeen.Load(), openAPIAuthSeen.Load(), openAPIDocumentFetches.Load())
	}

	expectedSources := map[string]sourceDTO{
		"remote_mcp": remoteSource, "grpc_embedded": embeddedSource, "grpc_uploaded": uploadedSource, "openapi": openAPISource,
	}
	assertAdapterE3DatabaseCorrelation(t, pool, base, taskID, token, bundle, expectedSources)
}

func createReadyAdapterE3Source(
	t *testing.T,
	control *ControlPlane,
	workspaceID string,
	userID string,
	body string,
) (sourceDTO, []toolDTO) {
	t.Helper()
	created := callControlHandler(t, control.CreateSource, http.MethodPost, "/api/tool-sources", body, workspaceID, userID, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create adapter Source = %d %s", created.Code, created.Body.String())
	}
	var source sourceDTO
	decodeRecorder(t, created, &source)
	return validateEnableAdapterE3Source(t, control, workspaceID, userID, source)
}

func createUploadedProtoE3Source(
	t *testing.T,
	control *ControlPlane,
	workspaceID string,
	userID string,
	endpoint string,
	authDocument string,
) (sourceDTO, []toolDTO) {
	t.Helper()
	created := callControlHandler(t, control.CreateSource, http.MethodPost, "/api/tool-sources", fmt.Sprintf(`{
		"name":"uploaded-e3","kind":"grpc","endpoint":%q,"transport_config":{"root_file":"fixture.proto"}
	}`, endpoint), workspaceID, userID, nil)
	if created.Code != http.StatusCreated {
		t.Fatalf("create uploaded Proto Source = %d %s", created.Code, created.Body.String())
	}
	var source sourceDTO
	decodeRecorder(t, created, &source)
	uploadBody, _ := json.Marshal(map[string]any{
		"media_type": "text/x-proto", "content_base64": base64.StdEncoding.EncodeToString([]byte(unaryFixtureProto)),
	})
	uploaded := callControlHandler(t, control.UploadSourceArtifact, http.MethodPost,
		"/api/tool-sources/"+source.ID+"/artifacts", string(uploadBody), workspaceID, userID, map[string]string{"sourceId": source.ID})
	if uploaded.Code != http.StatusCreated {
		t.Fatalf("upload Proto artifact = %d %s", uploaded.Code, uploaded.Body.String())
	}
	var artifact struct {
		ID string `json:"id"`
	}
	decodeRecorder(t, uploaded, &artifact)
	updated := callControlHandler(t, control.UpdateSource, http.MethodPut, "/api/tool-sources/"+source.ID, fmt.Sprintf(`{
		"endpoint":%q,"transport_config":{"root_file":"fixture.proto"},"artifact_id":%q,"auth":%s
	}`, endpoint, artifact.ID, authDocument), workspaceID, userID, map[string]string{"sourceId": source.ID})
	if updated.Code != http.StatusCreated {
		t.Fatalf("stage uploaded Proto revision = %d %s", updated.Code, updated.Body.String())
	}
	decodeRecorder(t, updated, &source)
	return validateEnableAdapterE3Source(t, control, workspaceID, userID, source)
}

func validateEnableAdapterE3Source(
	t *testing.T,
	control *ControlPlane,
	workspaceID string,
	userID string,
	source sourceDTO,
) (sourceDTO, []toolDTO) {
	t.Helper()
	if source.Revision == nil {
		t.Fatal("staged adapter Source has no revision")
	}
	validated := callControlHandler(t, control.ValidateSource, http.MethodPost,
		"/api/tool-sources/"+source.ID+"/validate", `{"revision_id":"`+source.Revision.ID+`"}`,
		workspaceID, userID, map[string]string{"sourceId": source.ID})
	if validated.Code != http.StatusOK {
		t.Fatalf("validate adapter Source = %d %s", validated.Code, validated.Body.String())
	}
	decodeRecorder(t, validated, &source)
	enabled := callControlHandler(t, control.EnableSource, http.MethodPost,
		"/api/tool-sources/"+source.ID+"/enable", `{}`, workspaceID, userID, map[string]string{"sourceId": source.ID})
	if enabled.Code != http.StatusOK {
		t.Fatalf("enable adapter Source = %d %s", enabled.Code, enabled.Body.String())
	}
	listed := callControlHandler(t, control.ListSourceTools, http.MethodGet,
		"/api/tool-sources/"+source.ID+"/tools", "", workspaceID, userID, map[string]string{"sourceId": source.ID})
	if listed.Code != http.StatusOK {
		t.Fatalf("list adapter Source tools = %d %s", listed.Code, listed.Body.String())
	}
	var tools []toolDTO
	decodeRecorder(t, listed, &tools)
	return source, tools
}

func callAdapterE3Tool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil || result == nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("adapter E3 call %s result=%+v err=%v", name, result, err)
	}
}

func assertAdapterE3DatabaseCorrelation(
	t *testing.T,
	pool *pgxpool.Pool,
	base registryFixture,
	taskID string,
	token string,
	bundle bundleDTO,
	expectedSources map[string]sourceDTO,
) {
	t.Helper()
	ctx := context.Background()
	var taskPin string
	var tokenPin string
	var storedTokenHash string
	if err := pool.QueryRow(ctx, `SELECT tool_bundle_id FROM agent_task_queue WHERE id = $1 AND workspace_id = $2`, taskID, base.workspaceID).Scan(&taskPin); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT tool_bundle_id, token_hash FROM task_token WHERE task_id = $1`, taskID).Scan(&tokenPin, &storedTokenHash); err != nil {
		t.Fatal(err)
	}
	if taskPin != bundle.ID || tokenPin != bundle.ID || storedTokenHash == token || strings.Contains(storedTokenHash, "dat_") {
		t.Fatalf("Task/Token pin correlation task=%q token=%q bundle=%q", taskPin, tokenPin, bundle.ID)
	}

	items, err := lwdb.New(pool).ListToolBundleItems(ctx, lwdb.ListToolBundleItemsParams{
		WorkspaceID: mustParseTestUUID(t, base.workspaceID), BundleID: bundle.ID,
	})
	if err != nil || len(items) != 4 {
		t.Fatalf("Bundle item correlation count=%d err=%v", len(items), err)
	}
	expectedByIdentity := make(map[string]sourceDTO, 4)
	for _, source := range expectedSources {
		if source.CurrentRevision == nil || source.Revision == nil || *source.CurrentRevision != source.Revision.ID || source.Revision.Status != "ready" {
			t.Fatalf("Source revision is not ready/current: %+v", source)
		}
		expectedByIdentity[source.ID+"|"+source.Revision.ID] = source
	}
	for _, item := range items {
		identity := uuidString(item.SourceID) + "|" + uuidString(item.SourceRevisionID)
		source, ok := expectedByIdentity[identity]
		if !ok {
			t.Fatalf("Bundle item references unexpected Source revision %s", identity)
		}
		if (source.Kind == "grpc" || source.Kind == "openapi") && !item.ArtifactID.Valid {
			t.Fatalf("%s Bundle item lost retained artifact", source.Kind)
		}
		delete(expectedByIdentity, identity)
	}
	if len(expectedByIdentity) != 0 {
		t.Fatalf("Bundle did not materialize all Source revisions: %+v", expectedByIdentity)
	}

	rows, err := pool.Query(ctx, `
		SELECT details::text FROM activity_log
		WHERE workspace_id = $1 AND action = 'mcp_gateway_tool_invoked' AND details ->> 'task_id' = $2
		ORDER BY created_at
	`, base.workspaceID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	auditCount := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		auditCount++
		if strings.Contains(raw, adapterE3AuthSecret) || strings.Contains(raw, "adapter-e3-payload-secret") ||
			strings.Contains(raw, `"arguments"`) || strings.Contains(raw, `"result"`) {
			t.Fatalf("adapter invocation audit leaked sensitive payload: %s", raw)
		}
		var details map[string]any
		if json.Unmarshal([]byte(raw), &details) != nil || details["bundle_id"] != bundle.ID || details["task_id"] != taskID || details["outcome"] != "succeeded" {
			t.Fatalf("adapter invocation audit lost correlation: %s", raw)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if auditCount != 4 {
		t.Fatalf("adapter invocation audit count = %d, want 4", auditCount)
	}

	var secretRows string
	if err := pool.QueryRow(ctx, `
		SELECT coalesce(string_agg(envelope::text, ''), '') FROM tool_source_secret WHERE workspace_id = $1
	`, base.workspaceID).Scan(&secretRows); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(secretRows, adapterE3AuthSecret) {
		t.Fatal("Source credential was persisted in plaintext")
	}
}

func adapterE3GRPCHandler(
	fullMethod string,
	method protoreflect.MethodDescriptor,
	calls *atomic.Int32,
	authSeen *atomic.Bool,
) grpc.MethodHandler {
	return func(
		srv any,
		ctx context.Context,
		decode func(any) error,
		interceptor grpc.UnaryServerInterceptor,
	) (any, error) {
		request := dynamicpb.NewMessage(method.Input())
		if err := decode(request); err != nil {
			return nil, err
		}
		handler := func(ctx context.Context, raw any) (any, error) {
			calls.Add(1)
			incoming, _ := metadata.FromIncomingContext(ctx)
			values := incoming.Get("x-fixture-auth")
			if len(values) != 1 || values[0] != adapterE3AuthSecret {
				return nil, status.Error(codes.Unauthenticated, "missing controlled fixture metadata")
			}
			authSeen.Store(true)
			message := raw.(*dynamicpb.Message)
			response := dynamicpb.NewMessage(method.Output())
			switch fullMethod {
			case "/dars.gateway.v1.GatewayUtility/Echo":
				value := message.Get(method.Input().Fields().ByName("value")).String()
				response.Set(method.Output().Fields().ByName("value"), protoreflect.ValueOfString(value))
			case "/fixture.v1.EchoService/Echo":
				value := message.Get(method.Input().Fields().ByName("message")).String()
				count := message.Get(method.Input().Fields().ByName("count")).Int()
				response.Set(method.Output().Fields().ByName("echoed"), protoreflect.ValueOfString(value+":"+strconv.FormatInt(count, 10)))
			default:
				return nil, status.Error(codes.Unimplemented, "unexpected controlled fixture method")
			}
			return response, nil
		}
		if interceptor == nil {
			return handler(ctx, request)
		}
		return interceptor(ctx, request, &grpc.UnaryServerInfo{Server: srv, FullMethod: fullMethod}, handler)
	}
}

func mustParseTestUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := util.ParseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
