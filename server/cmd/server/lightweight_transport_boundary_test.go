package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

func TestMCPTransportBypassesRESTErrorEnvelope(t *testing.T) {
	router := chi.NewRouter()
	router.Use(chimw.RequestID)
	mountLightweightTransportGroups(
		router,
		nil,
		[]string{"http://localhost"},
		func(next http.Handler) http.Handler { return next },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/mcp-test")
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("mcp-native-body"))
		}),
		func(rest chi.Router) {
			rest.Get("/rest-error", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"code":"invalid_argument"}`))
			})
		},
	)

	mcpRequest := httptest.NewRequest(http.MethodPost, "/bundles/tb_01ABCDEFG/mcp", strings.NewReader(`{}`))
	mcpRecorder := httptest.NewRecorder()
	router.ServeHTTP(mcpRecorder, mcpRequest)
	if mcpRecorder.Code != http.StatusTeapot || mcpRecorder.Body.String() != "mcp-native-body" {
		t.Fatalf("MCP response changed: status=%d body=%q", mcpRecorder.Code, mcpRecorder.Body.String())
	}
	if got := mcpRecorder.Header().Get("Content-Type"); got != "application/mcp-test" {
		t.Fatalf("MCP Content-Type = %q", got)
	}

	restRequest := httptest.NewRequest(http.MethodGet, "/rest-error", nil)
	restRecorder := httptest.NewRecorder()
	router.ServeHTTP(restRecorder, restRequest)
	if restRecorder.Code != http.StatusBadRequest || !strings.Contains(restRecorder.Body.String(), `"error":{"code":"invalid_argument"`) {
		t.Fatalf("REST response lost frozen envelope: status=%d body=%q", restRecorder.Code, restRecorder.Body.String())
	}
}

func TestMCPTaskAuthFailureIsNotRESTWrapped(t *testing.T) {
	router := chi.NewRouter()
	router.Use(chimw.RequestID)
	mountLightweightTransportGroups(
		router,
		nil,
		[]string{"http://localhost"},
		func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("task-auth-failed"))
			})
		},
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("Gateway must not run after auth failure")
		}),
		func(chi.Router) {},
	)

	request := httptest.NewRequest(http.MethodPost, "/bundles/tb_01ABCDEFG/mcp", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || recorder.Body.String() != "task-auth-failed" {
		t.Fatalf("auth response changed: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestMCPGatewayDoesNotMountLegacyRoute(t *testing.T) {
	router := chi.NewRouter()
	mountLightweightTransportGroups(
		router,
		nil,
		[]string{"http://localhost"},
		func(next http.Handler) http.Handler { return next },
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("legacy MCP route reached the Gateway")
		}),
		func(chi.Router) {},
	)

	request := httptest.NewRequest(http.MethodPost, "/mcp/bundles/tb_01ABCDEFG", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("legacy MCP route status = %d, want 404", recorder.Code)
	}
}

func TestRESTCORSPreflightDoesNotApplyToMCPFacade(t *testing.T) {
	router := chi.NewRouter()
	mountLightweightTransportGroups(
		router,
		nil,
		[]string{"http://localhost:23000"},
		func(next http.Handler) http.Handler { return next },
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}),
		func(rest chi.Router) {
			routes := []lightweightRoute{{Method: http.MethodPost, Path: "/auth/send-code"}}
			registerLightweightPreflightRoutes(rest, routes)
			rest.Post("/auth/send-code", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
		},
	)

	restRequest := httptest.NewRequest(http.MethodOptions, "/auth/send-code", nil)
	restRequest.Header.Set("Origin", "http://localhost:23000")
	restRequest.Header.Set("Access-Control-Request-Method", http.MethodPost)
	restRequest.Header.Set("Access-Control-Request-Headers", "content-type,x-request-id")
	restRecorder := httptest.NewRecorder()
	router.ServeHTTP(restRecorder, restRequest)
	if restRecorder.Code != http.StatusOK {
		t.Fatalf("REST preflight status = %d, want 200", restRecorder.Code)
	}
	if got := restRecorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:23000" {
		t.Fatalf("REST Access-Control-Allow-Origin = %q", got)
	}

	mcpRequest := httptest.NewRequest(http.MethodOptions, "/bundles/tb_01ABCDEFG/mcp", nil)
	mcpRequest.Header.Set("Origin", "http://localhost:23000")
	mcpRequest.Header.Set("Access-Control-Request-Method", http.MethodPost)
	mcpRecorder := httptest.NewRecorder()
	router.ServeHTTP(mcpRecorder, mcpRequest)
	if got := mcpRecorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("MCP preflight unexpectedly received REST CORS origin %q", got)
	}
}
