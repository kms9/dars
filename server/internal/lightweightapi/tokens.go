package lightweightapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/auth"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type tokenResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"token_prefix"`
	ExpiresAt  *string `json:"expires_at"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

type tokenSecretResponse struct {
	tokenResponse
	Token string `json:"token"`
}

func toTokenResponse(token lwdb.PersonalAccessToken) tokenResponse {
	return tokenResponse{
		ID: uuidString(token.ID), Name: token.Name, Prefix: token.TokenPrefix,
		ExpiresAt: timePointer(token.ExpiresAt), LastUsedAt: timePointer(token.LastUsedAt),
		CreatedAt: timeString(token.CreatedAt),
	}
}

type createTokenRequest struct {
	Name      string          `json:"name"`
	ExpiresAt json.RawMessage `json:"expires_at"`
}

func (h *Handler) CreatePersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var request createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || len(request.Name) > 120 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	expiresAt := pgtype.Timestamptz{Time: h.now().Add(defaultPATTTL), Valid: true}
	if request.ExpiresAt != nil {
		if string(request.ExpiresAt) == "null" {
			expiresAt = pgtype.Timestamptz{}
		} else {
			var rawExpiry string
			if err := json.Unmarshal(request.ExpiresAt, &rawExpiry); err != nil {
				writeCode(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			parsed, err := parseRFC3339Future(rawExpiry, h.now())
			if err != nil {
				writeCode(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			expiresAt = pgtype.Timestamptz{Time: parsed, Valid: true}
		}
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	plaintext, err := auth.GeneratePATToken()
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	row, err := h.q.CreatePersonalAccessToken(r.Context(), lwdb.CreatePersonalAccessTokenParams{
		UserID: userID, Name: request.Name, TokenHash: auth.HashToken(plaintext),
		TokenPrefix: tokenPrefix(plaintext), ExpiresAt: expiresAt,
	})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, tokenSecretResponse{tokenResponse: toTokenResponse(row), Token: plaintext})
}

func (h *Handler) ListPersonalAccessTokens(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	rows, err := h.q.ListPersonalAccessTokens(r.Context(), userID)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	response := make([]tokenResponse, 0, len(rows))
	for _, row := range rows {
		response = append(response, toTokenResponse(row))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) RenewCurrentPersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.Kind != principalPAT || principal.PATID == "" {
		writeCode(w, http.StatusForbidden, "forbidden")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	tokenID, err := parseUUID(principal.PATID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	plaintext, err := auth.GeneratePATToken()
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	row, err := h.q.RenewPersonalAccessToken(r.Context(), lwdb.RenewPersonalAccessTokenParams{
		ID: tokenID, UserID: userID, TokenHash: auth.HashToken(plaintext), TokenPrefix: tokenPrefix(plaintext),
		ExpiresAt: pgtype.Timestamptz{Time: h.now().Add(defaultPATTTL), Valid: true},
	})
	if err != nil {
		if notFound(err) {
			writeCode(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, tokenSecretResponse{tokenResponse: toTokenResponse(row), Token: plaintext})
}

func (h *Handler) RevokePersonalAccessToken(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	tokenID, err := parseUUID(chi.URLParam(r, "tokenId"))
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	count, err := h.q.RevokePersonalAccessToken(r.Context(), lwdb.RevokePersonalAccessTokenParams{ID: tokenID, UserID: userID})
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if count != 1 {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func tokenPrefix(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}

func parseRFC3339Future(value string, now time.Time) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil || !parsed.After(now) {
		return time.Time{}, errors.New("expiry must be a future RFC3339 timestamp")
	}
	return parsed, nil
}
