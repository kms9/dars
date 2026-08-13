package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kms9/dars/internal/util/secretbox"
)

const (
	lightweightSchemaEdition    = "dars_lightweight"
	lightweightSchemaMinVersion = 1
	lightweightSchemaMaxVersion = 1
	lightweightIdentityQuery    = `SELECT current_database(), edition, schema_version FROM schema_metadata WHERE id = 1`
	readinessCacheTTL           = 5 * time.Second
)

type readinessDB interface {
	Ping(context.Context) error
	QueryRow(context.Context, string, ...any) pgx.Row
}

type liveResponse struct {
	Status string `json:"status"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type lightweightServerHealth struct {
	db       readinessDB
	cacheTTL time.Duration
	cacheMu  sync.Mutex
	cache    atomic.Pointer[cachedLightweightReadiness]
}

type cachedLightweightReadiness struct {
	response   lightweightReadinessResponse
	statusCode int
	expiresAt  time.Time
}

type lightweightReadinessResponse struct {
	Status string                     `json:"status"`
	Checks lightweightReadinessChecks `json:"checks"`
}

type lightweightReadinessChecks struct {
	Database         string `json:"database"`
	DatabaseIdentity string `json:"database_identity"`
	Schema           string `json:"schema"`
	Email            string `json:"email"`
	AgentSecret      string `json:"agent_secret"`
}

func newLightweightServerHealth(pool *pgxpool.Pool) *lightweightServerHealth {
	return &lightweightServerHealth{db: pool, cacheTTL: readinessCacheTTL}
}

func (h *lightweightServerHealth) liveHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, liveResponse{Status: "ok"})
}

func (h *lightweightServerHealth) readyHandler(w http.ResponseWriter, r *http.Request) {
	response, status := h.readiness(r.Context())
	writeJSON(w, status, response)
}

func (h *lightweightServerHealth) readiness(parent context.Context) (lightweightReadinessResponse, int) {
	if h.cacheTTL <= 0 {
		return h.computeReadiness(parent)
	}
	now := time.Now()
	if cached := h.cache.Load(); cached != nil && now.Before(cached.expiresAt) {
		return cached.response, cached.statusCode
	}

	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	now = time.Now()
	if cached := h.cache.Load(); cached != nil && now.Before(cached.expiresAt) {
		return cached.response, cached.statusCode
	}
	response, status := h.computeReadiness(parent)
	h.cache.Store(&cachedLightweightReadiness{response: response, statusCode: status, expiresAt: now.Add(h.cacheTTL)})
	return response, status
}

func (h *lightweightServerHealth) computeReadiness(parent context.Context) (lightweightReadinessResponse, int) {
	response := lightweightReadinessResponse{
		Status: "ok",
		Checks: lightweightReadinessChecks{
			Database:         "ok",
			DatabaseIdentity: "ok",
			Schema:           "ok",
			Email:            "ok",
			AgentSecret:      "ok",
		},
	}

	if isProductionEnvironment() {
		if !productionEmailConfigured() {
			response.Checks.Email = "error"
		}
		if _, err := secretbox.LoadKey("DARS_AGENT_SECRET_KEY"); err != nil {
			response.Checks.AgentSecret = "error"
		}
	}

	if h.db == nil {
		response.Checks.Database = "error"
		response.Checks.DatabaseIdentity = "unknown"
		response.Checks.Schema = "unknown"
		return finalizeLightweightReadiness(response)
	}

	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		response.Checks.Database = "error"
		response.Checks.DatabaseIdentity = "unknown"
		response.Checks.Schema = "unknown"
		return finalizeLightweightReadiness(response)
	}

	var databaseName, edition string
	var schemaVersion int
	if err := h.db.QueryRow(ctx, lightweightIdentityQuery).Scan(&databaseName, &edition, &schemaVersion); err != nil {
		response.Checks.DatabaseIdentity = "error"
		response.Checks.Schema = "error"
		return finalizeLightweightReadiness(response)
	}
	if databaseName != expectedLightweightDatabaseName() {
		response.Checks.DatabaseIdentity = "mismatch"
	}
	if edition != lightweightSchemaEdition {
		response.Checks.Schema = "edition_mismatch"
	} else if schemaVersion < lightweightSchemaMinVersion || schemaVersion > lightweightSchemaMaxVersion {
		response.Checks.Schema = "unsupported"
	}

	return finalizeLightweightReadiness(response)
}

func finalizeLightweightReadiness(response lightweightReadinessResponse) (lightweightReadinessResponse, int) {
	checks := response.Checks
	if checks.Database != "ok" || checks.DatabaseIdentity != "ok" || checks.Schema != "ok" || checks.Email != "ok" || checks.AgentSecret != "ok" {
		response.Status = "not_ready"
		return response, http.StatusServiceUnavailable
	}
	return response, http.StatusOK
}

func expectedLightweightDatabaseName() string {
	if configured := strings.TrimSpace(os.Getenv("EXPECTED_DATABASE_NAME")); configured != "" {
		return configured
	}
	return "dars_lightweight"
}

func isProductionEnvironment() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
}

func productionEmailConfigured() bool {
	if strings.TrimSpace(os.Getenv("RESEND_API_KEY")) != "" {
		return true
	}
	if strings.TrimSpace(os.Getenv("SMTP_HOST")) == "" {
		return false
	}
	return strings.TrimSpace(os.Getenv("SMTP_FROM_EMAIL")) != "" || strings.TrimSpace(os.Getenv("RESEND_FROM_EMAIL")) != ""
}
