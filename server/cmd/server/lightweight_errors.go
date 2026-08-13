package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	chimw "github.com/go-chi/chi/v5/middleware"
)

type lightweightErrorBody struct {
	Error lightweightError `json:"error"`
}

type lightweightError struct {
	Code    string                  `json:"code"`
	Message string                  `json:"message"`
	Details lightweightErrorDetails `json:"details"`
}

type lightweightErrorDetails struct {
	RequestID string `json:"request_id,omitempty"`
}

var lightweightErrorMessages = map[string]string{
	"invalid_argument":                  "Invalid request.",
	"unauthenticated":                   "Authentication required.",
	"forbidden":                         "Access denied.",
	"not_found":                         "Resource not found.",
	"conflict":                          "Request conflicts with current state.",
	"invalid_cursor":                    "Invalid cursor.",
	"database_identity_mismatch":        "Database identity does not match this server.",
	"protocol_version_unsupported":      "Protocol version is unsupported.",
	"agent_runtime_required":            "An agent runtime is required.",
	"agent_archived":                    "The agent is archived.",
	"invocation_forbidden":              "Agent invocation is not allowed.",
	"runtime_unavailable":               "The runtime is unavailable.",
	"active_tasks_exist":                "Active tasks prevent this operation.",
	"idempotency_key_reused":            "The idempotency key was reused with a different request.",
	"task_state_conflict":               "The task state does not allow this operation.",
	"internal_error":                    "Internal server error.",
	"tool_source_invalid":               "Tool source configuration is invalid.",
	"tool_source_conflict":              "Tool source conflicts with current state.",
	"tool_source_revision_conflict":     "Tool source revision conflicts with current state.",
	"tool_source_secret_invalid":        "Tool source authentication is invalid.",
	"tool_source_artifact_invalid":      "Tool source artifact is invalid.",
	"tool_source_state_conflict":        "Tool source state transition is invalid.",
	"tool_source_retained":              "Tool source is retained by a bundle.",
	"tool_source_validator_unavailable": "Tool source validator is unavailable.",
	"tool_name_conflict":                "Tool name conflicts within the bundle.",
	"tool_bundle_invalid":               "Tool bundle configuration is invalid.",
	"tool_bundle_revoked":               "Tool bundle is revoked.",
	"provider_mcp_unsupported":          "Agent provider does not support Server MCP.",
}

// lightweightErrorEnvelope normalizes every target error at the outer HTTP
// boundary. Existing handlers can be migrated incrementally without leaking
// their raw SQL/path/error strings because error bodies are buffered and
// replaced before reaching the client. Successful responses and WebSocket
// upgrades stream through unchanged.
func lightweightErrorEnvelope(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writer := &lightweightErrorWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		if writer.status < http.StatusBadRequest {
			return
		}

		code := lightweightErrorCode(writer.status, writer.body.Bytes())
		message := lightweightErrorMessages[code]
		if message == "" {
			code = "internal_error"
			message = lightweightErrorMessages[code]
		}
		response := lightweightErrorBody{Error: lightweightError{
			Code:    code,
			Message: message,
			Details: lightweightErrorDetails{RequestID: chimw.GetReqID(r.Context())},
		}}
		body, err := json.Marshal(response)
		if err != nil {
			body = []byte(`{"error":{"code":"internal_error","message":"Internal server error.","details":{}}}`)
			writer.status = http.StatusInternalServerError
		}
		body = append(body, '\n')
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Type", "application/json")
		if requestID := chimw.GetReqID(r.Context()); requestID != "" {
			w.Header().Set("X-Request-ID", requestID)
		}
		w.WriteHeader(writer.status)
		_, _ = w.Write(body)
	})
}

func lightweightErrorCode(status int, body []byte) string {
	var decoded struct {
		Code  string `json:"code"`
		Error any    `json:"error"`
	}
	if json.Unmarshal(body, &decoded) == nil {
		if _, ok := lightweightErrorMessages[decoded.Code]; ok {
			return decoded.Code
		}
		if nested, ok := decoded.Error.(map[string]any); ok {
			if code, ok := nested["code"].(string); ok {
				if _, supported := lightweightErrorMessages[code]; supported {
					return code
				}
			}
		}
		if message, ok := decoded.Error.(string); ok {
			lower := strings.ToLower(message)
			switch {
			case strings.Contains(lower, "cursor"):
				return "invalid_cursor"
			case strings.Contains(lower, "active task"):
				return "active_tasks_exist"
			case strings.Contains(lower, "runtime") && strings.Contains(lower, "required"):
				return "agent_runtime_required"
			}
		}
	}

	switch status {
	case http.StatusBadRequest, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return "invalid_argument"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	default:
		return "internal_error"
	}
}

type lightweightErrorWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *lightweightErrorWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	if status < http.StatusBadRequest {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *lightweightErrorWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.status >= http.StatusBadRequest {
		return w.body.Write(body)
	}
	return w.ResponseWriter.Write(body)
}

func (w *lightweightErrorWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if w.status >= http.StatusBadRequest {
		return
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *lightweightErrorWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *lightweightErrorWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *lightweightErrorWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
