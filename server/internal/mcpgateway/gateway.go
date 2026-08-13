package mcpgateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Dependencies are the narrow adapters supplied by the Server composition
// root. The Gateway owns transport lifecycle, not authentication persistence.
type Dependencies struct {
	IdentityResolver IdentityResolver
	Store            Store
	Projector        Projector
	Version          string
	Resources        []GatewayResource
}

type GatewayResource interface {
	Close() error
}

// Gateway owns the Server-side MCP transport and all future adapter resources.
type Gateway struct {
	config            Config
	deps              Dependencies
	handler           http.Handler
	ctx               context.Context
	cancel            context.CancelFunc
	closed            atomic.Bool
	closeOnce         sync.Once
	resourceCloseOnce sync.Once
	resourceCloseErr  error
	lifecycle         sync.Mutex
	wg                sync.WaitGroup
}

func New(config Config, deps Dependencies) (*Gateway, error) {
	if deps.IdentityResolver == nil {
		return nil, errors.New("MCP Gateway identity resolver is required")
	}
	if deps.Store == nil {
		return nil, errors.New("MCP Gateway store is required")
	}
	if deps.Projector == nil {
		return nil, errors.New("MCP Gateway projector is required")
	}
	if config.MaxRequestBodyBytes <= 0 {
		return nil, errors.New("MCP Gateway request body limit must be positive")
	}
	if deps.Version == "" {
		deps.Version = "dev"
	}

	ctx, cancel := context.WithCancel(context.Background())
	gateway := &Gateway{config: config, deps: deps, ctx: ctx, cancel: cancel}
	gateway.handler = mcp.NewStreamableHTTPHandler(gateway.serverForRequest, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		MaxRequestBodyBytes:          config.MaxRequestBodyBytes,
		PropagateRequestCancellation: true,
		Logger:                       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return gateway, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.closed.Load() {
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	bundleID := bundleIDFromContext(r.Context())
	identity, ok := g.deps.IdentityResolver.ResolveIdentity(r.Context())
	if !ok || !identity.valid() || !ValidBundleID(bundleID) || identity.ToolBundleID != bundleID {
		writeUnauthorized(w)
		return
	}
	access := BundleAccess{Identity: identity, BundleID: bundleID}
	if err := g.deps.Store.AuthorizeTaskBundle(r.Context(), access); err != nil {
		writeUnauthorized(w)
		return
	}

	g.lifecycle.Lock()
	if g.closed.Load() {
		g.lifecycle.Unlock()
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	g.wg.Add(1)
	g.lifecycle.Unlock()
	defer g.wg.Done()
	requestCtx, cancel := context.WithCancel(r.Context())
	stopGatewayCancel := context.AfterFunc(g.ctx, cancel)
	defer func() {
		stopGatewayCancel()
		cancel()
	}()
	requestCtx = context.WithValue(requestCtx, bundleAccessContextKey{}, access)
	g.handler.ServeHTTP(w, r.WithContext(requestCtx))
}

func (g *Gateway) serverForRequest(r *http.Request) *mcp.Server {
	access, ok := r.Context().Value(bundleAccessContextKey{}).(BundleAccess)
	if !ok {
		return nil
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "dars", Version: g.deps.Version}, &mcp.ServerOptions{
		Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}},
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	registrations, err := g.deps.Projector.ProjectBundleTools(r.Context(), access)
	if err != nil {
		return nil
	}
	for _, registration := range registrations {
		if registration.Tool == nil || registration.Handler == nil {
			return nil
		}
		server.AddTool(registration.Tool, registration.Handler)
	}
	return server
}

// Close stops accepting new work, cancels Gateway-owned background resources,
// and waits for in-flight transport requests within the caller's deadline.
func (g *Gateway) Close(ctx context.Context) error {
	g.closeOnce.Do(func() {
		g.lifecycle.Lock()
		g.closed.Store(true)
		g.cancel()
		g.lifecycle.Unlock()
	})
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		g.resourceCloseOnce.Do(func() {
			for _, resource := range g.deps.Resources {
				if resource != nil {
					g.resourceCloseErr = errors.Join(g.resourceCloseErr, resource.Close())
				}
			}
		})
		return g.resourceCloseErr
	case <-ctx.Done():
		return fmt.Errorf("close MCP Gateway: %w", ctx.Err())
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
}

type bundleAccessContextKey struct{}
