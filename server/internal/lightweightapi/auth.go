package lightweightapi

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kms9/dars/internal/auth"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

type sendCodeRequest struct {
	Email string `json:"email"`
}

type verifyCodeRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type userResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	Language  string  `json:"language"`
	Timezone  *string `json:"timezone"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

type loginResponse struct {
	Token string       `json:"token"`
	User  userResponse `json:"user"`
}

func toUserResponse(user lwdb.User) userResponse {
	return userResponse{
		ID:        uuidString(user.ID),
		Name:      user.Name,
		Email:     user.Email,
		Language:  user.Language,
		Timezone:  textPointer(user.Timezone),
		CreatedAt: timeString(user.CreatedAt),
		UpdatedAt: timeString(user.UpdatedAt),
	}
}

func generateVerificationCode() (string, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", binary.BigEndian.Uint32(raw[:])%1_000_000), nil
}

func validSixDigitCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func verificationCodeHash(email, code string) string {
	mac := hmac.New(sha256.New, auth.JWTSecret())
	_, _ = mac.Write([]byte(strings.ToLower(email)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Handler) SendCode(w http.ResponseWriter, r *http.Request) {
	var request sendCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	email := strings.ToLower(strings.TrimSpace(request.Email))
	if !emailPattern.MatchString(email) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}

	// Production must never fall through to EmailService's development stdout
	// transport. Readiness reports the same condition, while this branch keeps
	// the mutation fail-closed even if an operator ignores readiness.
	if h.cfg.Production && !h.cfg.MailConfigured {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}

	_, lookupErr := h.q.GetUserByEmail(r.Context(), email)
	exists := lookupErr == nil
	if lookupErr != nil && !notFound(lookupErr) {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}

	// Enumeration shield: a syntactically valid email receives the same 202
	// whether it exists or is eligible for signup. Ineligible new addresses do
	// not get a row or an email.
	if !h.signupAllowed(email, exists) {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	code, err := generateVerificationCode()
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if !h.cfg.Production && validSixDigitCode(h.cfg.DevelopmentCode) {
		code = h.cfg.DevelopmentCode
	}
	if _, err := h.q.CreateVerificationCode(r.Context(), lwdb.CreateVerificationCodeParams{
		Email:     email,
		CodeHash:  verificationCodeHash(email, code),
		ExpiresAt: pgtype.Timestamptz{Time: h.now().Add(verificationTTL), Valid: true},
	}); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if h.email == nil || h.email.SendVerificationCode(email, code) != nil {
		writeCode(w, http.StatusServiceUnavailable, "internal_error")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) VerifyCode(w http.ResponseWriter, r *http.Request) {
	var request verifyCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	email := strings.ToLower(strings.TrimSpace(request.Email))
	code := strings.TrimSpace(request.Code)
	if !emailPattern.MatchString(email) || !validSixDigitCode(code) {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	defer tx.Rollback(r.Context())
	qtx := h.q.WithTx(tx)
	stored, err := qtx.GetLatestUsableVerificationCode(r.Context(), email)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	want := verificationCodeHash(email, code)
	if subtle.ConstantTimeCompare([]byte(want), []byte(stored.CodeHash)) != 1 {
		attempts, bumpErr := qtx.IncrementVerificationCodeAttempts(r.Context(), stored.ID)
		if bumpErr != nil && !errors.Is(bumpErr, pgx.ErrNoRows) {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error")
			return
		}
		_ = attempts == maxVerificationTry
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}

	consumed, err := qtx.ConsumeVerificationCode(r.Context(), stored.ID)
	if err != nil || consumed != 1 {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	user, err := qtx.GetUserByEmail(r.Context(), email)
	if notFound(err) {
		if !h.signupAllowed(email, false) {
			writeCode(w, http.StatusForbidden, "forbidden")
			return
		}
		name := email[:strings.LastIndex(email, "@")]
		user, err = qtx.CreateUser(r.Context(), lwdb.CreateUserParams{
			Name: name, Email: email, Language: "en",
		})
	}
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}

	token, err := issueJWT(user)
	if err != nil || auth.SetAuthCookies(w, token) != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, User: toUserResponse(user)})
}

func issueJWT(user lwdb.User) (string, error) {
	now := jwt.NewNumericDate(time.Now())
	claims := jwt.MapClaims{
		"sub": uuidString(user.ID), "email": user.Email, "name": user.Name,
		"iat": now.Unix(), "exp": now.Add(auth.AuthTokenTTL()).Unix(),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(auth.JWTSecret())
}

func (h *Handler) Logout(w http.ResponseWriter, _ *http.Request) {
	auth.ClearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok || principal.UserID == "" {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	user, err := h.q.GetUserByID(r.Context(), userID)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}

type updateMeRequest struct {
	Name     *string         `json:"name"`
	Language *string         `json:"language"`
	Timezone json.RawMessage `json:"timezone"`
}

func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := principalFromContext(r.Context())
	if !ok {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var request updateMeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_argument")
		return
	}
	if request.Name != nil {
		*request.Name = strings.TrimSpace(*request.Name)
		if *request.Name == "" || len(*request.Name) > 120 {
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	if request.Language != nil {
		switch *request.Language {
		case "en", "zh-Hans", "ko", "ja":
		default:
			writeCode(w, http.StatusBadRequest, "invalid_argument")
			return
		}
	}
	userID, err := parseUUID(principal.UserID)
	if err != nil {
		writeCode(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	params := lwdb.UpdateUserProfileParams{ID: userID}
	if request.Name != nil {
		params.SetName, params.Name = true, *request.Name
	}
	if request.Language != nil {
		params.SetLanguage, params.Language = true, *request.Language
	}
	if request.Timezone != nil {
		params.SetTimezone = true
		if string(request.Timezone) != "null" {
			var timezone string
			if err := json.Unmarshal(request.Timezone, &timezone); err != nil || len(timezone) > 120 {
				writeCode(w, http.StatusBadRequest, "invalid_argument")
				return
			}
			params.Timezone = pgtype.Text{String: timezone, Valid: timezone != ""}
		}
	}
	user, err := h.q.UpdateUserProfile(r.Context(), params)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, toUserResponse(user))
}
