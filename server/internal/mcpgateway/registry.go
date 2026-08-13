package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kms9/dars/internal/util"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerLocalHandler is one code-owned operation available to a server_local
// Tool Source. The registry key is captured in an immutable Bundle item and is
// never accepted from tools/call arguments.
type ServerLocalHandler func(context.Context, map[string]any) (any, error)

// DatabaseRegistryOptions configures database-authoritative Bundle projection.
type DatabaseRegistryOptions struct {
	Config            Config
	ProviderAllowlist map[string]bool
	ServerLocal       map[string]ServerLocalHandler
	RemoteMCP         *RemoteMCPAdapter
	GRPC              *GRPCAdapter
	OpenAPI           *OpenAPIAdapter
	Observer          Observer
}

// DatabaseRegistry implements both Bundle authorization and projection. It is
// intentionally stateless across MCP requests apart from bounded concurrency
// counters; every discovery and call consults PostgreSQL live state.
type DatabaseRegistry struct {
	q                 *lwdb.Queries
	config            Config
	providers         map[string]bool
	serverLocal       map[string]ServerLocalHandler
	remoteMCP         *RemoteMCPAdapter
	grpc              *GRPCAdapter
	openAPI           *OpenAPIAdapter
	observer          Observer
	taskConcurrency   *keyedGate
	sourceConcurrency *keyedGate
}

func NewDatabaseRegistry(pool *pgxpool.Pool, opts DatabaseRegistryOptions) *DatabaseRegistry {
	serverLocal := defaultServerLocalHandlers()
	for key, handler := range opts.ServerLocal {
		if handler != nil {
			serverLocal[key] = handler
		}
	}
	return &DatabaseRegistry{
		q:                 lwdb.New(pool),
		config:            opts.Config,
		providers:         copyProviderAllowlist(opts.ProviderAllowlist),
		serverLocal:       serverLocal,
		remoteMCP:         opts.RemoteMCP,
		grpc:              opts.GRPC,
		openAPI:           opts.OpenAPI,
		observer:          observerOrNop(opts.Observer),
		taskConcurrency:   newKeyedGate(opts.Config.TaskConcurrency),
		sourceConcurrency: newKeyedGate(opts.Config.SourceConcurrency),
	}
}

func (registry *DatabaseRegistry) AuthorizeTaskBundle(ctx context.Context, access BundleAccess) error {
	params, err := authorizedBundleParams(access)
	if err != nil {
		return ErrUnauthorized
	}
	if _, err := registry.q.GetAuthorizedTaskBundle(ctx, params); err != nil {
		return ErrUnauthorized
	}
	task, err := registry.q.GetAgentTask(ctx, lwdb.GetAgentTaskParams{ID: params.TaskID, WorkspaceID: params.WorkspaceID})
	if err != nil || task.AgentID != params.AgentID {
		return ErrUnauthorized
	}
	runtime, err := registry.q.GetAgentRuntime(ctx, lwdb.GetAgentRuntimeParams{ID: task.RuntimeID, WorkspaceID: params.WorkspaceID})
	if err != nil || !registry.ProviderSupported(runtime.Provider) {
		return ErrUnauthorized
	}
	return nil
}

func (registry *DatabaseRegistry) ProviderSupported(provider string) bool {
	return registry.providers[strings.ToLower(strings.TrimSpace(provider))]
}

func (registry *DatabaseRegistry) ProjectBundleTools(ctx context.Context, access BundleAccess) ([]ToolRegistration, error) {
	params, err := authorizedBundleParams(access)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if err := registry.AuthorizeTaskBundle(ctx, access); err != nil {
		return nil, ErrUnauthorized
	}
	items, err := registry.q.ListAuthorizedTaskBundleItems(ctx, lwdb.ListAuthorizedTaskBundleItemsParams{
		TaskID: params.TaskID, WorkspaceID: params.WorkspaceID, AgentID: params.AgentID, BundleID: params.BundleID,
	})
	if err != nil {
		return nil, ErrUnauthorized
	}
	registrations := make([]ToolRegistration, 0, len(items))
	for _, item := range items {
		registration, err := registry.registration(access, item)
		if err != nil {
			return nil, ErrUnauthorized
		}
		registrations = append(registrations, registration)
	}
	return registrations, nil
}

type bundleToolSnapshot struct {
	Name          string          `json:"name"`
	CanonicalName string          `json:"canonicalName"`
	Description   string          `json:"description"`
	InputSchema   json.RawMessage `json:"inputSchema"`
	OutputSchema  json.RawMessage `json:"outputSchema"`
}

type bundleInvocationPlan struct {
	Kind         string          `json:"kind"`
	Endpoint     string          `json:"endpoint"`
	UpstreamName string          `json:"upstreamName"`
	Operation    json.RawMessage `json:"operation"`
	Transport    json.RawMessage `json:"transport"`
}

type serverLocalOperation struct {
	RegistryKey string `json:"registry_key"`
}

func (registry *DatabaseRegistry) registration(access BundleAccess, item lwdb.ToolBundleItem) (ToolRegistration, error) {
	var snapshot bundleToolSnapshot
	if err := json.Unmarshal(item.DefinitionSnapshot, &snapshot); err != nil || snapshot.Name != item.PublicName || len(snapshot.InputSchema) == 0 {
		return ToolRegistration{}, errors.New("invalid Bundle tool snapshot")
	}
	if snapshot.CanonicalName == "" {
		snapshot.CanonicalName = snapshot.Name
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(snapshot.InputSchema, &schema); err != nil {
		return ToolRegistration{}, errors.New("invalid Bundle input schema")
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		return ToolRegistration{}, errors.New("invalid Bundle input schema")
	}
	tool := &mcp.Tool{
		Name:        snapshot.Name,
		Description: snapshot.Description,
		InputSchema: cloneRawJSON(snapshot.InputSchema),
	}
	if len(snapshot.OutputSchema) > 0 && string(snapshot.OutputSchema) != "null" {
		tool.OutputSchema = cloneRawJSON(snapshot.OutputSchema)
	}
	return ToolRegistration{
		Tool: tool,
		Handler: func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return registry.call(ctx, access, item.PublicName, resolved, request)
		},
	}, nil
}

func (registry *DatabaseRegistry) call(
	ctx context.Context,
	access BundleAccess,
	publicName string,
	inputSchema *jsonschema.Resolved,
	request *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	params, err := authorizedBundleParams(access)
	if err != nil {
		return nil, ErrUnauthorized
	}
	item, err := registry.q.GetAuthorizedTaskBundleItemByName(ctx, lwdb.GetAuthorizedTaskBundleItemByNameParams{
		TaskID: params.TaskID, WorkspaceID: params.WorkspaceID, AgentID: params.AgentID,
		BundleID: params.BundleID, PublicName: publicName,
	})
	if err != nil {
		return nil, ErrUnauthorized
	}
	startedAt := time.Now()
	outcome := "failed"
	inputBytes := 0
	outputBytes := 0
	defer func() {
		registry.recordInvocationAudit(ctx, access, item, outcome, startedAt, inputBytes, outputBytes)
	}()
	arguments := map[string]any{}
	if request != nil && request.Params != nil && len(request.Params.Arguments) > 0 {
		inputBytes = len(request.Params.Arguments)
		if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
			outcome = "invalid_argument"
			return toolError("invalid tool arguments"), nil
		}
	}
	if err := inputSchema.Validate(arguments); err != nil {
		outcome = "invalid_argument"
		return toolError("invalid tool arguments"), nil
	}

	invocationCtx, cancel := context.WithTimeout(ctx, registry.config.InvocationTimeout)
	defer cancel()
	releaseTask, err := registry.taskConcurrency.acquire(invocationCtx, access.Identity.TaskID)
	if err != nil {
		outcome = "limited_or_cancelled"
		return toolError("tool invocation timed out or was cancelled"), nil
	}
	defer releaseTask()
	releaseSource, err := registry.sourceConcurrency.acquire(invocationCtx, util.UUIDToString(item.SourceID))
	if err != nil {
		outcome = "limited_or_cancelled"
		return toolError("tool invocation timed out or was cancelled"), nil
	}
	defer releaseSource()

	response, err := registry.invoke(invocationCtx, item, arguments)
	if err != nil {
		if invocationCtx.Err() != nil {
			outcome = "cancelled_or_timed_out"
		}
		return toolError("tool invocation failed"), nil
	}
	encoded, err := json.Marshal(response)
	if err != nil || int64(len(encoded)) > registry.config.MaxResponseBytes {
		outcome = "response_too_large"
		return toolError("tool result exceeded the configured limit"), nil
	}
	outcome = "succeeded"
	outputBytes = len(encoded)
	return response, nil
}

func (registry *DatabaseRegistry) recordInvocationAudit(
	requestCtx context.Context,
	access BundleAccess,
	item lwdb.ToolBundleItem,
	outcome string,
	startedAt time.Time,
	inputBytes int,
	outputBytes int,
) {
	sourceKind := invocationPlanKind(item.InvocationPlan)
	registry.observer.RecordInvocation(sourceKind, outcome, time.Since(startedAt), inputBytes, outputBytes)
	workspaceID, workspaceErr := util.ParseUUID(access.Identity.WorkspaceID)
	agentID, agentErr := util.ParseUUID(access.Identity.AgentID)
	taskID, taskErr := util.ParseUUID(access.Identity.TaskID)
	if workspaceErr != nil || agentErr != nil || taskErr != nil {
		return
	}
	task, err := registry.q.GetAgentTask(context.WithoutCancel(requestCtx), lwdb.GetAgentTaskParams{ID: taskID, WorkspaceID: workspaceID})
	if err != nil {
		return
	}
	details, err := json.Marshal(map[string]any{
		"task_id": util.UUIDToString(taskID), "agent_id": util.UUIDToString(agentID), "bundle_id": access.BundleID,
		"bundle_item_id": util.UUIDToString(item.ID), "source_id": util.UUIDToString(item.SourceID),
		"source_revision_id": util.UUIDToString(item.SourceRevisionID), "tool_name": item.PublicName,
		"canonical_tool_name": bundleItemCanonicalName(item), "upstream_tool_name": bundleItemUpstreamName(item),
		"source_kind": sourceKind,
		"outcome":     outcome, "latency_ms": time.Since(startedAt).Milliseconds(),
		"input_bytes": inputBytes, "output_bytes": outputBytes,
	})
	if err != nil {
		return
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), time.Second)
	defer cancel()
	_, _ = registry.q.CreateActivity(auditCtx, lwdb.CreateActivityParams{
		WorkspaceID: workspaceID, IssueID: task.IssueID, ActorType: "agent", ActorID: agentID,
		Action: "mcp_gateway_tool_invoked", Details: details,
	})
}

func bundleItemCanonicalName(item lwdb.ToolBundleItem) string {
	var snapshot bundleToolSnapshot
	if json.Unmarshal(item.DefinitionSnapshot, &snapshot) == nil && strings.TrimSpace(snapshot.CanonicalName) != "" {
		return snapshot.CanonicalName
	}
	return item.PublicName
}

func bundleItemUpstreamName(item lwdb.ToolBundleItem) string {
	var plan bundleInvocationPlan
	if json.Unmarshal(item.InvocationPlan, &plan) == nil {
		return plan.UpstreamName
	}
	return ""
}

func invocationPlanKind(raw json.RawMessage) string {
	var plan struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(raw, &plan) != nil || plan.Kind == "" {
		return "invalid"
	}
	return plan.Kind
}

func (registry *DatabaseRegistry) invoke(ctx context.Context, item lwdb.ToolBundleItem, arguments map[string]any) (*mcp.CallToolResult, error) {
	var plan bundleInvocationPlan
	if err := json.Unmarshal(item.InvocationPlan, &plan); err != nil {
		return nil, errors.New("invalid invocation plan")
	}
	switch plan.Kind {
	case "server_local":
		var operation serverLocalOperation
		if err := json.Unmarshal(plan.Operation, &operation); err != nil || operation.RegistryKey == "" {
			return nil, errors.New("invalid server-local operation")
		}
		handler, ok := registry.serverLocal[operation.RegistryKey]
		if !ok {
			return nil, errors.New("server-local operation unavailable")
		}
		result, err := handler(ctx, arguments)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: compactJSON(result)}},
			StructuredContent: result,
		}, nil
	case "remote_mcp":
		if registry.remoteMCP == nil {
			return nil, errors.New("remote MCP adapter unavailable")
		}
		return registry.remoteMCP.Call(ctx, registry.q, item, plan, arguments)
	case "grpc":
		if registry.grpc == nil {
			return nil, errors.New("gRPC adapter unavailable")
		}
		return registry.grpc.Call(ctx, registry.q, item, plan, arguments)
	case "openapi":
		if registry.openAPI == nil {
			return nil, errors.New("OpenAPI adapter unavailable")
		}
		return registry.openAPI.Call(ctx, registry.q, item, plan, arguments)
	default:
		return nil, errors.New("tool adapter unavailable")
	}
}

func authorizedBundleParams(access BundleAccess) (lwdb.GetAuthorizedTaskBundleParams, error) {
	taskID, err := util.ParseUUID(access.Identity.TaskID)
	if err != nil {
		return lwdb.GetAuthorizedTaskBundleParams{}, err
	}
	workspaceID, err := util.ParseUUID(access.Identity.WorkspaceID)
	if err != nil {
		return lwdb.GetAuthorizedTaskBundleParams{}, err
	}
	agentID, err := util.ParseUUID(access.Identity.AgentID)
	if err != nil || access.BundleID != access.Identity.ToolBundleID || !ValidBundleID(access.BundleID) {
		return lwdb.GetAuthorizedTaskBundleParams{}, ErrUnauthorized
	}
	return lwdb.GetAuthorizedTaskBundleParams{
		TaskID: taskID, WorkspaceID: workspaceID, AgentID: agentID,
		BundleID: pgtype.Text{String: access.BundleID, Valid: true},
	}, nil
}

func defaultServerLocalHandlers() map[string]ServerLocalHandler {
	fixture := func(key string) ServerLocalHandler {
		return func(ctx context.Context, arguments map[string]any) (any, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return map[string]any{"registryKey": key, "arguments": arguments}, nil
		}
	}
	return map[string]ServerLocalHandler{
		"echo":   fixture("echo"),
		"first":  fixture("first"),
		"second": fixture("second"),
		"wait_for_cancel": func(ctx context.Context, _ map[string]any) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
}

func validServerLocalOperation(raw json.RawMessage) bool {
	var operation serverLocalOperation
	if err := json.Unmarshal(raw, &operation); err != nil || operation.RegistryKey == "" {
		return false
	}
	_, ok := defaultServerLocalHandlers()[operation.RegistryKey]
	return ok
}

func copyProviderAllowlist(values map[string]bool) map[string]bool {
	result := make(map[string]bool, len(values))
	for provider, allowed := range values {
		if allowed {
			result[strings.ToLower(strings.TrimSpace(provider))] = true
		}
	}
	return result
}

func cloneRawJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func compactJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func toolError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}

type keyedGate struct {
	limit int
	mu    sync.Mutex
	keys  map[string]*gateEntry
}

type gateEntry struct {
	semaphore  chan struct{}
	references int
}

func newKeyedGate(limit int) *keyedGate {
	return &keyedGate{limit: limit, keys: make(map[string]*gateEntry)}
}

func (gate *keyedGate) acquire(ctx context.Context, key string) (func(), error) {
	if gate.limit <= 0 {
		return nil, fmt.Errorf("invalid concurrency limit")
	}
	gate.mu.Lock()
	entry := gate.keys[key]
	if entry == nil {
		entry = &gateEntry{semaphore: make(chan struct{}, gate.limit)}
		gate.keys[key] = entry
	}
	entry.references++
	gate.mu.Unlock()

	select {
	case entry.semaphore <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() {
				<-entry.semaphore
				gate.releaseReference(key, entry)
			})
		}, nil
	case <-ctx.Done():
		gate.releaseReference(key, entry)
		return nil, ctx.Err()
	}
}

func (gate *keyedGate) releaseReference(key string, entry *gateEntry) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	entry.references--
	if entry.references == 0 {
		delete(gate.keys, key)
	}
}
