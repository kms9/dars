package lightweightapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/kms9/dars/internal/auth"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type principalKind string

const (
	principalJWT    principalKind = "jwt"
	principalPAT    principalKind = "pat"
	principalDaemon principalKind = "daemon_token"
	principalTask   principalKind = "task_token"
)

type Principal struct {
	Kind         principalKind
	UserID       string
	PATID        string
	WorkspaceID  string
	DaemonID     string
	TaskID       string
	AgentID      string
	ToolBundleID string
}

type requestContextKey uint8

const (
	principalKey requestContextKey = iota
	workspaceKey
	memberKey
)

func principalFromContext(ctx context.Context) (Principal, bool) {
	value, ok := ctx.Value(principalKey).(Principal)
	return value, ok
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	return principalFromContext(ctx)
}

func WorkspaceIDFromContext(ctx context.Context) string {
	value, _ := ctx.Value(workspaceKey).(string)
	return value
}

func MemberFromContext(ctx context.Context) (lwdb.Member, bool) {
	value, ok := ctx.Value(memberKey).(lwdb.Member)
	return value, ok
}

func bearerOrCookie(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), false
	}
	cookie, err := r.Cookie(auth.AuthCookieName)
	if err == nil {
		return cookie.Value, true
	}
	return "", false
}

func (h *Handler) authenticateHuman(r *http.Request) (Principal, int) {
	token, fromCookie := bearerOrCookie(r)
	if token == "" {
		return Principal{}, http.StatusUnauthorized
	}
	if fromCookie && !auth.ValidateCSRF(r) {
		return Principal{}, http.StatusForbidden
	}
	if strings.HasPrefix(token, "dpat_") {
		pat, err := h.q.GetPersonalAccessTokenByHash(r.Context(), auth.HashToken(token))
		if err != nil {
			return Principal{}, http.StatusUnauthorized
		}
		_, _ = h.q.TouchPersonalAccessToken(r.Context(), pat.ID)
		return Principal{Kind: principalPAT, UserID: uuidString(pat.UserID), PATID: uuidString(pat.ID)}, 0
	}
	if strings.HasPrefix(token, "ddt_") || strings.HasPrefix(token, "dat_") || strings.HasPrefix(token, "dcn_") {
		return Principal{}, http.StatusForbidden
	}
	parsed, err := jwt.Parse(token, func(candidate *jwt.Token) (any, error) {
		if candidate.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, jwt.ErrSignatureInvalid
		}
		return auth.JWTSecret(), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return Principal{}, http.StatusUnauthorized
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Principal{}, http.StatusUnauthorized
	}
	subject, _ := claims["sub"].(string)
	userID, err := parseUUID(subject)
	if err != nil {
		return Principal{}, http.StatusUnauthorized
	}
	if _, err := h.q.GetUserByID(r.Context(), userID); err != nil {
		return Principal{}, http.StatusUnauthorized
	}
	return Principal{Kind: principalJWT, UserID: subject}, 0
}

func (h *Handler) HumanAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-Actor-Source")
		r.Header.Del("X-Agent-ID")
		r.Header.Del("X-Task-ID")
		principal, status := h.authenticateHuman(r)
		if status != 0 {
			code := "unauthenticated"
			if status == http.StatusForbidden {
				code = "forbidden"
			}
			writeCode(w, status, code)
			return
		}
		r.Header.Set("X-User-ID", principal.UserID)
		ctx := context.WithValue(r.Context(), principalKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) HumanOrTaskAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, _ := bearerOrCookie(r)
		if strings.HasPrefix(token, "dat_") {
			h.TaskAuth(next).ServeHTTP(w, r)
			return
		}
		h.HumanAuth(next).ServeHTTP(w, r)
	})
}

func (h *Handler) DaemonAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, fromCookie := bearerOrCookie(r)
		if fromCookie || !strings.HasPrefix(token, "ddt_") {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		stored, err := h.q.GetDaemonTokenByHash(r.Context(), auth.HashToken(token))
		if err != nil {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		principal := Principal{
			Kind: principalDaemon, WorkspaceID: uuidString(stored.WorkspaceID), DaemonID: stored.DaemonID,
		}
		r.Header.Del("X-User-ID")
		r.Header.Del("X-Workspace-Slug")
		r.Header.Set("X-Actor-Source", string(principalDaemon))
		r.Header.Set("X-Workspace-ID", principal.WorkspaceID)
		r.Header.Set("X-Daemon-ID", principal.DaemonID)
		ctx := context.WithValue(r.Context(), principalKey, principal)
		ctx = context.WithValue(ctx, workspaceKey, principal.WorkspaceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) TaskAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, fromCookie := bearerOrCookie(r)
		if fromCookie || !strings.HasPrefix(token, "dat_") {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		stored, err := h.q.GetTaskTokenByHash(r.Context(), auth.HashToken(token))
		if err != nil {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		task, err := h.q.GetAgentTask(r.Context(), lwdb.GetAgentTaskParams{ID: stored.TaskID, WorkspaceID: stored.WorkspaceID})
		if err != nil || task.AgentID != stored.AgentID || task.ToolBundleID != stored.ToolBundleID {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		switch task.Status {
		case "dispatched", "waiting_local_directory", "running":
		default:
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		principal := Principal{
			Kind: principalTask, UserID: uuidString(stored.UserID), WorkspaceID: uuidString(stored.WorkspaceID),
			TaskID: uuidString(stored.TaskID), AgentID: uuidString(stored.AgentID),
		}
		if stored.ToolBundleID.Valid {
			principal.ToolBundleID = stored.ToolBundleID.String
		}
		r.Header.Del("X-Workspace-Slug")
		r.Header.Set("X-Actor-Source", string(principalTask))
		r.Header.Set("X-User-ID", principal.UserID)
		r.Header.Set("X-Workspace-ID", principal.WorkspaceID)
		r.Header.Set("X-Task-ID", principal.TaskID)
		r.Header.Set("X-Agent-ID", principal.AgentID)
		ctx := context.WithValue(r.Context(), principalKey, principal)
		ctx = context.WithValue(ctx, workspaceKey, principal.WorkspaceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) RequireWorkspaceMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := principalFromContext(r.Context())
		if !ok {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		if principal.Kind == principalTask {
			ctx := context.WithValue(r.Context(), workspaceKey, principal.WorkspaceID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		if principal.Kind != principalJWT && principal.Kind != principalPAT {
			writeCode(w, http.StatusForbidden, "forbidden")
			return
		}
		workspaceHeader := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
		workspaceID, err := parseUUID(workspaceHeader)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
		pathWorkspace := strings.TrimSpace(chi.URLParam(r, "workspaceId"))
		if pathWorkspace != "" && pathWorkspace != workspaceHeader {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		userID, err := parseUUID(principal.UserID)
		if err != nil {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		member, err := h.q.GetMember(r.Context(), lwdb.GetMemberParams{WorkspaceID: workspaceID, UserID: userID})
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found")
			return
		}
		r.Header.Del("X-Workspace-Slug")
		ctx := context.WithValue(r.Context(), workspaceKey, workspaceHeader)
		ctx = context.WithValue(ctx, memberKey, member)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) RequireWorkspaceRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			member, ok := MemberFromContext(r.Context())
			if !ok {
				writeCode(w, http.StatusForbidden, "forbidden")
				return
			}
			if _, ok := allowed[member.Role]; !ok {
				writeCode(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
