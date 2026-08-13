package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestControlPlaneSourceAndBundleLifecycleOnFreshCheckDatabase(t *testing.T) {
	pool := openControlPlaneCheckDatabase(t)
	codec, err := NewSourceSecretCodec("test", map[string][]byte{"test": bytes.Repeat([]byte{21}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	control := NewControlPlane(pool, ControlPlaneOptions{
		SecretCodec: codec, ProviderAllowlist: map[string]bool{"codex": true}, MaxArtifactBytes: 1024,
	})
	workspaceID := "11000000-0000-0000-0000-000000000001"
	otherWorkspaceID := "11000000-0000-0000-0000-000000000002"
	userID := "12000000-0000-0000-0000-000000000001"

	createBody := `{
		"name":"Fixture Source",
		"kind":"server_local",
		"transport_config":{},
		"auth":{"kind":"bearer","token":"sentinel-control-secret"},
		"tools":[
			{"public_name":"fixture.first","upstream_name":"fixture/first","description":"first","input_schema":{"type":"object"},"operation_metadata":{"registry_key":"first"}},
			{"public_name":"fixture.second","upstream_name":"fixture/second","description":"second","input_schema":{"type":"object"},"operation_metadata":{"registry_key":"second"}}
		]
	}`
	createRecorder := callControlHandler(t, control.CreateSource, http.MethodPost, "/api/tool-sources", createBody, workspaceID, userID, nil)
	if createRecorder.Code != http.StatusCreated {
		t.Fatalf("create source = %d %s", createRecorder.Code, createRecorder.Body.String())
	}
	if strings.Contains(createRecorder.Body.String(), "sentinel-control-secret") || strings.Contains(createRecorder.Body.String(), "ciphertext") {
		t.Fatalf("create response leaked secret material: %s", createRecorder.Body.String())
	}
	var created sourceDTO
	decodeRecorder(t, createRecorder, &created)
	if created.Enabled || created.CurrentRevision != nil || created.Revision == nil || !created.Revision.SecretConfigured || created.Revision.SecretKeyID == nil {
		t.Fatalf("unexpected staged source: %+v", created)
	}

	crossWorkspace := callControlHandler(t, control.GetSource, http.MethodGet, "/api/tool-sources/"+created.ID, "", otherWorkspaceID, userID, map[string]string{"sourceId": created.ID})
	if crossWorkspace.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace get = %d %s", crossWorkspace.Code, crossWorkspace.Body.String())
	}

	artifactBody := `{"media_type":"application/octet-stream","content_base64":"AQID"}`
	artifactRecorder := callControlHandler(t, control.UploadSourceArtifact, http.MethodPost, "/api/tool-sources/"+created.ID+"/artifacts", artifactBody, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if artifactRecorder.Code != http.StatusCreated || strings.Contains(artifactRecorder.Body.String(), "AQID") {
		t.Fatalf("artifact response = %d %s", artifactRecorder.Code, artifactRecorder.Body.String())
	}

	validateBody := `{"revision_id":"` + created.Revision.ID + `"}`
	validateRecorder := callControlHandler(t, control.ValidateSource, http.MethodPost, "/api/tool-sources/"+created.ID+"/validate", validateBody, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if validateRecorder.Code != http.StatusOK {
		t.Fatalf("validate source = %d %s", validateRecorder.Code, validateRecorder.Body.String())
	}
	var validated sourceDTO
	decodeRecorder(t, validateRecorder, &validated)
	if validated.CurrentRevision == nil || *validated.CurrentRevision != created.Revision.ID || validated.Enabled {
		t.Fatalf("validation did not atomically advance disabled source: %+v", validated)
	}
	conflictRecorder := callControlHandler(t, control.ValidateSource, http.MethodPost, "/api/tool-sources/"+created.ID+"/validate", validateBody, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if conflictRecorder.Code != http.StatusConflict || !strings.Contains(conflictRecorder.Body.String(), "tool_source_revision_conflict") {
		t.Fatalf("second validation = %d %s", conflictRecorder.Code, conflictRecorder.Body.String())
	}

	failedUpdateBody := `{
		"transport_config":{},
		"tools":[{"public_name":"fixture.invalid","upstream_name":"fixture/invalid","input_schema":{"type":"object"},"operation_metadata":{"registry_key":"not_registered"}}]
	}`
	failedUpdateRecorder := callControlHandler(t, control.UpdateSource, http.MethodPut, "/api/tool-sources/"+created.ID, failedUpdateBody, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if failedUpdateRecorder.Code != http.StatusCreated {
		t.Fatalf("stage invalid revision = %d %s", failedUpdateRecorder.Code, failedUpdateRecorder.Body.String())
	}
	var failedUpdate sourceDTO
	decodeRecorder(t, failedUpdateRecorder, &failedUpdate)
	failedValidateBody := `{"revision_id":"` + failedUpdate.Revision.ID + `"}`
	failedValidateRecorder := callControlHandler(t, control.ValidateSource, http.MethodPost, "/api/tool-sources/"+created.ID+"/validate", failedValidateBody, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if failedValidateRecorder.Code != http.StatusUnprocessableEntity || !strings.Contains(failedValidateRecorder.Body.String(), "tool_source_invalid") {
		t.Fatalf("invalid revision validation = %d %s", failedValidateRecorder.Code, failedValidateRecorder.Body.String())
	}
	currentRecorder := callControlHandler(t, control.GetSource, http.MethodGet, "/api/tool-sources/"+created.ID, "", workspaceID, userID, map[string]string{"sourceId": created.ID})
	var current sourceDTO
	decodeRecorder(t, currentRecorder, &current)
	if current.CurrentRevision == nil || *current.CurrentRevision != created.Revision.ID {
		t.Fatalf("failed revision replaced ready revision: %+v", current)
	}

	enableRecorder := callControlHandler(t, control.EnableSource, http.MethodPost, "/api/tool-sources/"+created.ID+"/enable", `{}`, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if enableRecorder.Code != http.StatusOK {
		t.Fatalf("enable source = %d %s", enableRecorder.Code, enableRecorder.Body.String())
	}
	disableRecorder := callControlHandler(t, control.DisableSource, http.MethodPost, "/api/tool-sources/"+created.ID+"/disable", `{}`, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if disableRecorder.Code != http.StatusOK || !strings.Contains(disableRecorder.Body.String(), `"enabled":false`) {
		t.Fatalf("disable source = %d %s", disableRecorder.Code, disableRecorder.Body.String())
	}
	enableRecorder = callControlHandler(t, control.EnableSource, http.MethodPost, "/api/tool-sources/"+created.ID+"/enable", `{}`, workspaceID, userID, map[string]string{"sourceId": created.ID})
	if enableRecorder.Code != http.StatusOK {
		t.Fatalf("re-enable source = %d %s", enableRecorder.Code, enableRecorder.Body.String())
	}
	toolsRecorder := callControlHandler(t, control.ListSourceTools, http.MethodGet, "/api/tool-sources/"+created.ID+"/tools", "", workspaceID, userID, map[string]string{"sourceId": created.ID})
	if toolsRecorder.Code != http.StatusOK {
		t.Fatalf("list tools = %d %s", toolsRecorder.Code, toolsRecorder.Body.String())
	}
	var tools []toolDTO
	decodeRecorder(t, toolsRecorder, &tools)
	if len(tools) != 2 || tools[0].PublicName != "fixture.first" || tools[1].PublicName != "fixture.second" {
		t.Fatalf("tools = %+v", tools)
	}

	runtimeID := "13000000-0000-0000-0000-000000000001"
	agentID := "14000000-0000-0000-0000-000000000001"
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO agent_runtime (id, workspace_id, daemon_id, name, provider, owner_id)
		VALUES ($1, $2, 'daemon-fixture', 'runtime-fixture', 'codex', $3)
	`, runtimeID, workspaceID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO agent (id, workspace_id, runtime_id, owner_id, name, permission_mode)
		VALUES ($1, $2, $3, $4, 'agent-fixture', 'private')
	`, agentID, workspaceID, runtimeID, userID); err != nil {
		t.Fatal(err)
	}

	firstSelection := `{"items":[{"tool_definition_id":"` + tools[0].ID + `","exported_name":"fixture.first"}]}`
	firstBundle := publishBundleForTest(t, control, agentID, firstSelection, workspaceID, userID, http.StatusCreated)
	if len(firstBundle.Items) != 1 || firstBundle.Items[0].ExportedName != "fixture.first" || firstBundle.Items[0].CanonicalPublicName != "fixture.first" {
		t.Fatalf("default Bundle names = %+v", firstBundle.Items)
	}
	currentBundleRecorder := callControlHandler(t, control.GetAgentBundle, http.MethodGet, "/api/agents/"+agentID+"/tool-bundle", "", workspaceID, userID, map[string]string{"agentId": agentID})
	if currentBundleRecorder.Code != http.StatusOK || !strings.Contains(currentBundleRecorder.Body.String(), firstBundle.ID) {
		t.Fatalf("get current Agent Bundle = %d %s", currentBundleRecorder.Code, currentBundleRecorder.Body.String())
	}
	partialSelection := `{"items":[{"tool_definition_id":"` + tools[0].ID + `","exported_name":"fixture.first"},{"tool_definition_id":"15000000-0000-0000-0000-000000000099","exported_name":"missing.tool"}]}`
	partialRecorder := callControlHandler(t, control.PublishAgentBundle, http.MethodPut, "/api/agents/"+agentID+"/tool-bundle", partialSelection, workspaceID, userID, map[string]string{"agentId": agentID})
	if partialRecorder.Code != http.StatusUnprocessableEntity || !strings.Contains(partialRecorder.Body.String(), "tool_bundle_invalid") {
		t.Fatalf("partial publication = %d %s", partialRecorder.Code, partialRecorder.Body.String())
	}
	sameBundle := publishBundleForTest(t, control, agentID, firstSelection, workspaceID, userID, http.StatusOK)
	if sameBundle.ID != firstBundle.ID {
		t.Fatalf("no-op publication changed ID: %s -> %s", firstBundle.ID, sameBundle.ID)
	}
	aliasSelection := `{"items":[{"tool_definition_id":"` + tools[0].ID + `","exported_name":"skill.fixture.first"}]}`
	aliasBundle := publishBundleForTest(t, control, agentID, aliasSelection, workspaceID, userID, http.StatusCreated)
	if aliasBundle.ID == firstBundle.ID || len(aliasBundle.Items) != 1 || aliasBundle.Items[0].ExportedName != "skill.fixture.first" || aliasBundle.Items[0].CanonicalPublicName != "fixture.first" {
		t.Fatalf("alias-only publication = %+v", aliasBundle)
	}
	duplicateNames := `{"items":[{"tool_definition_id":"` + tools[0].ID + `","exported_name":"duplicate.name"},{"tool_definition_id":"` + tools[1].ID + `","exported_name":"duplicate.name"}]}`
	duplicateRecorder := callControlHandler(t, control.PublishAgentBundle, http.MethodPut, "/api/agents/"+agentID+"/tool-bundle", duplicateNames, workspaceID, userID, map[string]string{"agentId": agentID})
	if duplicateRecorder.Code != http.StatusConflict || !strings.Contains(duplicateRecorder.Body.String(), "tool_name_conflict") {
		t.Fatalf("duplicate alias publication = %d %s", duplicateRecorder.Code, duplicateRecorder.Body.String())
	}
	invalidAlias := `{"items":[{"tool_definition_id":"` + tools[0].ID + `","exported_name":"invalid alias"}]}`
	invalidAliasRecorder := callControlHandler(t, control.PublishAgentBundle, http.MethodPut, "/api/agents/"+agentID+"/tool-bundle", invalidAlias, workspaceID, userID, map[string]string{"agentId": agentID})
	if invalidAliasRecorder.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidAliasRecorder.Body.String(), "tool_bundle_invalid") {
		t.Fatalf("invalid alias publication = %d %s", invalidAliasRecorder.Code, invalidAliasRecorder.Body.String())
	}
	currentAfterFailures := callControlHandler(t, control.GetAgentBundle, http.MethodGet, "/api/agents/"+agentID+"/tool-bundle", "", workspaceID, userID, map[string]string{"agentId": agentID})
	if currentAfterFailures.Code != http.StatusOK || !strings.Contains(currentAfterFailures.Body.String(), aliasBundle.ID) {
		t.Fatalf("failed alias publication changed Agent head = %d %s", currentAfterFailures.Code, currentAfterFailures.Body.String())
	}
	secondSelection := `{"items":[{"tool_definition_id":"` + tools[0].ID + `","exported_name":"fixture.first"},{"tool_definition_id":"` + tools[1].ID + `","exported_name":"fixture.second"}]}`
	secondBundle := publishBundleForTest(t, control, agentID, secondSelection, workspaceID, userID, http.StatusCreated)
	if secondBundle.ID == firstBundle.ID {
		t.Fatal("semantic change reused Bundle ID")
	}
	revertedBundle := publishBundleForTest(t, control, agentID, firstSelection, workspaceID, userID, http.StatusCreated)
	if revertedBundle.ID == firstBundle.ID || revertedBundle.ID == secondBundle.ID {
		t.Fatal("semantic revert reused a historical Bundle ID")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE agent_runtime SET provider = 'unsupported-fixture' WHERE id = $1 AND workspace_id = $2`, runtimeID, workspaceID); err != nil {
		t.Fatal(err)
	}
	unsupported := callControlHandler(t, control.PublishAgentBundle, http.MethodPut, "/api/agents/"+agentID+"/tool-bundle", firstSelection, workspaceID, userID, map[string]string{"agentId": agentID})
	if unsupported.Code != http.StatusConflict || !strings.Contains(unsupported.Body.String(), "provider_mcp_unsupported") {
		t.Fatalf("unsupported Provider publication = %d %s", unsupported.Code, unsupported.Body.String())
	}
	if _, err := pool.Exec(context.Background(), `UPDATE agent_runtime SET provider = 'codex' WHERE id = $1 AND workspace_id = $2`, runtimeID, workspaceID); err != nil {
		t.Fatal(err)
	}
	clearRecorder := callControlHandler(t, control.ClearAgentBundle, http.MethodDelete, "/api/agents/"+agentID+"/tool-bundle", "", workspaceID, userID, map[string]string{"agentId": agentID})
	if clearRecorder.Code != http.StatusOK || !strings.Contains(clearRecorder.Body.String(), `"bundle":null`) {
		t.Fatalf("clear current Agent Bundle = %d %s", clearRecorder.Code, clearRecorder.Body.String())
	}
	clearedRecorder := callControlHandler(t, control.GetAgentBundle, http.MethodGet, "/api/agents/"+agentID+"/tool-bundle", "", workspaceID, userID, map[string]string{"agentId": agentID})
	if clearedRecorder.Code != http.StatusOK || !strings.Contains(clearedRecorder.Body.String(), `"bundle":null`) {
		t.Fatalf("get cleared Agent Bundle = %d %s", clearedRecorder.Code, clearedRecorder.Body.String())
	}
	retainedBundleRecorder := callControlHandler(t, control.GetBundle, http.MethodGet, "/api/tool-bundles/"+firstBundle.ID, "", workspaceID, userID, map[string]string{"bundleId": firstBundle.ID})
	if retainedBundleRecorder.Code != http.StatusOK {
		t.Fatalf("clearing Agent head removed retained Bundle = %d %s", retainedBundleRecorder.Code, retainedBundleRecorder.Body.String())
	}

	deleteRetained := callControlHandler(t, control.DeleteSource, http.MethodDelete, "/api/tool-sources/"+created.ID, "", workspaceID, userID, map[string]string{"sourceId": created.ID})
	if deleteRetained.Code != http.StatusConflict || !strings.Contains(deleteRetained.Body.String(), "tool_source_retained") {
		t.Fatalf("retained source delete = %d %s", deleteRetained.Code, deleteRetained.Body.String())
	}

	revokeRecorder := callControlHandler(t, control.RevokeBundle, http.MethodPost, "/api/tool-bundles/"+firstBundle.ID+"/revoke", `{}`, workspaceID, userID, map[string]string{"bundleId": firstBundle.ID})
	if revokeRecorder.Code != http.StatusOK || !strings.Contains(revokeRecorder.Body.String(), `"status":"revoked"`) {
		t.Fatalf("revoke bundle = %d %s", revokeRecorder.Code, revokeRecorder.Body.String())
	}

	unretainedRecorder := callControlHandler(t, control.CreateSource, http.MethodPost, "/api/tool-sources", `{
		"name":"Delete Fixture","kind":"server_local","transport_config":{},
		"tools":[{"public_name":"delete.fixture","upstream_name":"delete/fixture","input_schema":{"type":"object"},"operation_metadata":{"registry_key":"first"}}]
	}`, workspaceID, userID, nil)
	if unretainedRecorder.Code != http.StatusCreated {
		t.Fatalf("create unretained source = %d %s", unretainedRecorder.Code, unretainedRecorder.Body.String())
	}
	var unretained sourceDTO
	decodeRecorder(t, unretainedRecorder, &unretained)
	deleteRecorder := callControlHandler(t, control.DeleteSource, http.MethodDelete, "/api/tool-sources/"+unretained.ID, "", workspaceID, userID, map[string]string{"sourceId": unretained.ID})
	if deleteRecorder.Code != http.StatusNoContent {
		t.Fatalf("delete unretained source = %d %s", deleteRecorder.Code, deleteRecorder.Body.String())
	}

	var encryptedSecrets string
	if err := pool.QueryRow(context.Background(), `SELECT coalesce(string_agg(envelope::text, ''), '') FROM tool_source_secret WHERE workspace_id = $1`, workspaceID).Scan(&encryptedSecrets); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encryptedSecrets, "sentinel-control-secret") || !strings.Contains(encryptedSecrets, "ciphertext") {
		t.Fatalf("Source secret storage is not a redacted envelope: %s", encryptedSecrets)
	}
	var activityDetails string
	if err := pool.QueryRow(context.Background(), `
		SELECT coalesce(string_agg(details::text, ''), '') FROM activity_log
		WHERE workspace_id = $1 AND action LIKE 'mcp_gateway_%'
	`, workspaceID).Scan(&activityDetails); err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"sentinel-control-secret", "AQID", "ciphertext"} {
		if strings.Contains(activityDetails, sentinel) {
			t.Fatalf("activity audit leaked sentinel %q: %s", sentinel, activityDetails)
		}
	}
}

func TestControlPlaneRejectsInvalidIdentifiersAndUnknownFields(t *testing.T) {
	control := NewControlPlane(nil, ControlPlaneOptions{})
	workspaceID := "21000000-0000-0000-0000-000000000001"
	userID := "22000000-0000-0000-0000-000000000001"

	invalidID := callControlHandler(t, control.GetSource, http.MethodGet, "/api/tool-sources/not-a-uuid", "", workspaceID, userID, map[string]string{"sourceId": "not-a-uuid"})
	if invalidID.Code != http.StatusUnprocessableEntity || !strings.Contains(invalidID.Body.String(), "invalid_argument") {
		t.Fatalf("invalid Source ID = %d %s", invalidID.Code, invalidID.Body.String())
	}
	unknownField := callControlHandler(t, control.CreateSource, http.MethodPost, "/api/tool-sources", `{
		"name":"strict","kind":"server_local","transport_config":{},"unknown":true
	}`, workspaceID, userID, nil)
	if unknownField.Code != http.StatusUnprocessableEntity || !strings.Contains(unknownField.Body.String(), "tool_source_invalid") {
		t.Fatalf("unknown request field = %d %s", unknownField.Code, unknownField.Body.String())
	}
}

func TestControlPlaneAtomicMultipartSwaggerImportOnFreshCheckDatabase(t *testing.T) {
	pool := openControlPlaneCheckDatabase(t)
	config := Config{
		AllowedPorts: []uint16{443}, MaxArtifactBytes: 4096,
		ConnectTimeout: time.Second, InvocationTimeout: time.Second,
	}
	resolver := &staticResolver{answers: map[string][][]netip.Addr{
		"swagger-fixture.example": {{netip.MustParseAddr("8.8.8.8")}},
	}}
	adapter := NewOpenAPIAdapter(config, NewEgressPolicy(config, EgressPolicyOptions{Resolver: resolver}), nil)
	control := NewControlPlane(pool, ControlPlaneOptions{
		OpenAPI: adapter, MaxArtifactBytes: 4096, ValidationTimeout: time.Second,
	})
	workspaceID := "11000000-0000-0000-0000-000000000001"
	userID := "12000000-0000-0000-0000-000000000001"
	swagger := []byte(`{"swagger":"2.0","info":{"title":"legacy","version":"1"},"paths":{"/pets":{"get":{"operationId":"listPets","responses":{"200":{"description":"ok"}}}}}}`)
	recorder := callControlMultipartHandler(t, control.ImportSource, "/api/tool-sources/import", workspaceID, userID, map[string]string{
		"name": "Swagger Fixture", "kind": "openapi", "endpoint": "https://swagger-fixture.example/api",
	}, "swagger.json", "application/json", swagger)
	if recorder.Code != http.StatusCreated || strings.Contains(recorder.Body.String(), string(swagger)) {
		t.Fatalf("Swagger multipart import = %d %s", recorder.Code, recorder.Body.String())
	}
	var imported importToolSourceResponse
	decodeRecorder(t, recorder, &imported)
	if imported.Source.CurrentRevision == nil || imported.Source.Revision == nil || imported.Source.Revision.Status != "ready" ||
		len(imported.Tools) != 1 || imported.Tools[0].PublicName != "swagger-fixture.listPets" || imported.Artifact.SizeBytes != int64(len(swagger)) {
		t.Fatalf("unexpected Swagger import response: %+v", imported)
	}
	updatedSwagger := []byte(`{"swagger":"2.0","info":{"title":"legacy","version":"2"},"paths":{"/pets":{"get":{"operationId":"getPets","responses":{"200":{"description":"ok"}}}}}}`)
	updateRecorder := callControlMultipartHandler(t, control.ImportSource, "/api/tool-sources/import", workspaceID, userID, map[string]string{
		"source_id": imported.Source.ID, "name": imported.Source.Name, "kind": "openapi", "endpoint": "https://swagger-fixture.example/api",
	}, "swagger-v2.json", "application/json", updatedSwagger)
	if updateRecorder.Code != http.StatusCreated {
		t.Fatalf("Swagger multipart revision update = %d %s", updateRecorder.Code, updateRecorder.Body.String())
	}
	var updated importToolSourceResponse
	decodeRecorder(t, updateRecorder, &updated)
	if updated.Source.ID != imported.Source.ID || updated.Source.Revision == nil || updated.Source.Revision.Revision != 2 ||
		len(updated.Tools) != 1 || updated.Tools[0].PublicName != "swagger-fixture.getPets" {
		t.Fatalf("unexpected Swagger revision response: %+v", updated)
	}

	failure := callControlMultipartHandler(t, control.ImportSource, "/api/tool-sources/import", workspaceID, userID, map[string]string{
		"name": "Atomic Failure", "kind": "openapi", "endpoint": "https://swagger-fixture.example/api",
	}, "broken.yaml", "application/yaml", []byte("swagger: '2.0'\npaths: ["))
	if failure.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid multipart import = %d %s", failure.Code, failure.Body.String())
	}
	var partialCount int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM tool_source WHERE workspace_id = $1 AND name = 'atomic-failure'`, workspaceID).Scan(&partialCount); err != nil {
		t.Fatal(err)
	}
	if partialCount != 0 {
		t.Fatalf("failed atomic import retained %d Sources", partialCount)
	}
}

func publishBundleForTest(t *testing.T, control *ControlPlane, agentID, body, workspaceID, userID string, expectedStatus int) bundleDTO {
	t.Helper()
	recorder := callControlHandler(t, control.PublishAgentBundle, http.MethodPut, "/api/agents/"+agentID+"/tool-bundle", body, workspaceID, userID, map[string]string{"agentId": agentID})
	if recorder.Code != expectedStatus {
		t.Fatalf("publish bundle = %d, want %d; %s", recorder.Code, expectedStatus, recorder.Body.String())
	}
	var bundle bundleDTO
	decodeRecorder(t, recorder, &bundle)
	return bundle
}

func callControlHandler(t *testing.T, handler http.HandlerFunc, method, path, body, workspaceID, userID string, params map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Workspace-ID", workspaceID)
	request.Header.Set("X-User-ID", userID)
	if len(params) > 0 {
		routeContext := chi.NewRouteContext()
		for key, value := range params {
			routeContext.URLParams.Add(key, value)
		}
		request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func callControlMultipartHandler(
	t *testing.T,
	handler http.HandlerFunc,
	path string,
	workspaceID string,
	userID string,
	fields map[string]string,
	filename string,
	mediaType string,
	content []byte,
) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteField("media_type", mediaType); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Workspace-ID", workspaceID)
	request.Header.Set("X-User-ID", userID)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeRecorder(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v\n%s", err, recorder.Body.String())
	}
}

func openControlPlaneCheckDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if raw == "" {
		t.Skip("DATABASE_URL is not set; control-plane integration runs on the Fresh check DB")
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
