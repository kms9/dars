package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kms9/dars/pkg/protocol"
)

const (
	lightweightDefaultLimit = 30
	lightweightMaxLimit     = 100
	lightweightMaxJSONBody  = 1 << 20
	lightweightMaxCursor    = 4096
)

type lightweightJSONField struct {
	Required bool
	Nullable bool
}

type lightweightJSONSchema struct {
	Fields         map[string]lightweightJSONField
	AllowEmpty     bool
	AllowMultipart bool
}

// lightweightTransportForRoute installs the target transport contract after
// authentication and before a Handler can observe or mutate state.
func lightweightTransportForRoute(route lightweightRoute) func(http.Handler) http.Handler {
	key := route.Method + " " + route.Path
	jsonSchema, hasJSONSchema := lightweightMutationSchemas[key]
	_, paginated := lightweightPaginatedRoutes[key]
	if !hasJSONSchema && !paginated {
		return nil
	}

	return func(next http.Handler) http.Handler {
		if hasJSONSchema {
			next = lightweightStrictJSON(key, jsonSchema)(next)
		}
		if paginated {
			next = lightweightPagination(next)
		}
		return next
	}
}

func lightweightStrictJSON(routeKey string, schema lightweightJSONSchema) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				writeLightweightTransportError(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			if schema.AllowMultipart && strings.EqualFold(mediaType, "multipart/form-data") {
				next.ServeHTTP(w, r)
				return
			}
			if !strings.EqualFold(mediaType, "application/json") {
				writeLightweightTransportError(w, http.StatusBadRequest, "invalid_argument")
				return
			}

			payload, err := readLightweightJSONBody(r.Body)
			if err != nil {
				writeLightweightTransportError(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			if len(bytes.TrimSpace(payload)) == 0 {
				if !schema.AllowEmpty {
					writeLightweightTransportError(w, http.StatusBadRequest, "invalid_argument")
					return
				}
				payload = []byte("{}")
			}

			fields, err := decodeLightweightJSONObject(payload)
			if err != nil || !validateLightweightJSONFields(fields, schema) {
				writeLightweightTransportError(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			if routeKey == "PUT /api/issues/{issueId}" && r.Header.Get("X-Actor-Source") == "task_token" {
				for name := range fields {
					switch name {
					case "status", "assignee_type", "assignee_id":
					default:
						writeLightweightTransportError(w, http.StatusForbidden, "forbidden")
						return
					}
				}
			}

			r.Body = io.NopCloser(bytes.NewReader(payload))
			r.ContentLength = int64(len(payload))
			next.ServeHTTP(w, r)
		})
	}
}

func readLightweightJSONBody(body io.Reader) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	limited := io.LimitReader(body, lightweightMaxJSONBody+1)
	payload, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(payload) > lightweightMaxJSONBody {
		return nil, errors.New("request body too large")
	}
	return payload, nil
}

func decodeLightweightJSONObject(payload []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var fields map[string]json.RawMessage
	if err := decoder.Decode(&fields); err != nil || fields == nil {
		if err == nil {
			err = errors.New("JSON body must be an object")
		}
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON body must contain one object")
	}
	return fields, nil
}

func validateLightweightJSONFields(fields map[string]json.RawMessage, schema lightweightJSONSchema) bool {
	for name, raw := range fields {
		field, ok := schema.Fields[name]
		if !ok {
			return false
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && !field.Nullable {
			return false
		}
	}
	for name, field := range schema.Fields {
		if field.Required {
			if _, ok := fields[name]; !ok {
				return false
			}
		}
	}
	return true
}

func lightweightPagination(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if err := normalizeLightweightLimit(query); err != nil {
			writeLightweightTransportError(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		if err := validateLightweightCursor(query.Get("cursor")); err != nil {
			writeLightweightTransportError(w, http.StatusBadRequest, "invalid_cursor")
			return
		}
		r.URL.RawQuery = query.Encode()
		next.ServeHTTP(w, r)
	})
}

func normalizeLightweightLimit(query url.Values) error {
	raw := query.Get("limit")
	if raw == "" {
		query.Set("limit", strconv.Itoa(lightweightDefaultLimit))
		return nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > lightweightMaxLimit {
		return fmt.Errorf("limit must be between 1 and %d", lightweightMaxLimit)
	}
	query.Set("limit", strconv.Itoa(limit))
	return nil
}

func validateLightweightCursor(cursor string) error {
	if cursor == "" {
		return nil
	}
	if len(cursor) > lightweightMaxCursor {
		return errors.New("cursor too long")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(decoded) == 0 || len(decoded) > lightweightMaxCursor {
		return errors.New("invalid opaque cursor")
	}
	return nil
}

func writeLightweightTransportError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}

func lightweightSchema(required, optional, nullable []string) lightweightJSONSchema {
	fields := make(map[string]lightweightJSONField, len(required)+len(optional)+len(nullable))
	for _, name := range required {
		fields[name] = lightweightJSONField{Required: true}
	}
	for _, name := range optional {
		fields[name] = lightweightJSONField{}
	}
	for _, name := range nullable {
		field := fields[name]
		field.Nullable = true
		fields[name] = field
	}
	return lightweightJSONSchema{Fields: fields}
}

func lightweightEmptySchema() lightweightJSONSchema {
	return lightweightJSONSchema{Fields: map[string]lightweightJSONField{}, AllowEmpty: true}
}

func lightweightImportSchema() lightweightJSONSchema {
	schema := lightweightSchema([]string{"url"}, []string{"on_conflict"}, nil)
	schema.AllowMultipart = true
	return schema
}

func lightweightAvatarSchema() lightweightJSONSchema {
	schema := lightweightEmptySchema()
	schema.AllowMultipart = true
	return schema
}

func lightweightToolSourceImportSchema() lightweightJSONSchema {
	schema := lightweightSchema(nil, nil, nil)
	schema.AllowMultipart = true
	return schema
}

// This allowlist is the wire DTO contract. Read-only, exited and legacy fields
// are absent by design, so they are rejected before a Handler can write.
var lightweightMutationSchemas = map[string]lightweightJSONSchema{
	"POST /auth/send-code":   lightweightSchema([]string{"email"}, nil, nil),
	"POST /auth/verify-code": lightweightSchema([]string{"email", "code"}, nil, nil),
	"POST /auth/logout":      lightweightEmptySchema(),

	"POST /api/tokens":               lightweightSchema([]string{"name"}, nil, []string{"expires_at"}),
	"POST /api/tokens/current/renew": lightweightEmptySchema(),
	"PATCH /api/me":                  lightweightSchema(nil, []string{"name", "language"}, []string{"timezone"}),
	"POST /api/workspaces":           lightweightSchema([]string{"name"}, []string{"slug"}, nil),
	"PUT /api/workspaces/{workspaceId}": lightweightSchema(nil,
		[]string{"name", "context", "settings", "repos"}, []string{"description"}),
	"POST /api/workspaces/{workspaceId}/runtime-profiles": lightweightSchema(
		[]string{"display_name", "protocol_family", "command_name"}, []string{"fixed_args", "enabled"}, []string{"description"}),
	"PUT /api/workspaces/{workspaceId}/runtime-profiles/{profileId}": lightweightSchema(nil,
		[]string{"display_name", "protocol_family", "command_name", "fixed_args", "enabled"}, []string{"description"}),

	"POST /api/tool-sources": lightweightSchema([]string{"name", "kind"},
		[]string{"endpoint", "transport_config", "artifact_id", "auth", "tools"}, nil),
	"POST /api/tool-sources/import": lightweightToolSourceImportSchema(),
	"PUT /api/tool-sources/{sourceId}": lightweightSchema(nil,
		[]string{"endpoint", "transport_config", "artifact_id", "auth", "tools"}, nil),
	"POST /api/tool-sources/{sourceId}/validate":  lightweightSchema([]string{"revision_id"}, nil, nil),
	"POST /api/tool-sources/{sourceId}/enable":    lightweightEmptySchema(),
	"POST /api/tool-sources/{sourceId}/disable":   lightweightEmptySchema(),
	"POST /api/tool-sources/{sourceId}/artifacts": lightweightSchema([]string{"media_type", "content_base64"}, nil, nil),
	"POST /api/tool-bundles/{bundleId}/revoke":    lightweightEmptySchema(),
	"PUT /api/agents/{agentId}/tool-bundle":       lightweightSchema([]string{"items"}, nil, nil),

	"PATCH /api/runtimes/{runtimeId}":                         lightweightSchema(nil, nil, []string{"custom_name"}),
	"POST /api/runtimes/{runtimeId}/models":                   lightweightEmptySchema(),
	"POST /api/runtimes/{runtimeId}/local-skills":             lightweightEmptySchema(),
	"POST /api/runtimes/{runtimeId}/local-skills/import":      lightweightSchema([]string{"skill_key"}, []string{"action", "target_skill_id", "supports_conflict"}, []string{"name", "description"}),
	"POST /api/runtimes/{runtimeId}/unbind-agents-and-delete": lightweightSchema([]string{"expected_active_agent_ids"}, nil, nil),

	"POST /api/agents": lightweightSchema([]string{"name", "runtime_id"}, []string{
		"description", "instructions", "model", "thinking_level", "service_tier", "runtime_config", "custom_args", "mcp_config",
		"max_concurrent_tasks", "permission_mode", "invocation_targets", "disabled_runtime_skills",
	}, nil),
	"PUT /api/agents/{agentId}": lightweightSchema(nil, []string{
		"name", "description", "instructions", "runtime_config", "custom_args", "mcp_config", "max_concurrent_tasks",
		"permission_mode", "invocation_targets", "disabled_runtime_skills",
	}, []string{"runtime_id", "model", "thinking_level", "service_tier"}),
	"POST /api/agents/{agentId}/archive":      lightweightEmptySchema(),
	"POST /api/agents/{agentId}/restore":      lightweightEmptySchema(),
	"POST /api/agents/{agentId}/tasks/cancel": lightweightEmptySchema(),
	"POST /api/agents/{agentId}/avatar":       lightweightAvatarSchema(),
	"PUT /api/agents/{agentId}/env":           lightweightSchema([]string{"custom_env"}, nil, nil),
	"PUT /api/agents/{agentId}/skills":        lightweightSchema([]string{"skills"}, nil, nil),

	"POST /api/agent-builder/sessions":                      lightweightSchema([]string{"runtime_id"}, []string{"model"}, nil),
	"PATCH /api/agent-builder/sessions/{sessionId}/runtime": lightweightSchema([]string{"runtime_id"}, []string{"model"}, nil),
	"PUT /api/agent-builder/sessions/{sessionId}/draft":     lightweightSchema([]string{"draft"}, []string{"finalize"}, nil),

	"POST /api/skills":                lightweightSchema([]string{"name"}, []string{"description", "content", "config"}, nil),
	"POST /api/skills/import":         lightweightImportSchema(),
	"PUT /api/skills/{skillId}":       lightweightSchema(nil, []string{"name", "description", "content", "config"}, nil),
	"PUT /api/skills/{skillId}/files": lightweightSchema([]string{"files"}, nil, nil),

	"POST /api/chat/sessions":                      lightweightSchema([]string{"agent_id"}, []string{"title"}, nil),
	"PATCH /api/chat/sessions/{sessionId}":         lightweightSchema(nil, []string{"title", "status"}, nil),
	"POST /api/chat/sessions/{sessionId}/messages": lightweightSchema([]string{"content"}, nil, nil),

	"POST /api/squads":                         lightweightSchema([]string{"name", "leader_id"}, []string{"description", "instructions", "additional_agent_ids"}, nil),
	"PUT /api/squads/{squadId}":                lightweightSchema(nil, []string{"name", "description", "leader_id", "instructions"}, nil),
	"POST /api/squads/{squadId}/avatar":        lightweightAvatarSchema(),
	"POST /api/squads/{squadId}/members":       lightweightSchema([]string{"agent_id", "role"}, nil, nil),
	"DELETE /api/squads/{squadId}/members":     lightweightSchema([]string{"agent_id"}, nil, nil),
	"PATCH /api/squads/{squadId}/members/role": lightweightSchema([]string{"agent_id", "role"}, nil, nil),

	"POST /api/issues": lightweightSchema([]string{"title", "status", "assignee_type", "assignee_id"}, []string{
		"description", "acceptance_criteria", "context_refs",
	}, nil),
	"PUT /api/issues/{issueId}": lightweightSchema(nil, []string{
		"title", "description", "status", "assignee_type", "assignee_id", "acceptance_criteria", "context_refs",
	}, nil),
	"POST /api/issues/{issueId}/comments":        lightweightSchema([]string{"content"}, nil, nil),
	"POST /api/issues/{issueId}/squad-evaluated": lightweightSchema([]string{"outcome"}, []string{"reason"}, nil),
	"POST /api/tasks/{taskId}/cancel":            lightweightEmptySchema(),

	http.MethodPost + " " + protocol.DaemonRouteRegister: lightweightSchema(
		[]string{"protocol_version", "workspace_id", "daemon_id", "runtimes"}, []string{"device_name", "cli_version", "failed_profiles"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteDeregister: lightweightSchema([]string{"runtime_ids"}, nil, nil),
	http.MethodPost + " " + protocol.DaemonRouteHeartbeat:  lightweightSchema([]string{"protocol_version", "runtime_id"}, []string{"supports_batch_import"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteTasksClaim: lightweightSchema(
		[]string{"daemon_id", "runtime_ids", "max_tasks"}, nil, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskPrepareLease:        lightweightEmptySchema(),
	http.MethodPost + " " + protocol.DaemonRouteTaskSkillBundlesResolve: lightweightSchema([]string{"skills"}, nil, nil),
	http.MethodPost + " " + protocol.DaemonRouteModelListResult: lightweightSchema(
		[]string{"status"}, []string{"models", "supported", "error", "fallback"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteLocalSkillListResult: lightweightSchema(
		[]string{"status"}, []string{"skills", "supported", "mcp_servers", "mcp_supported", "error"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteLocalSkillImportResult: lightweightSchema(
		[]string{"status"}, []string{"skill", "error"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskStart:              lightweightEmptySchema(),
	http.MethodPost + " " + protocol.DaemonRouteTaskWaitLocalDirectory: lightweightSchema(nil, []string{"reason"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskProgress:           lightweightSchema([]string{"summary"}, []string{"step", "total"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskMessages:           lightweightSchema([]string{"messages"}, nil, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskUsage:              lightweightSchema([]string{"usage"}, nil, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskComplete: lightweightSchema(nil,
		[]string{"output", "session_id", "work_dir", "session_rollout_missing", "retired_session_id"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskFail: lightweightSchema([]string{"error"},
		[]string{"session_id", "work_dir", "failure_reason", "session_rollout_missing", "retired_session_id"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteTaskCancelAck:          lightweightEmptySchema(),
	http.MethodPost + " " + protocol.DaemonRouteTaskSession:            lightweightSchema(nil, []string{"session_id", "work_dir"}, nil),
	http.MethodPost + " " + protocol.DaemonRouteWorkspaceIssueGCChecks: lightweightSchema([]string{"issue_ids"}, nil, nil),
	http.MethodPost + " " + protocol.DaemonRouteRuntimeRecoverOrphans:  lightweightEmptySchema(),
}

var lightweightPaginatedRoutes = map[string]struct{}{
	"GET /api/agents/{agentId}/tasks":             {},
	"GET /api/chat/sessions":                      {},
	"GET /api/chat/sessions/{sessionId}/messages": {},
	"GET /api/issues":                             {},
	"GET /api/issues/{issueId}/comments":          {},
	"GET /api/issues/{issueId}/task-runs":         {},
	"GET /api/tasks/{taskId}/messages":            {},
}
