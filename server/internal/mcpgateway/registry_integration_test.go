package mcpgateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kms9/dars/internal/auth"
	"github.com/kms9/dars/internal/lightweightapi"
	"github.com/kms9/dars/internal/util"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDatabaseRegistryOfficialMCPClientOnFreshCheckDatabase(t *testing.T) {
	pool := openControlPlaneCheckDatabase(t)
	fixture := createRegistryFixture(t, pool)

	config := Config{
		InvocationTimeout: 2 * time.Second, MaxRequestBodyBytes: 1 << 20, MaxResponseBytes: 1 << 20,
		TaskConcurrency: 2, SourceConcurrency: 2,
	}
	cancelObserved := make(chan struct{})
	var cancelOnce sync.Once
	registry := NewDatabaseRegistry(pool, DatabaseRegistryOptions{
		Config: config, ProviderAllowlist: map[string]bool{"codex": true},
		ServerLocal: map[string]ServerLocalHandler{
			"wait_for_cancel": func(ctx context.Context, _ map[string]any) (any, error) {
				<-ctx.Done()
				cancelOnce.Do(func() { close(cancelObserved) })
				return nil, ctx.Err()
			},
		},
	})
	core := lightweightapi.New(pool, nil, lightweightapi.Config{})
	firstGateway := newDatabaseGateway(t, config, registry)
	firstServer := serveAuthenticatedGateway(t, core, firstGateway)

	session := connectMCPClient(t, firstServer.URL, fixture.firstBundleID, fixture.token)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 2 || tools.Tools[0].Name != "fixture.wait" || tools.Tools[1].Name != "skill.fixture.echo" {
		t.Fatalf("Task-pinned tools = %+v, want B1 deterministic order", toolNames(tools.Tools))
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "skill.fixture.echo", Arguments: map[string]any{"value": "ok"},
	})
	if err != nil || result.IsError {
		t.Fatalf("echo call result=%+v err=%v", result, err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["registryKey"] != "echo" {
		t.Fatalf("structured result = %#v", result.StructuredContent)
	}
	var auditDetails string
	if err := pool.QueryRow(context.Background(), `
		SELECT details::text FROM activity_log
		WHERE workspace_id = $1 AND action = 'mcp_gateway_tool_invoked'
		  AND details ->> 'tool_name' = 'skill.fixture.echo'
		ORDER BY created_at DESC LIMIT 1
	`, fixture.workspaceID).Scan(&auditDetails); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auditDetails, `"value"`) || strings.Contains(auditDetails, `"ok"`) ||
		!strings.Contains(auditDetails, `"bundle_id"`) || !strings.Contains(auditDetails, `"canonical_tool_name": "fixture.echo"`) ||
		!strings.Contains(auditDetails, `"upstream_tool_name": "fixture/echo"`) {
		t.Fatalf("invocation audit is not metadata-only: %s", auditDetails)
	}
	invalid, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "skill.fixture.echo", Arguments: map[string]any{"value": 42},
	})
	if err != nil || !invalid.IsError {
		t.Fatalf("schema-invalid call result=%+v err=%v", invalid, err)
	}

	cancelCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _ = session.CallTool(cancelCtx, &mcp.CallToolParams{Name: "fixture.wait", Arguments: map[string]any{}})
	select {
	case <-cancelObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("request cancellation did not reach the Server-local invoker")
	}
	_ = session.Close()

	canonicalSession := connectMCPClient(t, firstServer.URL, fixture.firstBundleID, fixture.token)
	canonicalResult, canonicalErr := canonicalSession.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "fixture.echo", Arguments: map[string]any{"value": "must-not-call"},
	})
	if canonicalErr == nil && canonicalResult != nil && !canonicalResult.IsError {
		t.Fatalf("canonical name unexpectedly acted as an alias: %+v", canonicalResult)
	}
	_ = canonicalSession.Close()

	// A second Gateway process projects the same Task pin from PostgreSQL.
	secondRegistry := NewDatabaseRegistry(pool, DatabaseRegistryOptions{
		Config: config, ProviderAllowlist: map[string]bool{"codex": true},
	})
	secondGateway := newDatabaseGateway(t, config, secondRegistry)
	secondServer := serveAuthenticatedGateway(t, core, secondGateway)
	secondSession := connectMCPClient(t, secondServer.URL, fixture.firstBundleID, fixture.token)
	secondTools, err := secondSession.ListTools(context.Background(), nil)
	if err != nil || len(secondTools.Tools) != 2 {
		t.Fatalf("second replica tools=%+v err=%v", toolNames(secondTools.Tools), err)
	}
	_ = secondSession.Close()

	// The same token cannot address the Agent's newer head Bundle, and neither a
	// syntactically valid unknown token nor an unknown Bundle reveals the pin.
	assertMCPConnectFails(t, firstServer.URL, fixture.secondBundleID, fixture.token)
	assertMCPConnectFails(t, firstServer.URL, fixture.firstBundleID, "dat_"+strings.ReplaceAll(uuid.NewString(), "-", ""))
	assertMCPConnectFails(t, firstServer.URL, "tb_missing_bundle_12345", fixture.token)
	if _, err := pool.Exec(context.Background(), `UPDATE agent_runtime SET provider = 'unsupported-fixture' WHERE id = $1 AND workspace_id = $2`, fixture.runtimeID, fixture.workspaceID); err != nil {
		t.Fatal(err)
	}
	assertMCPConnectFails(t, firstServer.URL, fixture.firstBundleID, fixture.token)
	if _, err := pool.Exec(context.Background(), `UPDATE agent_runtime SET provider = 'codex' WHERE id = $1 AND workspace_id = $2`, fixture.runtimeID, fixture.workspaceID); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE tool_source SET enabled = false WHERE id = $1 AND workspace_id = $2`, fixture.sourceID, fixture.workspaceID); err != nil {
		t.Fatal(err)
	}
	disabledSession := connectMCPClient(t, firstServer.URL, fixture.firstBundleID, fixture.token)
	if _, err := disabledSession.CallTool(context.Background(), &mcp.CallToolParams{Name: "skill.fixture.echo", Arguments: map[string]any{"value": "blocked"}}); err == nil {
		t.Fatal("disabled Source still accepted tools/call")
	}
	_ = disabledSession.Close()

	if _, err := pool.Exec(context.Background(), `UPDATE tool_source SET enabled = true WHERE id = $1 AND workspace_id = $2`, fixture.sourceID, fixture.workspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE tool_bundle SET status = 'revoked', revoked_at = now() WHERE id = $1 AND workspace_id = $2`, fixture.firstBundleID, fixture.workspaceID); err != nil {
		t.Fatal(err)
	}
	assertMCPConnectFails(t, firstServer.URL, fixture.firstBundleID, fixture.token)

	var storedHash string
	if err := pool.QueryRow(context.Background(), `SELECT token_hash FROM task_token WHERE task_id = $1`, fixture.taskID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == fixture.token || strings.Contains(storedHash, "dat_") {
		t.Fatal("Task token was persisted in plaintext")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE agent_task_queue SET status = 'completed', completed_at = now() WHERE id = $1 AND workspace_id = $2`, fixture.taskID, fixture.workspaceID); err != nil {
		t.Fatal(err)
	}
	assertMCPConnectFails(t, firstServer.URL, fixture.firstBundleID, fixture.token)
}

func TestAgentHeadSwitchAndTaskPinConcurrencyOnFreshCheckDatabase(t *testing.T) {
	pool := openControlPlaneCheckDatabase(t)
	fixture := createRegistryFixture(t, pool)
	workspaceID, err := util.ParseUUID(fixture.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	agentID, _ := util.ParseUUID(fixture.agentID)
	userID, _ := util.ParseUUID(fixture.userID)
	start := make(chan struct{})
	errorsCh := make(chan error, 96)
	taskIDs := make(chan string, 48)
	var wait sync.WaitGroup

	wait.Add(1)
	go func() {
		defer wait.Done()
		<-start
		for index := 0; index < 48; index++ {
			target := fixture.firstBundleID
			if index%2 == 1 {
				target = fixture.secondBundleID
			}
			tx, err := pool.Begin(context.Background())
			if err != nil {
				errorsCh <- err
				return
			}
			q := lwdb.New(tx)
			if _, err = q.LockAgent(context.Background(), lwdb.LockAgentParams{ID: agentID, WorkspaceID: workspaceID}); err == nil {
				_, err = q.UpsertAgentToolBundleHead(context.Background(), lwdb.UpsertAgentToolBundleHeadParams{
					WorkspaceID: workspaceID, AgentID: agentID, BundleID: target, UpdatedBy: userID,
				})
			}
			if err == nil {
				err = tx.Commit(context.Background())
			} else {
				_ = tx.Rollback(context.Background())
			}
			if err != nil {
				errorsCh <- err
				return
			}
		}
	}()

	for index := 0; index < 48; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			taskID := uuid.NewString()
			tx, err := pool.Begin(context.Background())
			if err != nil {
				errorsCh <- err
				return
			}
			q := lwdb.New(tx)
			if _, err = q.LockAgent(context.Background(), lwdb.LockAgentParams{ID: agentID, WorkspaceID: workspaceID}); err == nil {
				_, err = tx.Exec(context.Background(), `
					INSERT INTO agent_task_queue (id, workspace_id, agent_id, runtime_id, issue_id, status, initiator_user_id, tool_bundle_id)
					SELECT $1, $2, $3, $4, $5, 'queued', $6, head.bundle_id
					FROM agent_tool_bundle_head head
					WHERE head.workspace_id = $2 AND head.agent_id = $3
				`, taskID, fixture.workspaceID, fixture.agentID, fixture.runtimeID, fixture.issueID, fixture.userID)
			}
			if err == nil {
				err = tx.Commit(context.Background())
			} else {
				_ = tx.Rollback(context.Background())
			}
			if err != nil {
				errorsCh <- err
				return
			}
			taskIDs <- taskID
		}()
	}
	close(start)
	wait.Wait()
	close(errorsCh)
	close(taskIDs)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}

	projector := NewClaimProjector(Config{PublicURL: "https://dars.example"}, map[string]bool{"codex": true})
	seen := 0
	for taskID := range taskIDs {
		seen++
		var pin string
		if err := pool.QueryRow(context.Background(), `SELECT tool_bundle_id FROM agent_task_queue WHERE id = $1`, taskID).Scan(&pin); err != nil {
			t.Fatal(err)
		}
		expectedCount := 0
		switch pin {
		case fixture.firstBundleID:
			expectedCount = 2
		case fixture.secondBundleID:
			expectedCount = 3
		default:
			t.Fatalf("Task observed mixed/unknown Bundle pin %q", pin)
		}
		var itemCount int
		if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM tool_bundle_item WHERE workspace_id = $1 AND bundle_id = $2`, fixture.workspaceID, pin).Scan(&itemCount); err != nil {
			t.Fatal(err)
		}
		if itemCount != expectedCount {
			t.Fatalf("Bundle %s item count = %d, want %d", pin, itemCount, expectedCount)
		}
		token := "dat_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, tool_bundle_id, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, now() + interval '1 hour')
		`, auth.HashToken(token), taskID, fixture.agentID, fixture.workspaceID, fixture.userID, pin); err != nil {
			t.Fatal(err)
		}
		document, err := projector.ProjectClaimMCPConfig(pin, "codex", token)
		if err != nil || !strings.Contains(string(document), "/bundles/"+pin+"/mcp") || !strings.Contains(string(document), "Bearer "+token) {
			t.Fatalf("Task %s Claim projection = %s err=%v", taskID, document, err)
		}
	}
	if seen != 48 {
		t.Fatalf("verified %d Tasks, want 48", seen)
	}
}

type registryFixture struct {
	workspaceID    string
	sourceID       string
	taskID         string
	firstBundleID  string
	secondBundleID string
	token          string
	userID         string
	runtimeID      string
	agentID        string
	issueID        string
}

func createRegistryFixture(t *testing.T, pool *pgxpool.Pool) registryFixture {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	workspaceID := uuid.NewString()
	runtimeID := uuid.NewString()
	agentID := uuid.NewString()
	issueID := uuid.NewString()
	taskID := uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]

	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO "user" (id, name, email) VALUES ($1, 'Registry User', $2)`, []any{userID, "registry-" + suffix + "@example.test"}},
		{`INSERT INTO workspace (id, name, slug, issue_prefix) VALUES ($1, 'Registry Workspace', $2, $3)`, []any{workspaceID, "registry-" + suffix, "R" + strings.ToUpper(suffix[:5])}},
		{`INSERT INTO agent_runtime (id, workspace_id, daemon_id, name, provider, status, owner_id) VALUES ($1, $2, $3, 'Registry Runtime', 'codex', 'online', $4)`, []any{runtimeID, workspaceID, "daemon-" + suffix, userID}},
		{`INSERT INTO agent (id, workspace_id, runtime_id, owner_id, name, status, permission_mode) VALUES ($1, $2, $3, $4, 'Registry Agent', 'idle', 'private')`, []any{agentID, workspaceID, runtimeID, userID}},
		{`INSERT INTO issue (id, workspace_id, title, status, assignee_type, assignee_id, creator_type, creator_id, number) VALUES ($1, $2, 'Registry Issue', 'in_progress', 'agent', $3, 'member', $4, 1)`, []any{issueID, workspaceID, agentID, userID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("prepare registry fixture: %v", err)
		}
	}

	control := NewControlPlane(pool, ControlPlaneOptions{
		ProviderAllowlist: map[string]bool{"codex": true}, MaxArtifactBytes: 1024,
	})
	createBody := fmt.Sprintf(`{
		"name":"registry-%s",
		"kind":"server_local",
		"transport_config":{},
		"tools":[
			{"public_name":"fixture.echo","upstream_name":"fixture/echo","description":"echo","input_schema":{"type":"object","properties":{"value":{"type":"string"}},"required":["value"],"additionalProperties":false},"operation_metadata":{"registry_key":"echo"}},
			{"public_name":"fixture.second","upstream_name":"fixture/second","description":"second","input_schema":{"type":"object"},"operation_metadata":{"registry_key":"second"}},
			{"public_name":"fixture.wait","upstream_name":"fixture/wait","description":"wait","input_schema":{"type":"object","additionalProperties":false},"operation_metadata":{"registry_key":"wait_for_cancel"}}
		]
	}`, suffix)
	createRecorder := callControlHandler(t, control.CreateSource, http.MethodPost, "/api/tool-sources", createBody, workspaceID, userID, nil)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create registry Source = %d %s", createRecorder.Code, createRecorder.Body.String())
	}
	var source sourceDTO
	decodeRecorder(t, createRecorder, &source)
	validateRecorder := callControlHandler(t, control.ValidateSource, http.MethodPost, "/api/tool-sources/"+source.ID+"/validate", `{"revision_id":"`+source.Revision.ID+`"}`, workspaceID, userID, map[string]string{"sourceId": source.ID})
	if validateRecorder.Code != http.StatusOK {
		t.Fatalf("validate registry Source = %d %s", validateRecorder.Code, validateRecorder.Body.String())
	}
	enableRecorder := callControlHandler(t, control.EnableSource, http.MethodPost, "/api/tool-sources/"+source.ID+"/enable", `{}`, workspaceID, userID, map[string]string{"sourceId": source.ID})
	if enableRecorder.Code != http.StatusOK {
		t.Fatalf("enable registry Source = %d %s", enableRecorder.Code, enableRecorder.Body.String())
	}
	toolsRecorder := callControlHandler(t, control.ListSourceTools, http.MethodGet, "/api/tool-sources/"+source.ID+"/tools", "", workspaceID, userID, map[string]string{"sourceId": source.ID})
	if toolsRecorder.Code != http.StatusOK {
		t.Fatalf("list registry tools = %d %s", toolsRecorder.Code, toolsRecorder.Body.String())
	}
	var tools []toolDTO
	decodeRecorder(t, toolsRecorder, &tools)
	definitionIDs := make(map[string]string, len(tools))
	for _, tool := range tools {
		definitionIDs[tool.PublicName] = tool.ID
	}
	firstBody := fmt.Sprintf(`{"items":[{"tool_definition_id":%q,"exported_name":"skill.fixture.echo"},{"tool_definition_id":%q,"exported_name":"fixture.wait"}]}`, definitionIDs["fixture.echo"], definitionIDs["fixture.wait"])
	firstBundle := publishBundleForTest(t, control, agentID, firstBody, workspaceID, userID, http.StatusCreated)

	if _, err := pool.Exec(ctx, `
		INSERT INTO agent_task_queue (id, workspace_id, agent_id, runtime_id, issue_id, status, tool_bundle_id)
		VALUES ($1, $2, $3, $4, $5, 'dispatched', $6)
	`, taskID, workspaceID, agentID, runtimeID, issueID, firstBundle.ID); err != nil {
		t.Fatalf("insert Bundle-pinned Task: %v", err)
	}
	secondBody := fmt.Sprintf(`{"items":[{"tool_definition_id":%q,"exported_name":"skill.fixture.echo.v2"},{"tool_definition_id":%q,"exported_name":"fixture.wait"},{"tool_definition_id":%q,"exported_name":"fixture.second"}]}`, definitionIDs["fixture.echo"], definitionIDs["fixture.wait"], definitionIDs["fixture.second"])
	secondBundle := publishBundleForTest(t, control, agentID, secondBody, workspaceID, userID, http.StatusCreated)

	token := "dat_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, tool_bundle_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, now() + interval '1 hour')
	`, auth.HashToken(token), taskID, agentID, workspaceID, userID, firstBundle.ID); err != nil {
		t.Fatalf("insert Task token: %v", err)
	}
	return registryFixture{
		workspaceID: workspaceID, sourceID: source.ID, taskID: taskID,
		firstBundleID: firstBundle.ID, secondBundleID: secondBundle.ID, token: token,
		userID: userID, runtimeID: runtimeID, agentID: agentID, issueID: issueID,
	}
}

func newDatabaseGateway(t *testing.T, config Config, registry *DatabaseRegistry) *Gateway {
	t.Helper()
	gateway, err := New(config, Dependencies{
		IdentityResolver: IdentityResolverFunc(func(ctx context.Context) (Identity, bool) {
			principal, ok := lightweightapi.PrincipalFromContext(ctx)
			if !ok {
				return Identity{}, false
			}
			return Identity{
				UserID: principal.UserID, WorkspaceID: principal.WorkspaceID, AgentID: principal.AgentID,
				TaskID: principal.TaskID, ToolBundleID: principal.ToolBundleID,
			}, true
		}),
		Store: registry, Projector: registry, Version: "integration-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gateway.Close(context.Background()) })
	return gateway
}

func serveAuthenticatedGateway(t *testing.T, core *lightweightapi.Handler, gateway *Gateway) *httptest.Server {
	t.Helper()
	handler := core.TaskAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bundleID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/bundles/"), "/mcp")
		gateway.ServeHTTP(w, WithBundleID(r, bundleID))
	}))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func connectMCPClient(t *testing.T, serverURL, bundleID, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "registry-integration", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             serverURL + "/bundles/" + bundleID + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{token: token}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err != nil {
		t.Fatalf("connect MCP client: %v", err)
	}
	return session
}

func assertMCPConnectFails(t *testing.T, serverURL, bundleID, token string) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "registry-negative", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             serverURL + "/bundles/" + bundleID + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{token: token}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}, nil)
	if err == nil {
		_ = session.Close()
		t.Fatalf("MCP connect unexpectedly succeeded for Bundle %s", bundleID)
	}
}

type bearerRoundTripper struct {
	token string
}

func (transport bearerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	cloned.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(cloned)
}

func toolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
