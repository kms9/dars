package main

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/kms9/dars/internal/agentconfigsecret"
	"github.com/kms9/dars/internal/daemonws"
	"github.com/kms9/dars/internal/events"
	"github.com/kms9/dars/internal/lightweightapi"
	"github.com/kms9/dars/internal/mcpgateway"
	obsmetrics "github.com/kms9/dars/internal/metrics"
	"github.com/kms9/dars/internal/middleware"
	"github.com/kms9/dars/internal/realtime"
	"github.com/kms9/dars/internal/service"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/kms9/dars/pkg/protocol"
)

// LightweightComposition is the server's single shipping composition root.
// It exposes only the lifecycle objects main needs; every request dependency is
// owned by Handler and every route is registered from one manifest below.
type LightweightComposition struct {
	Router             chi.Router
	Core               *lightweightapi.Handler
	Queries            *lwdb.Queries
	HeartbeatScheduler *lightweightapi.BatchedHeartbeatScheduler
	Gateway            *mcpgateway.Gateway
	ToolControl        *mcpgateway.ControlPlane
}

// LightweightCompositionOptions contains process-owned dependencies whose
// lifecycle starts outside the router. There is deliberately no edition or
// Full/Light switch: this composition is the product.
type LightweightCompositionOptions struct {
	HTTPMetrics        *obsmetrics.HTTPMetrics
	DaemonHub          *daemonws.Hub
	DaemonWakeup       lightweightapi.DaemonWakeupNotifier
	HeartbeatScheduler *lightweightapi.BatchedHeartbeatScheduler
	GatewayConfig      *mcpgateway.Config
	GatewayStore       mcpgateway.Store
	GatewayProjector   mcpgateway.Projector
	GatewayProviders   map[string]bool
	GatewayObserver    mcpgateway.Observer
}

// NewRouter is the test convenience entry point for the same composition used
// by main. It does not select between server editions.
func NewRouter(pool *pgxpool.Pool, hub *realtime.Hub, bus *events.Bus, rdb *redis.Client) chi.Router {
	composition, err := NewLightweightComposition(pool, hub, bus, rdb, LightweightCompositionOptions{})
	if err != nil {
		panic(err)
	}
	return composition.Router
}

// NewLightweightComposition builds the complete target request graph.
func NewLightweightComposition(
	pool *pgxpool.Pool,
	hub *realtime.Hub,
	bus *events.Bus,
	rdb *redis.Client,
	opts LightweightCompositionOptions,
) (*LightweightComposition, error) {
	queries := lwdb.New(pool)
	daemonHub := opts.DaemonHub
	if daemonHub == nil {
		daemonHub = daemonws.NewHub()
	}

	emailService := service.NewEmailService()
	agentSecrets, _ := agentconfigsecret.LoadFromEnv()
	gatewayProviders := opts.GatewayProviders
	if gatewayProviders == nil {
		gatewayProviders = mcpgateway.ProviderCompatibilityAllowlist()
	}
	gatewayConfig := opts.GatewayConfig
	if gatewayConfig == nil {
		loaded, err := mcpgateway.ConfigFromEnv()
		if err != nil {
			return nil, err
		}
		gatewayConfig = &loaded
	}
	claimProjector := mcpgateway.NewClaimProjector(*gatewayConfig, gatewayProviders)
	core := lightweightapi.New(pool, emailService, lightweightapi.Config{
		Production:                strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production"),
		MailConfigured:            productionEmailConfigured(),
		AllowSignup:               os.Getenv("ALLOW_SIGNUP") != "false",
		AllowedEmails:             splitAndTrim(os.Getenv("ALLOWED_EMAILS")),
		AllowedEmailDomains:       splitAndTrim(os.Getenv("ALLOWED_EMAIL_DOMAINS")),
		DevelopmentCode:           strings.TrimSpace(os.Getenv("DARS_DEV_VERIFICATION_CODE")),
		ServerVersion:             normalizeServerVersion(version),
		WorkspaceCreationDisabled: os.Getenv("DISABLE_WORKSPACE_CREATION") == "true",
		AgentSecrets:              agentSecrets,
		DaemonHub:                 daemonHub,
		Wakeup:                    opts.DaemonWakeup,
		Heartbeat:                 opts.HeartbeatScheduler,
		EventBus:                  bus,
		ClaimMCPProjector:         claimProjector,
		GatewayProviderPolicy:     claimProjector,
	})

	daemonHub.SetHeartbeatHandler(core.HandleDaemonWSHeartbeat)
	daemonHub.SetRPCHandler(core.DaemonRPCHandler)

	sourceSecretCodec, _ := mcpgateway.LoadSourceSecretCodecFromEnv()
	egressPolicy := mcpgateway.NewEgressPolicy(*gatewayConfig, mcpgateway.EgressPolicyOptions{})
	remoteMCP := mcpgateway.NewRemoteMCPAdapter(*gatewayConfig, egressPolicy, sourceSecretCodec)
	descriptors, err := mcpgateway.NewDescriptorRegistry(gatewayConfig.MaxArtifactBytes)
	if err != nil {
		return nil, err
	}
	grpcAdapter := mcpgateway.NewGRPCAdapter(*gatewayConfig, egressPolicy, descriptors, sourceSecretCodec)
	openAPIAdapter := mcpgateway.NewOpenAPIAdapter(*gatewayConfig, egressPolicy, sourceSecretCodec)
	registry := mcpgateway.NewDatabaseRegistry(pool, mcpgateway.DatabaseRegistryOptions{
		Config: *gatewayConfig, ProviderAllowlist: gatewayProviders,
		RemoteMCP: remoteMCP, GRPC: grpcAdapter, OpenAPI: openAPIAdapter, Observer: opts.GatewayObserver,
	})
	gatewayStore := opts.GatewayStore
	if gatewayStore == nil {
		gatewayStore = registry
	}
	gatewayProjector := opts.GatewayProjector
	if gatewayProjector == nil {
		gatewayProjector = registry
	}
	gateway, err := mcpgateway.New(*gatewayConfig, mcpgateway.Dependencies{
		IdentityResolver: mcpgateway.IdentityResolverFunc(func(ctx context.Context) (mcpgateway.Identity, bool) {
			principal, ok := lightweightapi.PrincipalFromContext(ctx)
			if !ok {
				return mcpgateway.Identity{}, false
			}
			return mcpgateway.Identity{
				UserID:       principal.UserID,
				WorkspaceID:  principal.WorkspaceID,
				AgentID:      principal.AgentID,
				TaskID:       principal.TaskID,
				ToolBundleID: principal.ToolBundleID,
			}, true
		}),
		Store:     gatewayStore,
		Projector: gatewayProjector,
		Version:   normalizeServerVersion(version),
		Resources: []mcpgateway.GatewayResource{grpcAdapter},
	})
	if err != nil {
		return nil, err
	}
	toolControl := mcpgateway.NewControlPlane(pool, mcpgateway.ControlPlaneOptions{
		SecretCodec: sourceSecretCodec, MaxArtifactBytes: gatewayConfig.MaxArtifactBytes,
		ValidationWorkers: gatewayConfig.SourceConcurrency, ValidationTimeout: gatewayConfig.InvocationTimeout,
		RemoteMCP: remoteMCP, GRPC: grpcAdapter, OpenAPI: openAPIAdapter, Observer: opts.GatewayObserver,
		// Providers remain fail-closed until unchanged-Daemon E3 evidence is
		// recorded for a code-owned allowlist entry.
		ProviderAllowlist: gatewayProviders,
	})

	router := newLightweightRouter(pool, hub, core, gateway, toolControl, rdb, opts.HTTPMetrics)
	return &LightweightComposition{
		Router:             router,
		Core:               core,
		Queries:            queries,
		HeartbeatScheduler: opts.HeartbeatScheduler,
		Gateway:            gateway,
		ToolControl:        toolControl,
	}, nil
}

type lightweightAuthBoundary uint8

const (
	boundaryPublic lightweightAuthBoundary = iota
	boundaryHuman
	boundaryWorkspace
	boundaryWorkspaceAdmin
	boundaryWorkspaceOwner
	boundaryTaskScoped
	boundaryDaemonRegister
	boundaryDaemon
	boundaryWebSocket
)

type lightweightRoute struct {
	Method      string
	Path        string
	Boundary    lightweightAuthBoundary
	Handler     http.Handler
	Middlewares []func(http.Handler) http.Handler
}

func newLightweightRouter(
	pool *pgxpool.Pool,
	hub *realtime.Hub,
	core *lightweightapi.Handler,
	gateway *mcpgateway.Gateway,
	toolControl *mcpgateway.ControlPlane,
	rdb *redis.Client,
	httpMetrics *obsmetrics.HTTPMetrics,
) chi.Router {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)

	origins := allowedOrigins()
	realtime.SetAllowedOrigins(origins)
	realtime.SetTrustedProxies(parseTrustedProxies(os.Getenv("DARS_TRUSTED_PROXIES")))

	mountLightweightTransportGroups(r, httpMetrics, origins, core.TaskAuth, gateway, func(rest chi.Router) {
		registerLightweightRESTRoutes(rest, pool, hub, rdb, core, toolControl)
	})

	return r
}

func mountLightweightTransportGroups(
	router chi.Router,
	httpMetrics *obsmetrics.HTTPMetrics,
	origins []string,
	taskAuth func(http.Handler) http.Handler,
	gateway http.Handler,
	registerREST func(chi.Router),
) {
	// MCP is intentionally outside the REST error envelope and browser CORS
	// policy so the official transport owns every protocol response unchanged.
	router.Group(func(mcpRouter chi.Router) {
		useLightweightRequestMiddleware(mcpRouter, httpMetrics)
		mountMCPGateway(mcpRouter, taskAuth, gateway)
	})

	// Keep the established REST middleware order byte-for-byte: the error
	// envelope remains outside request metadata, logging, recovery, CSP and CORS.
	router.Group(func(rest chi.Router) {
		rest.Use(lightweightErrorEnvelope)
		useLightweightRequestMiddleware(rest, httpMetrics)
		rest.Use(cors.Handler(cors.Options{
			AllowedOrigins:   origins,
			AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
			AllowedHeaders:   corsAllowedHeaders,
			AllowCredentials: true,
			MaxAge:           300,
		}))
		registerREST(rest)
	})
}

func useLightweightRequestMiddleware(router chi.Router, httpMetrics *obsmetrics.HTTPMetrics) {
	router.Use(middleware.ClientMetadata)
	router.Use(middleware.RequestLogger)
	if httpMetrics != nil {
		router.Use(httpMetrics.Middleware)
	}
	router.Use(chimw.Recoverer)
	router.Use(middleware.ContentSecurityPolicy)
}

func mountMCPGateway(router chi.Router, taskAuth func(http.Handler) http.Handler, gateway http.Handler) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gateway.ServeHTTP(w, mcpgateway.WithBundleID(r, chi.URLParam(r, "bundleId")))
	})
	router.Handle(mcpgateway.BundleRoutePattern, taskAuth(handler))
}

func registerLightweightRESTRoutes(
	router chi.Router,
	pool *pgxpool.Pool,
	hub *realtime.Hub,
	rdb *redis.Client,
	core *lightweightapi.Handler,
	toolControl *mcpgateway.ControlPlane,
) {
	boundaries := map[lightweightAuthBoundary][]func(http.Handler) http.Handler{
		boundaryPublic:         nil,
		boundaryHuman:          {core.HumanAuth},
		boundaryWorkspace:      {core.HumanAuth, core.RequireWorkspaceMember},
		boundaryWorkspaceAdmin: {core.HumanAuth, core.RequireWorkspaceMember, core.RequireWorkspaceRole("owner", "admin")},
		boundaryWorkspaceOwner: {core.HumanAuth, core.RequireWorkspaceMember, core.RequireWorkspaceRole("owner")},
		boundaryTaskScoped:     {core.HumanOrTaskAuth, core.RequireWorkspaceMember},
		boundaryDaemonRegister: {core.HumanOrDaemonAuth},
		boundaryDaemon:         {core.DaemonAuth},
		boundaryWebSocket:      nil,
	}

	routes := lightweightRouteManifest(pool, hub, rdb, core, toolControl)
	registerLightweightPreflightRoutes(router, routes)
	for _, route := range routes {
		chain := route.Handler
		if transport := lightweightTransportForRoute(route); transport != nil {
			chain = transport(chain)
		}
		for i := len(route.Middlewares) - 1; i >= 0; i-- {
			chain = route.Middlewares[i](chain)
		}
		boundaryChain := boundaries[route.Boundary]
		for i := len(boundaryChain) - 1; i >= 0; i-- {
			chain = boundaryChain[i](chain)
		}
		router.Method(route.Method, route.Path, chain)
	}
}

// The CORS middleware is intentionally scoped to the REST group. chi needs an
// explicit OPTIONS match before group middleware can handle browser preflight,
// so register one for every distinct REST path without exposing MCP routes.
func registerLightweightPreflightRoutes(router chi.Router, routes []lightweightRoute) {
	registered := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if _, ok := registered[route.Path]; ok {
			continue
		}
		registered[route.Path] = struct{}{}
		router.Method(http.MethodOptions, route.Path, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
	}
}

// lightweightRouteManifest is the only target route registration manifest.
// Path parameter names intentionally match the frozen public contract.
func lightweightRouteManifest(
	pool *pgxpool.Pool,
	hub *realtime.Hub,
	rdb *redis.Client,
	core *lightweightapi.Handler,
	toolControl *mcpgateway.ControlPlane,
) []lightweightRoute {
	health := newLightweightServerHealth(pool)
	authRL := middleware.RateLimit(rdb, envPositiveInt("RATE_LIMIT_AUTH", 5), time.Minute, middleware.ParseTrustedProxies(os.Getenv("RATE_LIMIT_TRUSTED_PROXIES")))
	authVerifyRL := middleware.RateLimit(rdb, envPositiveInt("RATE_LIMIT_AUTH_VERIFY", 20), time.Minute, middleware.ParseTrustedProxies(os.Getenv("RATE_LIMIT_TRUSTED_PROXIES")))

	webSocketHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		realtime.HandleWebSocket(hub, core, core, w, r)
	})

	route := func(method, path string, boundary lightweightAuthBoundary, fn http.Handler) lightweightRoute {
		return lightweightRoute{Method: method, Path: path, Boundary: boundary, Handler: fn}
	}

	return []lightweightRoute{
		route(http.MethodGet, "/health", boundaryPublic, http.HandlerFunc(health.liveHandler)),
		route(http.MethodGet, "/readyz", boundaryPublic, http.HandlerFunc(health.readyHandler)),
		route(http.MethodGet, "/media/avatars/{avatarId}", boundaryPublic, http.HandlerFunc(core.ServeAvatar)),
		route(http.MethodGet, "/api/config", boundaryPublic, http.HandlerFunc(core.GetConfig)),
		{Method: http.MethodPost, Path: "/auth/send-code", Boundary: boundaryPublic, Handler: http.HandlerFunc(core.SendCode), Middlewares: []func(http.Handler) http.Handler{authRL}},
		{Method: http.MethodPost, Path: "/auth/verify-code", Boundary: boundaryPublic, Handler: http.HandlerFunc(core.VerifyCode), Middlewares: []func(http.Handler) http.Handler{authVerifyRL}},
		route(http.MethodPost, "/auth/logout", boundaryHuman, http.HandlerFunc(core.Logout)),
		route(http.MethodGet, "/ws", boundaryWebSocket, webSocketHandler),

		route(http.MethodGet, "/api/tokens", boundaryHuman, http.HandlerFunc(core.ListPersonalAccessTokens)),
		route(http.MethodPost, "/api/tokens", boundaryHuman, http.HandlerFunc(core.CreatePersonalAccessToken)),
		route(http.MethodPost, "/api/tokens/current/renew", boundaryHuman, http.HandlerFunc(core.RenewCurrentPersonalAccessToken)),
		route(http.MethodDelete, "/api/tokens/{tokenId}", boundaryHuman, http.HandlerFunc(core.RevokePersonalAccessToken)),

		route(http.MethodGet, "/api/me", boundaryHuman, http.HandlerFunc(core.GetMe)),
		route(http.MethodPatch, "/api/me", boundaryHuman, http.HandlerFunc(core.UpdateMe)),
		route(http.MethodGet, "/api/workspaces", boundaryHuman, http.HandlerFunc(core.ListWorkspaces)),
		route(http.MethodPost, "/api/workspaces", boundaryHuman, http.HandlerFunc(core.CreateWorkspace)),
		route(http.MethodGet, "/api/workspaces/{workspaceId}", boundaryWorkspace, http.HandlerFunc(core.GetWorkspace)),
		route(http.MethodPut, "/api/workspaces/{workspaceId}", boundaryWorkspaceAdmin, http.HandlerFunc(core.UpdateWorkspace)),
		route(http.MethodGet, "/api/workspaces/{workspaceId}/daemon-tokens", boundaryWorkspaceAdmin, http.HandlerFunc(core.ListDaemonTokens)),
		route(http.MethodPost, "/api/workspaces/{workspaceId}/daemon-tokens", boundaryWorkspaceAdmin, http.HandlerFunc(core.CreateDaemonToken)),
		route(http.MethodDelete, "/api/workspaces/{workspaceId}/daemon-tokens/{tokenId}", boundaryWorkspaceAdmin, http.HandlerFunc(core.RevokeDaemonToken)),
		route(http.MethodDelete, "/api/workspaces/{workspaceId}", boundaryWorkspaceOwner, http.HandlerFunc(core.DeleteWorkspace)),
		route(http.MethodGet, "/api/workspaces/{workspaceId}/members", boundaryWorkspace, http.HandlerFunc(core.ListWorkspaceMembers)),
		route(http.MethodGet, "/api/workspaces/{workspaceId}/runtime-profiles", boundaryWorkspace, http.HandlerFunc(core.ListRuntimeProfiles)),
		route(http.MethodGet, "/api/workspaces/{workspaceId}/runtime-profiles/{profileId}", boundaryWorkspace, http.HandlerFunc(core.GetRuntimeProfile)),
		route(http.MethodPost, "/api/workspaces/{workspaceId}/runtime-profiles", boundaryWorkspaceAdmin, http.HandlerFunc(core.CreateRuntimeProfile)),
		route(http.MethodPut, "/api/workspaces/{workspaceId}/runtime-profiles/{profileId}", boundaryWorkspaceAdmin, http.HandlerFunc(core.UpdateRuntimeProfile)),
		route(http.MethodDelete, "/api/workspaces/{workspaceId}/runtime-profiles/{profileId}", boundaryWorkspaceAdmin, http.HandlerFunc(core.DeleteRuntimeProfile)),

		route(http.MethodGet, "/api/tool-sources", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.ListSources)),
		route(http.MethodPost, "/api/tool-sources", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.CreateSource)),
		route(http.MethodPost, "/api/tool-sources/import", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.ImportSource)),
		route(http.MethodGet, "/api/tool-sources/{sourceId}", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.GetSource)),
		route(http.MethodPut, "/api/tool-sources/{sourceId}", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.UpdateSource)),
		route(http.MethodDelete, "/api/tool-sources/{sourceId}", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.DeleteSource)),
		route(http.MethodPost, "/api/tool-sources/{sourceId}/validate", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.ValidateSource)),
		route(http.MethodPost, "/api/tool-sources/{sourceId}/enable", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.EnableSource)),
		route(http.MethodPost, "/api/tool-sources/{sourceId}/disable", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.DisableSource)),
		route(http.MethodGet, "/api/tool-sources/{sourceId}/tools", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.ListSourceTools)),
		route(http.MethodPost, "/api/tool-sources/{sourceId}/artifacts", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.UploadSourceArtifact)),
		route(http.MethodGet, "/api/tool-bundles/{bundleId}", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.GetBundle)),
		route(http.MethodPost, "/api/tool-bundles/{bundleId}/revoke", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.RevokeBundle)),
		route(http.MethodGet, "/api/agents/{agentId}/tool-bundle", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.GetAgentBundle)),
		route(http.MethodPut, "/api/agents/{agentId}/tool-bundle", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.PublishAgentBundle)),
		route(http.MethodDelete, "/api/agents/{agentId}/tool-bundle", boundaryWorkspaceAdmin, http.HandlerFunc(toolControl.ClearAgentBundle)),

		route(http.MethodGet, "/api/runtimes", boundaryWorkspace, http.HandlerFunc(core.ListAgentRuntimes)),
		route(http.MethodPatch, "/api/runtimes/{runtimeId}", boundaryWorkspace, http.HandlerFunc(core.UpdateAgentRuntime)),
		route(http.MethodPost, "/api/runtimes/{runtimeId}/models", boundaryWorkspace, http.HandlerFunc(core.InitiateListModels)),
		route(http.MethodGet, "/api/runtimes/{runtimeId}/models/{requestId}", boundaryWorkspace, http.HandlerFunc(core.GetModelListRequest)),
		route(http.MethodPost, "/api/runtimes/{runtimeId}/local-skills", boundaryWorkspace, http.HandlerFunc(core.InitiateListLocalSkills)),
		route(http.MethodGet, "/api/runtimes/{runtimeId}/local-skills/{requestId}", boundaryWorkspace, http.HandlerFunc(core.GetLocalSkillListRequest)),
		route(http.MethodPost, "/api/runtimes/{runtimeId}/local-skills/import", boundaryWorkspace, http.HandlerFunc(core.InitiateImportLocalSkill)),
		route(http.MethodGet, "/api/runtimes/{runtimeId}/local-skills/import/{requestId}", boundaryWorkspace, http.HandlerFunc(core.GetLocalSkillImportRequest)),
		route(http.MethodDelete, "/api/runtimes/{runtimeId}", boundaryWorkspace, http.HandlerFunc(core.DeleteAgentRuntime)),
		route(http.MethodPost, "/api/runtimes/{runtimeId}/unbind-agents-and-delete", boundaryWorkspace, http.HandlerFunc(core.UnbindAgentsAndDeleteRuntime)),

		route(http.MethodGet, "/api/agents", boundaryWorkspace, http.HandlerFunc(core.ListAgents)),
		route(http.MethodGet, "/api/agents/snapshot", boundaryWorkspace, http.HandlerFunc(core.ListAgentSnapshot)),
		route(http.MethodPost, "/api/agents", boundaryWorkspace, http.HandlerFunc(core.CreateAgent)),
		route(http.MethodGet, "/api/agents/{agentId}", boundaryWorkspace, http.HandlerFunc(core.GetAgent)),
		route(http.MethodPut, "/api/agents/{agentId}", boundaryWorkspace, http.HandlerFunc(core.UpdateAgent)),
		route(http.MethodPost, "/api/agents/{agentId}/archive", boundaryWorkspace, http.HandlerFunc(core.ArchiveAgent)),
		route(http.MethodPost, "/api/agents/{agentId}/restore", boundaryWorkspace, http.HandlerFunc(core.RestoreAgent)),
		route(http.MethodGet, "/api/agents/{agentId}/env", boundaryWorkspace, http.HandlerFunc(core.GetAgentEnv)),
		route(http.MethodPut, "/api/agents/{agentId}/env", boundaryWorkspace, http.HandlerFunc(core.UpdateAgentEnv)),
		route(http.MethodGet, "/api/agents/{agentId}/skills", boundaryWorkspace, http.HandlerFunc(core.ListAgentSkills)),
		route(http.MethodPut, "/api/agents/{agentId}/skills", boundaryWorkspace, http.HandlerFunc(core.SetAgentSkills)),
		route(http.MethodGet, "/api/agents/{agentId}/tasks", boundaryWorkspace, http.HandlerFunc(core.ListAgentTasks)),
		route(http.MethodPost, "/api/agents/{agentId}/tasks/cancel", boundaryWorkspace, http.HandlerFunc(core.CancelAgentTasks)),
		route(http.MethodPost, "/api/agents/{agentId}/avatar", boundaryWorkspace, http.HandlerFunc(core.UploadAgentAvatar)),

		route(http.MethodGet, "/api/agent-builder/sessions", boundaryWorkspace, http.HandlerFunc(core.ListAgentBuilderSessions)),
		route(http.MethodPost, "/api/agent-builder/sessions", boundaryWorkspace, http.HandlerFunc(core.CreateAgentBuilderSession)),
		route(http.MethodPatch, "/api/agent-builder/sessions/{sessionId}/runtime", boundaryWorkspace, http.HandlerFunc(core.SwitchAgentBuilderRuntime)),
		route(http.MethodPut, "/api/agent-builder/sessions/{sessionId}/draft", boundaryWorkspace, http.HandlerFunc(core.SaveAgentBuilderDraft)),

		route(http.MethodGet, "/api/skills", boundaryWorkspace, http.HandlerFunc(core.ListSkills)),
		route(http.MethodPost, "/api/skills", boundaryWorkspace, http.HandlerFunc(core.CreateSkill)),
		route(http.MethodPost, "/api/skills/import", boundaryWorkspace, http.HandlerFunc(core.ImportSkill)),
		route(http.MethodGet, "/api/skills/search", boundaryWorkspace, http.HandlerFunc(core.SearchSkills)),
		route(http.MethodGet, "/api/skills/{skillId}", boundaryWorkspace, http.HandlerFunc(core.GetSkill)),
		route(http.MethodPut, "/api/skills/{skillId}", boundaryWorkspace, http.HandlerFunc(core.UpdateSkill)),
		route(http.MethodDelete, "/api/skills/{skillId}", boundaryWorkspace, http.HandlerFunc(core.DeleteSkill)),
		route(http.MethodGet, "/api/skills/{skillId}/files", boundaryWorkspace, http.HandlerFunc(core.ListSkillFiles)),
		route(http.MethodPut, "/api/skills/{skillId}/files", boundaryWorkspace, http.HandlerFunc(core.UpsertSkillFiles)),
		route(http.MethodDelete, "/api/skills/{skillId}/files/{fileId}", boundaryWorkspace, http.HandlerFunc(core.DeleteSkillFile)),

		route(http.MethodGet, "/api/chat/sessions", boundaryWorkspace, http.HandlerFunc(core.ListChatSessions)),
		route(http.MethodPost, "/api/chat/sessions", boundaryWorkspace, http.HandlerFunc(core.CreateChatSession)),
		route(http.MethodGet, "/api/chat/sessions/{sessionId}", boundaryWorkspace, http.HandlerFunc(core.GetChatSession)),
		route(http.MethodPatch, "/api/chat/sessions/{sessionId}", boundaryWorkspace, http.HandlerFunc(core.UpdateChatSession)),
		route(http.MethodDelete, "/api/chat/sessions/{sessionId}", boundaryWorkspace, http.HandlerFunc(core.DeleteChatSession)),
		route(http.MethodGet, "/api/chat/sessions/{sessionId}/messages", boundaryTaskScoped, http.HandlerFunc(core.ListChatMessages)),
		route(http.MethodPost, "/api/chat/sessions/{sessionId}/messages", boundaryWorkspace, http.HandlerFunc(core.SendChatMessage)),
		route(http.MethodGet, "/api/chat/sessions/{sessionId}/pending-task", boundaryWorkspace, http.HandlerFunc(core.GetPendingChatTask)),
		route(http.MethodGet, "/api/chat/sessions/{sessionId}/draft-restores", boundaryWorkspace, http.HandlerFunc(core.ListChatDraftRestores)),
		route(http.MethodDelete, "/api/chat/sessions/{sessionId}/draft-restores/{restoreId}", boundaryWorkspace, http.HandlerFunc(core.ConsumeChatDraftRestore)),

		route(http.MethodGet, "/api/squads", boundaryWorkspace, http.HandlerFunc(core.ListSquads)),
		route(http.MethodPost, "/api/squads", boundaryWorkspace, http.HandlerFunc(core.CreateSquad)),
		route(http.MethodGet, "/api/squads/{squadId}", boundaryWorkspace, http.HandlerFunc(core.GetSquad)),
		route(http.MethodPut, "/api/squads/{squadId}", boundaryWorkspace, http.HandlerFunc(core.UpdateSquad)),
		route(http.MethodDelete, "/api/squads/{squadId}", boundaryWorkspace, http.HandlerFunc(core.DeleteSquad)),
		route(http.MethodPost, "/api/squads/{squadId}/avatar", boundaryWorkspace, http.HandlerFunc(core.UploadSquadAvatar)),
		route(http.MethodGet, "/api/squads/{squadId}/members", boundaryWorkspace, http.HandlerFunc(core.ListSquadMembers)),
		route(http.MethodPost, "/api/squads/{squadId}/members", boundaryWorkspace, http.HandlerFunc(core.AddSquadMember)),
		route(http.MethodDelete, "/api/squads/{squadId}/members", boundaryWorkspace, http.HandlerFunc(core.RemoveSquadMember)),
		route(http.MethodPatch, "/api/squads/{squadId}/members/role", boundaryWorkspace, http.HandlerFunc(core.UpdateSquadMemberRole)),
		route(http.MethodGet, "/api/squads/{squadId}/members/status", boundaryWorkspace, http.HandlerFunc(core.ListSquadMemberStatus)),

		route(http.MethodGet, "/api/issues", boundaryWorkspace, http.HandlerFunc(core.ListIssues)),
		route(http.MethodPost, "/api/issues", boundaryWorkspace, http.HandlerFunc(core.CreateIssue)),
		route(http.MethodGet, "/api/issues/{issueId}", boundaryTaskScoped, http.HandlerFunc(core.GetIssue)),
		route(http.MethodPut, "/api/issues/{issueId}", boundaryTaskScoped, http.HandlerFunc(core.UpdateIssue)),
		route(http.MethodDelete, "/api/issues/{issueId}", boundaryWorkspace, http.HandlerFunc(core.DeleteIssue)),
		route(http.MethodGet, "/api/issues/{issueId}/comments", boundaryTaskScoped, http.HandlerFunc(core.ListComments)),
		route(http.MethodPost, "/api/issues/{issueId}/comments", boundaryTaskScoped, http.HandlerFunc(core.CreateComment)),
		route(http.MethodGet, "/api/issues/{issueId}/task-runs", boundaryTaskScoped, http.HandlerFunc(core.ListTaskRuns)),
		route(http.MethodGet, "/api/issues/{issueId}/active-task", boundaryTaskScoped, http.HandlerFunc(core.GetActiveIssueTasks)),
		route(http.MethodPost, "/api/issues/{issueId}/squad-evaluated", boundaryTaskScoped, http.HandlerFunc(core.RecordSquadLeaderEvaluation)),
		route(http.MethodGet, "/api/tasks/{taskId}/messages", boundaryTaskScoped, http.HandlerFunc(core.ListTaskMessagesForUser)),
		route(http.MethodPost, "/api/tasks/{taskId}/cancel", boundaryWorkspace, http.HandlerFunc(core.CancelTaskByUser)),

		route(http.MethodPost, protocol.DaemonRouteRegister, boundaryDaemonRegister, http.HandlerFunc(core.DaemonRegister)),
		route(http.MethodPost, protocol.DaemonRouteDeregister, boundaryDaemon, http.HandlerFunc(core.DaemonDeregister)),
		route(http.MethodPost, protocol.DaemonRouteHeartbeat, boundaryDaemon, http.HandlerFunc(core.DaemonHeartbeat)),
		route(http.MethodGet, protocol.DaemonRouteWebSocket, boundaryDaemon, http.HandlerFunc(core.DaemonWebSocket)),
		route(http.MethodGet, protocol.DaemonRouteWorkspaces, boundaryDaemon, http.HandlerFunc(core.ListDaemonWorkspaces)),
		route(http.MethodGet, protocol.DaemonRouteWorkspaceRepos, boundaryDaemon, http.HandlerFunc(core.GetDaemonWorkspaceRepos)),
		route(http.MethodGet, protocol.DaemonRouteWorkspaceRuntimeProfiles, boundaryDaemon, http.HandlerFunc(core.DaemonListRuntimeProfiles)),
		route(http.MethodPost, protocol.DaemonRouteTasksClaim, boundaryDaemon, http.HandlerFunc(core.ClaimTasks)),
		route(http.MethodPost, protocol.DaemonRouteTaskPrepareLease, boundaryDaemon, http.HandlerFunc(core.ExtendTaskPrepareLease)),
		route(http.MethodPost, protocol.DaemonRouteTaskSkillBundlesResolve, boundaryDaemon, http.HandlerFunc(core.ResolveTaskSkillBundles)),
		route(http.MethodGet, protocol.DaemonRouteRuntimePendingTasks, boundaryDaemon, http.HandlerFunc(core.ListPendingTasksByRuntime)),
		route(http.MethodPost, protocol.DaemonRouteModelListResult, boundaryDaemon, http.HandlerFunc(core.ReportModelListResult)),
		route(http.MethodPost, protocol.DaemonRouteLocalSkillListResult, boundaryDaemon, http.HandlerFunc(core.ReportLocalSkillListResult)),
		route(http.MethodPost, protocol.DaemonRouteLocalSkillImportResult, boundaryDaemon, http.HandlerFunc(core.ReportLocalSkillImportResult)),
		route(http.MethodGet, protocol.DaemonRouteTaskStatus, boundaryDaemon, http.HandlerFunc(core.GetTaskStatus)),
		route(http.MethodPost, protocol.DaemonRouteTaskStart, boundaryDaemon, http.HandlerFunc(core.StartTask)),
		route(http.MethodPost, protocol.DaemonRouteTaskWaitLocalDirectory, boundaryDaemon, http.HandlerFunc(core.MarkTaskWaitingLocalDirectory)),
		route(http.MethodPost, protocol.DaemonRouteTaskProgress, boundaryDaemon, http.HandlerFunc(core.ReportTaskProgress)),
		route(http.MethodPost, protocol.DaemonRouteTaskMessages, boundaryDaemon, http.HandlerFunc(core.ReportTaskMessages)),
		route(http.MethodGet, protocol.DaemonRouteTaskMessages, boundaryDaemon, http.HandlerFunc(core.ListTaskMessages)),
		route(http.MethodPost, protocol.DaemonRouteTaskUsage, boundaryDaemon, http.HandlerFunc(core.ReportTaskUsage)),
		route(http.MethodPost, protocol.DaemonRouteTaskComplete, boundaryDaemon, http.HandlerFunc(core.CompleteTask)),
		route(http.MethodPost, protocol.DaemonRouteTaskFail, boundaryDaemon, http.HandlerFunc(core.FailTask)),
		route(http.MethodPost, protocol.DaemonRouteTaskCancelAck, boundaryDaemon, http.HandlerFunc(core.AckTaskCancelled)),
		route(http.MethodPost, protocol.DaemonRouteTaskSession, boundaryDaemon, http.HandlerFunc(core.PinTaskSession)),
		route(http.MethodPost, protocol.DaemonRouteWorkspaceIssueGCChecks, boundaryDaemon, http.HandlerFunc(core.BatchIssueGCCheck)),
		route(http.MethodGet, protocol.DaemonRouteIssueGCCheck, boundaryDaemon, http.HandlerFunc(core.GetIssueGCCheck)),
		route(http.MethodGet, protocol.DaemonRouteChatSessionGCCheck, boundaryDaemon, http.HandlerFunc(core.GetChatSessionGCCheck)),
		route(http.MethodGet, protocol.DaemonRouteTaskGCCheck, boundaryDaemon, http.HandlerFunc(core.GetTaskGCCheck)),
		route(http.MethodPost, protocol.DaemonRouteRuntimeRecoverOrphans, boundaryDaemon, http.HandlerFunc(core.RecoverOrphanedTasks)),
	}
}
