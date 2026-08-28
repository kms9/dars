package lightweightapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kms9/dars/internal/auth"
)

func TestDaemonTokenProvisionRegisterRotateRevokeAndRestart(t *testing.T) {
	pool := openClaimCheckDatabase(t)
	handler := New(pool, nil, Config{})
	ctx := context.Background()
	userID := uuid.NewString()
	workspaceID := uuid.NewString()
	daemonID := "daemon-" + uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	seedDaemonTokenWorkspaceForTest(
		t, pool, ctx, userID, workspaceID,
		"Daemon Token User", "daemon-token-"+suffix+"@example.test",
		"Daemon Token Workspace", "daemon-token-"+suffix, "DT", "owner",
	)

	first := createDaemonTokenForTest(t, handler, workspaceID, userID, daemonID, time.Time{})
	if !strings.HasPrefix(first.Token, "ddt_") || first.TokenPrefix == nil || *first.TokenPrefix != tokenPrefix(first.Token) {
		t.Fatalf("invalid provision response: %+v", first)
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/workspaces/"+workspaceID+"/daemon-tokens", nil)
	listRequest = listRequest.WithContext(context.WithValue(listRequest.Context(), workspaceKey, workspaceID))
	listRecorder := httptest.NewRecorder()
	handler.ListDaemonTokens(listRecorder, listRequest)
	if listRecorder.Code != http.StatusOK || strings.Contains(listRecorder.Body.String(), first.Token) {
		t.Fatalf("list response = %d %s", listRecorder.Code, listRecorder.Body.String())
	}

	registered := registerWithDaemonTokenForTest(t, handler, first.Token, workspaceID, daemonID, http.StatusCreated)
	if registered.Token != "" || len(registered.Runtimes) != 1 {
		t.Fatalf("pre-provisioned Register rotated token or missed runtime: %+v", registered)
	}
	runtimeID := registered.Runtimes[0].ID

	deregisterBody := bytes.NewBufferString(`{"runtime_ids":["` + runtimeID + `"]}`)
	deregisterRequest := httptest.NewRequest(http.MethodPost, "/api/daemon/deregister", deregisterBody)
	deregisterRequest.Header.Set("Authorization", "Bearer "+first.Token)
	deregisterRecorder := httptest.NewRecorder()
	handler.DaemonAuth(http.HandlerFunc(handler.DaemonDeregister)).ServeHTTP(deregisterRecorder, deregisterRequest)
	if deregisterRecorder.Code != http.StatusOK {
		t.Fatalf("Deregister = %d %s", deregisterRecorder.Code, deregisterRecorder.Body.String())
	}
	restarted := registerWithDaemonTokenForTest(t, handler, first.Token, workspaceID, daemonID, http.StatusCreated)
	if restarted.Runtimes[0].ID != runtimeID {
		t.Fatalf("same-token restart changed runtime ID: %s -> %s", runtimeID, restarted.Runtimes[0].ID)
	}

	crossBody := registerBodyForTest(workspaceID, daemonID+"-other")
	crossRequest := httptest.NewRequest(http.MethodPost, "/api/daemon/register", bytes.NewReader(crossBody))
	crossRequest.Header.Set("Content-Type", "application/json")
	crossRequest.Header.Set("Authorization", "Bearer "+first.Token)
	crossRecorder := httptest.NewRecorder()
	handler.HumanOrDaemonAuth(http.HandlerFunc(handler.DaemonRegister)).ServeHTTP(crossRecorder, crossRequest)
	if crossRecorder.Code != http.StatusForbidden {
		t.Fatalf("cross-daemon Register = %d %s", crossRecorder.Code, crossRecorder.Body.String())
	}

	second := createDaemonTokenForTest(t, handler, workspaceID, userID, daemonID, time.Time{})
	registerWithDaemonTokenForTest(t, handler, first.Token, workspaceID, daemonID, http.StatusUnauthorized)
	rotated := registerWithDaemonTokenForTest(t, handler, second.Token, workspaceID, daemonID, http.StatusCreated)
	if rotated.Runtimes[0].ID != runtimeID {
		t.Fatalf("rotation changed runtime ID: %s -> %s", runtimeID, rotated.Runtimes[0].ID)
	}

	revokeRequest := httptest.NewRequest(http.MethodDelete, "/api/workspaces/"+workspaceID+"/daemon-tokens/"+second.ID, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("tokenId", second.ID)
	revokeContext := context.WithValue(revokeRequest.Context(), chi.RouteCtxKey, routeContext)
	revokeContext = context.WithValue(revokeContext, workspaceKey, workspaceID)
	revokeRequest = revokeRequest.WithContext(revokeContext)
	revokeRecorder := httptest.NewRecorder()
	handler.RevokeDaemonToken(revokeRecorder, revokeRequest)
	if revokeRecorder.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d %s", revokeRecorder.Code, revokeRecorder.Body.String())
	}
	registerWithDaemonTokenForTest(t, handler, second.Token, workspaceID, daemonID, http.StatusUnauthorized)

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_runtime WHERE id = $1`, runtimeID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "offline" {
		t.Fatalf("revoked runtime status = %q, want offline", status)
	}
}

func TestDaemonTokenRejectsExcessiveExpiryAndUnownedLegacyRegister(t *testing.T) {
	pool := openClaimCheckDatabase(t)
	handler := New(pool, nil, Config{})
	ctx := context.Background()
	userID := uuid.NewString()
	workspaceID := uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	seedDaemonTokenWorkspaceForTest(
		t, pool, ctx, userID, workspaceID,
		"Daemon Expiry User", "daemon-expiry-"+suffix+"@example.test",
		"Daemon Expiry Workspace", "daemon-expiry-"+suffix, "DE", "owner",
	)
	tooLate := time.Now().UTC().Add(366 * 24 * time.Hour)
	request := daemonTokenCreateRequestForTest(t, workspaceID, userID, "too-late-"+suffix, tooLate)
	recorder := httptest.NewRecorder()
	handler.CreateDaemonToken(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("366-day token = %d %s", recorder.Code, recorder.Body.String())
	}

	legacyPlaintext, err := auth.GenerateDaemonToken()
	if err != nil {
		t.Fatal(err)
	}
	legacyDaemonID := "legacy-" + suffix
	if _, err := pool.Exec(ctx, `
		INSERT INTO daemon_token (token_hash, workspace_id, daemon_id, expires_at, name, token_prefix)
		VALUES ($1, $2, $3, now() + interval '1 hour', $3, $4)
	`, auth.HashToken(legacyPlaintext), workspaceID, legacyDaemonID, tokenPrefix(legacyPlaintext)); err != nil {
		t.Fatal(err)
	}
	registerWithDaemonTokenForTest(t, handler, legacyPlaintext, workspaceID, legacyDaemonID, http.StatusUnauthorized)
}

func TestHumanRegisterStillReturnsRotatedDaemonTokenMetadata(t *testing.T) {
	pool := openClaimCheckDatabase(t)
	handler := New(pool, nil, Config{})
	ctx := context.Background()
	userID := uuid.NewString()
	workspaceID := uuid.NewString()
	daemonID := "human-register-" + uuid.NewString()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	seedDaemonTokenWorkspaceForTest(
		t, pool, ctx, userID, workspaceID,
		"Human Register User", "human-register-"+suffix+"@example.test",
		"Human Register Workspace", "human-register-"+suffix, "HR", "member",
	)
	request := httptest.NewRequest(http.MethodPost, "/api/daemon/register", bytes.NewReader(registerBodyForTest(workspaceID, daemonID)))
	request = request.WithContext(context.WithValue(request.Context(), principalKey, Principal{Kind: principalJWT, UserID: userID}))
	recorder := httptest.NewRecorder()
	handler.DaemonRegister(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("Human Register = %d %s", recorder.Code, recorder.Body.String())
	}
	var response daemonRegisterResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(response.Token, "ddt_") || response.ExpiresAt == "" {
		t.Fatalf("Human Register response = %+v", response)
	}
	var storedUserID, storedName, storedPrefix string
	if err := pool.QueryRow(ctx, `
		SELECT user_id::text, name, token_prefix
		FROM daemon_token
		WHERE workspace_id = $1 AND daemon_id = $2
	`, workspaceID, daemonID).Scan(&storedUserID, &storedName, &storedPrefix); err != nil {
		t.Fatal(err)
	}
	if storedUserID != userID || storedName != "Pi container" || storedPrefix != tokenPrefix(response.Token) {
		t.Fatalf("stored metadata = user %q name %q prefix %q", storedUserID, storedName, storedPrefix)
	}
}

type daemonTokenTestExecer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func seedDaemonTokenWorkspaceForTest(
	t *testing.T,
	pool daemonTokenTestExecer,
	ctx context.Context,
	userID, workspaceID, userName, email, workspaceName, slug, issuePrefix, role string,
) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO "user" (id, name, email) VALUES ($1, $2, $3)`, []any{userID, userName, email}},
		{`INSERT INTO workspace (id, name, slug, issue_prefix) VALUES ($1, $2, $3, $4)`, []any{workspaceID, workspaceName, slug, issuePrefix}},
		{`INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)`, []any{workspaceID, userID, role}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func createDaemonTokenForTest(
	t *testing.T,
	handler *Handler,
	workspaceID, userID, daemonID string,
	expiry time.Time,
) daemonTokenSecretResponse {
	t.Helper()
	request := daemonTokenCreateRequestForTest(t, workspaceID, userID, daemonID, expiry)
	recorder := httptest.NewRecorder()
	handler.CreateDaemonToken(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create Daemon Token = %d %s", recorder.Code, recorder.Body.String())
	}
	var response daemonTokenSecretResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func daemonTokenCreateRequestForTest(
	t *testing.T,
	workspaceID, userID, daemonID string,
	expiry time.Time,
) *http.Request {
	t.Helper()
	payload := map[string]string{"daemon_id": daemonID, "name": "Pi container"}
	if !expiry.IsZero() {
		payload["expires_at"] = expiry.Format(time.RFC3339Nano)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/workspaces/"+workspaceID+"/daemon-tokens", bytes.NewReader(body))
	ctx := context.WithValue(request.Context(), workspaceKey, workspaceID)
	ctx = context.WithValue(ctx, principalKey, Principal{Kind: principalJWT, UserID: userID})
	return request.WithContext(ctx)
}

func registerWithDaemonTokenForTest(
	t *testing.T,
	handler *Handler,
	token, workspaceID, daemonID string,
	wantStatus int,
) daemonRegisterResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/daemon/register", bytes.NewReader(registerBodyForTest(workspaceID, daemonID)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.HumanOrDaemonAuth(http.HandlerFunc(handler.DaemonRegister)).ServeHTTP(recorder, request)
	if recorder.Code != wantStatus {
		t.Fatalf("Register status = %d, want %d: %s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var response daemonRegisterResponse
	if recorder.Code == http.StatusCreated {
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
	}
	return response
}

func registerBodyForTest(workspaceID, daemonID string) []byte {
	body, _ := json.Marshal(map[string]any{
		"protocol_version": protocolVersion,
		"workspace_id":     workspaceID,
		"daemon_id":        daemonID,
		"device_name":      "Pi container",
		"cli_version":      "test",
		"runtimes":         []map[string]string{{"name": "DARS Pi Runtime", "type": "pi", "status": "online"}},
	})
	return body
}
