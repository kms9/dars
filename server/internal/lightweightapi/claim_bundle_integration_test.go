package lightweightapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kms9/dars/internal/agentconfigsecret"
	"github.com/kms9/dars/internal/auth"
	"github.com/kms9/dars/internal/mcpgateway"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

func TestClaimBundleProjectionAndReclaimOnFreshCheckDatabase(t *testing.T) {
	pool := openClaimCheckDatabase(t)
	codec, err := agentconfigsecret.New("test", map[string][]byte{"test": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	fixture := createClaimBundleFixture(t, pool, codec)
	projector := mcpgateway.NewClaimProjector(mcpgateway.Config{PublicURL: "https://dars.example"}, map[string]bool{"codex": true})
	handler := New(pool, nil, Config{AgentSecrets: codec, ClaimMCPProjector: projector})

	first := buildClaimedTaskForTest(t, handler, fixture.bundleTaskID)
	assertGatewayOnlyClaimConfig(t, first.Agent.MCPConfig, fixture.firstBundleID, first.AuthToken)
	if strings.Contains(string(first.Agent.MCPConfig), "stored-existing") {
		t.Fatalf("Bundle Claim leaked stored Agent MCP entries: %s", first.Agent.MCPConfig)
	}
	encodedClaim, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedClaim), "tool_bundle_id") {
		t.Fatalf("Claim DTO shape added a Bundle field: %s", encodedClaim)
	}

	second := buildClaimedTaskForTest(t, handler, fixture.bundleTaskID)
	assertGatewayOnlyClaimConfig(t, second.Agent.MCPConfig, fixture.firstBundleID, second.AuthToken)
	if second.AuthToken == first.AuthToken {
		t.Fatal("re-Claim reused plaintext Task token")
	}
	var tokenCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM task_token
		WHERE task_id = $1 AND token_hash = $2 AND tool_bundle_id = $3
	`, fixture.bundleTaskID, auth.HashToken(second.AuthToken), fixture.firstBundleID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 1 {
		t.Fatalf("current re-Claim Token scope count = %d", tokenCount)
	}
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM task_token WHERE token_hash = $1`, auth.HashToken(first.AuthToken)).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 0 {
		t.Fatal("old re-Claim Token hash was not revoked")
	}

	legacy := buildClaimedTaskForTest(t, handler, fixture.legacyTaskID)
	var legacyDocument map[string]any
	if err := json.Unmarshal(legacy.Agent.MCPConfig, &legacyDocument); err != nil {
		t.Fatal(err)
	}
	servers, ok := legacyDocument["mcpServers"].(map[string]any)
	if !ok || len(servers) != 1 || servers["stored-existing"] == nil {
		t.Fatalf("non-Bundle Claim semantics changed: %s", legacy.Agent.MCPConfig)
	}
}

func TestGatewayProviderPolicyGatesTaskRuntimeAndClaimOnFreshCheckDatabase(t *testing.T) {
	pool := openClaimCheckDatabase(t)
	codec, err := agentconfigsecret.New("test", map[string][]byte{"test": bytes.Repeat([]byte{8}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	fixture := createClaimBundleFixture(t, pool, codec)
	denyProjector := mcpgateway.NewClaimProjector(mcpgateway.Config{PublicURL: "https://dars.example"}, map[string]bool{})
	denyHandler := New(pool, nil, Config{
		AgentSecrets: codec, ClaimMCPProjector: denyProjector, GatewayProviderPolicy: denyProjector,
	})
	workspaceID, _ := parseUUID(fixture.workspaceID)
	agentID, _ := parseUUID(fixture.agentID)
	runtimeID, _ := parseUUID(fixture.runtimeID)
	if err := denyHandler.requireGatewayProviderForNewTask(context.Background(), denyHandler.q, workspaceID, agentID, runtimeID); err == nil || err.Error() != "provider_mcp_unsupported" {
		t.Fatalf("unsupported Provider new-Task gate = %v", err)
	}

	issue, err := denyHandler.q.GetIssue(context.Background(), lwdb.GetIssueParams{ID: mustParseTestUUID(t, fixture.issueID), WorkspaceID: workspaceID})
	if err != nil {
		t.Fatal(err)
	}
	beforeTasks := countAgentTasksForTest(t, pool, fixture.agentID)
	_, _, err = denyHandler.enqueueInitialIssueTask(context.Background(), denyHandler.q, issue, "GP", mustParseTestUUID(t, fixture.userID), "manual", issueAssigneeResolution{
		agentID: agentID, runtimeID: runtimeID, displayType: "agent",
	})
	if err == nil || err.Error() != "provider_mcp_unsupported" {
		t.Fatalf("unsupported Provider enqueue gate = %v", err)
	}
	if after := countAgentTasksForTest(t, pool, fixture.agentID); after != beforeTasks {
		t.Fatalf("unsupported Provider created a Task: before=%d after=%d", beforeTasks, after)
	}

	unsupportedRuntimeID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO agent_runtime (id, workspace_id, daemon_id, name, provider, status, owner_id)
		VALUES ($1, $2, $3, 'Unsupported Runtime', 'unsupported-fixture', 'online', $4)
	`, unsupportedRuntimeID, fixture.workspaceID, "unsupported-"+unsupportedRuntimeID, fixture.userID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPatch, "/api/agents/"+fixture.agentID, strings.NewReader(`{"runtime_id":"`+unsupportedRuntimeID+`"}`))
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("agentId", fixture.agentID)
	ctx := context.WithValue(request.Context(), chi.RouteCtxKey, routeContext)
	ctx = context.WithValue(ctx, workspaceKey, fixture.workspaceID)
	ctx = context.WithValue(ctx, principalKey, Principal{Kind: principalJWT, UserID: fixture.userID})
	request = request.WithContext(ctx)
	recorder := httptest.NewRecorder()
	denyHandler.UpdateAgent(recorder, request)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "provider_mcp_unsupported") {
		t.Fatalf("unsupported Runtime update = %d %s", recorder.Code, recorder.Body.String())
	}

	assertClaimBuildFailsForTest(t, denyHandler, fixture.bundleTaskID)
	missingURLProjector := mcpgateway.NewClaimProjector(mcpgateway.Config{}, map[string]bool{"codex": true})
	missingURLHandler := New(pool, nil, Config{
		AgentSecrets: codec, ClaimMCPProjector: missingURLProjector, GatewayProviderPolicy: missingURLProjector,
	})
	assertClaimBuildFailsForTest(t, missingURLHandler, fixture.bundleTaskID)
}

type claimBundleFixture struct {
	bundleTaskID  string
	legacyTaskID  string
	firstBundleID string
	workspaceID   string
	userID        string
	runtimeID     string
	agentID       string
	issueID       string
}

func createClaimBundleFixture(t *testing.T, pool *pgxpool.Pool, codec *agentconfigsecret.Codec) claimBundleFixture {
	t.Helper()
	ctx := context.Background()
	userID := uuid.NewString()
	workspaceID := uuid.NewString()
	runtimeID := uuid.NewString()
	agentID := uuid.NewString()
	issueID := uuid.NewString()
	bundleTaskID := uuid.NewString()
	legacyTaskID := uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	firstBundleID := "tb_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	secondBundleID := "tb_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	emptyEnv, err := codec.EncryptCustomEnv(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	storedMCP, err := codec.EncryptMCPConfig(json.RawMessage(`{"mcpServers":{"stored-existing":{"url":"https://stored.example"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO "user" (id, name, email) VALUES ($1, 'Claim User', $2)`, []any{userID, "claim-" + suffix + "@example.test"}},
		{`INSERT INTO workspace (id, name, slug, issue_prefix) VALUES ($1, 'Claim Workspace', $2, $3)`, []any{workspaceID, "claim-" + suffix, "C" + strings.ToUpper(suffix[:5])}},
		{`INSERT INTO agent_runtime (id, workspace_id, daemon_id, name, provider, status, owner_id) VALUES ($1, $2, $3, 'Claim Runtime', 'codex', 'online', $4)`, []any{runtimeID, workspaceID, "daemon-" + suffix, userID}},
		{`INSERT INTO agent (id, workspace_id, runtime_id, owner_id, name, status, permission_mode, custom_env, mcp_config) VALUES ($1, $2, $3, $4, 'Claim Agent', 'idle', 'private', $5, $6)`, []any{agentID, workspaceID, runtimeID, userID, string(emptyEnv), string(storedMCP)}},
		{`INSERT INTO issue (id, workspace_id, title, status, assignee_type, assignee_id, creator_type, creator_id, number) VALUES ($1, $2, 'Claim Issue', 'in_progress', 'agent', $3, 'member', $4, 1)`, []any{issueID, workspaceID, agentID, userID}},
		{`INSERT INTO tool_bundle (id, workspace_id, manifest_hash, created_by) VALUES ($1, $2, $3, $4)`, []any{firstBundleID, workspaceID, strings.Repeat("1", 64), userID}},
		{`INSERT INTO tool_bundle (id, workspace_id, manifest_hash, created_by) VALUES ($1, $2, $3, $4)`, []any{secondBundleID, workspaceID, strings.Repeat("2", 64), userID}},
		{`INSERT INTO agent_tool_bundle_head (workspace_id, agent_id, bundle_id, updated_by) VALUES ($1, $2, $3, $4)`, []any{workspaceID, agentID, firstBundleID, userID}},
		{`INSERT INTO agent_task_queue (id, workspace_id, agent_id, runtime_id, issue_id, status, tool_bundle_id) VALUES ($1, $2, $3, $4, $5, 'dispatched', $6)`, []any{bundleTaskID, workspaceID, agentID, runtimeID, issueID, firstBundleID}},
		{`INSERT INTO agent_task_queue (id, workspace_id, agent_id, runtime_id, issue_id, status) VALUES ($1, $2, $3, $4, $5, 'dispatched')`, []any{legacyTaskID, workspaceID, agentID, runtimeID, issueID}},
		{`UPDATE agent_tool_bundle_head SET bundle_id = $1, updated_by = $2, updated_at = now() WHERE workspace_id = $3 AND agent_id = $4`, []any{secondBundleID, userID, workspaceID, agentID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("prepare Claim fixture: %v", err)
		}
	}
	return claimBundleFixture{
		bundleTaskID: bundleTaskID, legacyTaskID: legacyTaskID, firstBundleID: firstBundleID,
		workspaceID: workspaceID, userID: userID, runtimeID: runtimeID, agentID: agentID, issueID: issueID,
	}
}

func buildClaimedTaskForTest(t *testing.T, handler *Handler, taskID string) claimedTask {
	t.Helper()
	ctx := context.Background()
	taskUUID, err := parseUUID(taskID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := handler.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	qtx := lwdb.New(tx)
	task, err := qtx.GetAgentTask(ctx, lwdb.GetAgentTaskParams{ID: taskUUID, WorkspaceID: mustTaskWorkspace(t, handler, taskUUID)})
	if err != nil {
		t.Fatal(err)
	}
	request, err := httpRequestWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := handler.buildClaimedTask(request, qtx, task)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return claimed
}

func assertClaimBuildFailsForTest(t *testing.T, handler *Handler, taskID string) {
	t.Helper()
	ctx := context.Background()
	taskUUID, err := parseUUID(taskID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := handler.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	qtx := lwdb.New(tx)
	task, err := qtx.GetAgentTask(ctx, lwdb.GetAgentTaskParams{ID: taskUUID, WorkspaceID: mustTaskWorkspace(t, handler, taskUUID)})
	if err != nil {
		t.Fatal(err)
	}
	request, err := httpRequestWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.buildClaimedTask(request, qtx, task); err == nil {
		t.Fatal("Bundle Claim succeeded despite fail-closed Provider/public URL policy")
	}
}

func mustParseTestUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := parseUUID(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func countAgentTasksForTest(t *testing.T, pool *pgxpool.Pool, agentID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM agent_task_queue WHERE agent_id = $1`, agentID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func mustTaskWorkspace(t *testing.T, handler *Handler, taskID pgtype.UUID) pgtype.UUID {
	t.Helper()
	var workspaceID pgtype.UUID
	if err := handler.pool.QueryRow(context.Background(), `SELECT workspace_id FROM agent_task_queue WHERE id = $1`, taskID).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	return workspaceID
}

func httpRequestWithContext(ctx context.Context) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodPost, "http://claim.test", nil)
}

func assertGatewayOnlyClaimConfig(t *testing.T, raw json.RawMessage, bundleID, token string) {
	t.Helper()
	var document struct {
		MCPServers map[string]struct {
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.MCPServers) != 1 {
		t.Fatalf("Bundle Claim MCP servers = %+v", document.MCPServers)
	}
	entry := document.MCPServers["dars"]
	if entry.URL != "https://dars.example/bundles/"+bundleID+"/mcp" || entry.Headers["Authorization"] != "Bearer "+token {
		t.Fatalf("Bundle Claim entry = %+v", entry)
	}
}

func openClaimCheckDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if raw == "" {
		t.Skip("DATABASE_URL is not set; Claim integration runs on the Fresh check DB")
	}
	config, err := pgxpool.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(config.ConnConfig.Database, "dars_lightweight_check_") {
		t.Skipf("refusing database mutation outside isolated check DB: %s", config.ConnConfig.Database)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
