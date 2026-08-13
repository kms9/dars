package mcpgateway

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ErrUnauthorized = errors.New("MCP Bundle access is not authorized")

// Identity is the trusted Task identity resolved by existing Server auth.
type Identity struct {
	UserID       string
	WorkspaceID  string
	AgentID      string
	TaskID       string
	ToolBundleID string
}

func (i Identity) valid() bool {
	return i.UserID != "" && i.WorkspaceID != "" && i.AgentID != "" && i.TaskID != "" && ValidBundleID(i.ToolBundleID)
}

// IdentityResolver adapts the existing Server request principal into the
// Gateway without making this package depend on Lightweight Handler internals.
type IdentityResolver interface {
	ResolveIdentity(context.Context) (Identity, bool)
}

type IdentityResolverFunc func(context.Context) (Identity, bool)

func (f IdentityResolverFunc) ResolveIdentity(ctx context.Context) (Identity, bool) {
	return f(ctx)
}

// BundleAccess is the complete trusted authority tuple for one request.
type BundleAccess struct {
	Identity Identity
	BundleID string
}

// Store performs database-authoritative Token/Task/Agent/Workspace/Bundle-pin
// authorization. Implementations must return the same failure for missing and
// mismatched records so callers cannot enumerate Bundle IDs.
type Store interface {
	AuthorizeTaskBundle(context.Context, BundleAccess) error
}

type StoreFunc func(context.Context, BundleAccess) error

func (f StoreFunc) AuthorizeTaskBundle(ctx context.Context, access BundleAccess) error {
	return f(ctx, access)
}

// ToolRegistration is one immutable Bundle tool projected into the official
// MCP server. Its handler must use the fixed invocation plan captured by the
// Bundle item rather than accepting endpoint or identity overrides.
type ToolRegistration struct {
	Tool    *mcp.Tool
	Handler mcp.ToolHandler
}

// Projector materializes an authorized immutable Bundle as MCP tools.
type Projector interface {
	ProjectBundleTools(context.Context, BundleAccess) ([]ToolRegistration, error)
}

type ProjectorFunc func(context.Context, BundleAccess) ([]ToolRegistration, error)

func (f ProjectorFunc) ProjectBundleTools(ctx context.Context, access BundleAccess) ([]ToolRegistration, error) {
	return f(ctx, access)
}

type bundleContextKey struct{}

// WithBundleID carries a Chi path value to the transport-neutral Gateway.
func WithBundleID(r *http.Request, bundleID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), bundleContextKey{}, bundleID))
}

func bundleIDFromContext(ctx context.Context) string {
	bundleID, _ := ctx.Value(bundleContextKey{}).(string)
	return strings.TrimSpace(bundleID)
}

// ValidBundleID validates only the opaque ID syntax. Authorization never
// follows from knowledge of the ID.
func ValidBundleID(value string) bool {
	if !strings.HasPrefix(value, "tb_") || len(value) < 8 || len(value) > 80 {
		return false
	}
	for _, r := range value[3:] {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
