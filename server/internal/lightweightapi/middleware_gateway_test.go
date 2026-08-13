package lightweightapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

func TestGatewayControlPlaneRoleMatrix(t *testing.T) {
	handler := &Handler{}
	allowed := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	requireAdmin := handler.RequireWorkspaceRole("owner", "admin")(allowed)
	for _, role := range []string{"owner", "admin", "member"} {
		request := httptest.NewRequest(http.MethodGet, "/api/tool-sources", nil)
		request = request.WithContext(context.WithValue(request.Context(), memberKey, lwdb.Member{Role: role}))
		recorder := httptest.NewRecorder()
		requireAdmin.ServeHTTP(recorder, request)
		expected := http.StatusNoContent
		if role == "member" {
			expected = http.StatusForbidden
		}
		if recorder.Code != expected {
			t.Errorf("role %q status = %d, want %d", role, recorder.Code, expected)
		}
	}

	workspaceAdminBoundary := handler.RequireWorkspaceMember(handler.RequireWorkspaceRole("owner", "admin")(allowed))
	for _, principal := range []Principal{
		{Kind: principalTask, WorkspaceID: "workspace"},
		{Kind: principalDaemon, WorkspaceID: "workspace"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/tool-sources", nil)
		request = request.WithContext(context.WithValue(request.Context(), principalKey, principal))
		recorder := httptest.NewRecorder()
		workspaceAdminBoundary.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Errorf("principal %q status = %d, want 403", principal.Kind, recorder.Code)
		}
	}
}
