// Package lightweightapi implements the HTTP and authorization core that is
// backed exclusively by the checked-in Lightweight schema manifest.
package lightweightapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kms9/dars/internal/agentconfigsecret"
	"github.com/kms9/dars/internal/daemonws"
	"github.com/kms9/dars/internal/events"
	"github.com/kms9/dars/internal/util"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

const (
	verificationTTL    = 10 * time.Minute
	maxVerificationTry = int32(5)
	defaultPATTTL      = 90 * 24 * time.Hour
	defaultDaemonTTL   = 30 * 24 * time.Hour
)

const protocolVersion = protocol.DaemonProtocolVersion

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type EmailSender interface {
	SendVerificationCode(to, code string) error
}

// ClaimMCPProjector creates the ephemeral Gateway-only MCP document for a
// Bundle-pinned Task. Implementations must not persist the plaintext token.
type ClaimMCPProjector interface {
	ProjectClaimMCPConfig(bundleID, provider, taskToken string) (json.RawMessage, error)
}

// GatewayProviderPolicy is the Server-owned compatibility gate for Runtime
// providers that can consume the unchanged Claim MCP document.
type GatewayProviderPolicy interface {
	ProviderSupported(provider string) bool
}

type Config struct {
	Production                bool
	MailConfigured            bool
	AllowSignup               bool
	AllowedEmails             []string
	AllowedEmailDomains       []string
	DevelopmentCode           string
	ServerVersion             string
	WorkspaceCreationDisabled bool
	AgentSecrets              *agentconfigsecret.Codec
	DaemonHub                 *daemonws.Hub
	Wakeup                    DaemonWakeupNotifier
	Heartbeat                 HeartbeatScheduler
	EventBus                  *events.Bus
	AvatarStore               AvatarObjectStore
	ClaimMCPProjector         ClaimMCPProjector
	GatewayProviderPolicy     GatewayProviderPolicy
}

type DaemonWakeupNotifier interface {
	NotifyTaskAvailable(runtimeID, taskID string)
	NotifyPendingWork(runtimeID, kind string)
}

type HeartbeatScheduler interface {
	Schedule(runtimeID pgtype.UUID)
}

type Handler struct {
	pool     *pgxpool.Pool
	q        *lwdb.Queries
	email    EmailSender
	cfg      Config
	now      func() time.Time
	requests *runtimeRequestStore
	avatars  AvatarObjectStore
}

func New(pool *pgxpool.Pool, email EmailSender, cfg Config) *Handler {
	avatars := cfg.AvatarStore
	if avatars == nil {
		avatars = newMemoryAvatarStore()
	}
	return &Handler{
		pool: pool, q: lwdb.New(pool), email: email, cfg: cfg, now: time.Now,
		requests: newRuntimeRequestStore(),
		avatars:  avatars,
	}
}

func (h *Handler) Queries() *lwdb.Queries { return h.q }

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func writeCode(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"code": code})
}

func parseUUID(value string) (pgtype.UUID, error) {
	return util.ParseUUID(strings.TrimSpace(value))
}

func uuidString(value pgtype.UUID) string { return util.UUIDToString(value) }

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timePointer(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func timeString(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func notFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

func stringAllowed(value string, exact, domains []string) bool {
	for _, item := range exact {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	at := strings.LastIndex(value, "@")
	if at < 0 {
		return false
	}
	domain := value[at+1:]
	for _, item := range domains {
		if strings.EqualFold(strings.TrimSpace(item), domain) {
			return true
		}
	}
	return false
}

func (h *Handler) signupAllowed(email string, exists bool) bool {
	if exists {
		return true
	}
	if stringAllowed(email, h.cfg.AllowedEmails, h.cfg.AllowedEmailDomains) {
		return true
	}
	if len(h.cfg.AllowedEmails) > 0 || len(h.cfg.AllowedEmailDomains) > 0 {
		return false
	}
	return h.cfg.AllowSignup
}

func beginTx(r *http.Request, pool *pgxpool.Pool) (pgx.Tx, bool) {
	tx, err := pool.Begin(r.Context())
	if err != nil {
		return nil, false
	}
	return tx, true
}
