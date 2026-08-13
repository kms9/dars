package mcpgateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/kms9/dars/internal/util"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type grpcInvocationOperation struct {
	FullMethod     string `json:"full_method"`
	InputType      string `json:"input_type"`
	OutputType     string `json:"output_type"`
	ArtifactSHA256 string `json:"artifact_sha256"`
}

type grpcChannelEntry struct {
	conn       *grpc.ClientConn
	sourceID   string
	revisionID string
	references int
	idleTimer  *time.Timer
}

// GRPCAdapter owns immutable descriptor caches and replica-local channels.
// Authorization remains in DatabaseRegistry and is never inferred from cache.
type GRPCAdapter struct {
	config      Config
	egress      *EgressPolicy
	descriptors *DescriptorRegistry
	secretCodec *SourceSecretCodec
	mu          sync.Mutex
	channels    map[string]*grpcChannelEntry
	closed      bool
}

func NewGRPCAdapter(config Config, egress *EgressPolicy, descriptors *DescriptorRegistry, secretCodec *SourceSecretCodec) *GRPCAdapter {
	return &GRPCAdapter{
		config: config, egress: egress, descriptors: descriptors, secretCodec: secretCodec,
		channels: make(map[string]*grpcChannelEntry),
	}
}

func (adapter *GRPCAdapter) EmbeddedArtifact(name string) (lwdb.ToolSourceArtifact, bool) {
	if adapter == nil || adapter.descriptors == nil {
		return lwdb.ToolSourceArtifact{}, false
	}
	return adapter.descriptors.EmbeddedArtifact(name)
}

func (adapter *GRPCAdapter) Discover(
	ctx context.Context,
	q *lwdb.Queries,
	source lwdb.ToolSource,
	revision lwdb.ToolSourceRevision,
) ([]discoveredTool, error) {
	if adapter == nil || adapter.egress == nil || adapter.descriptors == nil || q == nil || source.Kind != "grpc" ||
		!revision.Endpoint.Valid || !revision.ArtifactID.Valid {
		return nil, errors.New("gRPC Source is invalid")
	}
	if _, err := adapter.egress.ValidateGRPCEndpoint(ctx, revision.Endpoint.String); err != nil {
		return nil, err
	}
	if _, err := grpcRevisionMetadata(ctx, q, revision, adapter.secretCodec); err != nil {
		return nil, err
	}
	artifact, err := q.GetToolSourceArtifact(ctx, lwdb.GetToolSourceArtifactParams{
		ID: revision.ArtifactID, WorkspaceID: revision.WorkspaceID, SourceID: revision.SourceID,
	})
	if err != nil {
		return nil, errors.New("gRPC descriptor artifact is unavailable")
	}
	compiled, err := adapter.descriptors.CompileArtifact(ctx, artifact, revision.TransportConfig)
	if err != nil {
		return nil, err
	}
	return compiled.Discover(source.Name, artifact.Sha256)
}

func (adapter *GRPCAdapter) Call(
	ctx context.Context,
	q *lwdb.Queries,
	item lwdb.ToolBundleItem,
	plan bundleInvocationPlan,
	arguments map[string]any,
) (*mcp.CallToolResult, error) {
	if adapter == nil || q == nil || plan.Kind != "grpc" || plan.Endpoint == "" || !item.ArtifactID.Valid {
		return nil, errors.New("gRPC invocation plan is invalid")
	}
	revision, err := q.GetToolSourceRevision(ctx, lwdb.GetToolSourceRevisionParams{
		ID: item.SourceRevisionID, WorkspaceID: item.WorkspaceID, SourceID: item.SourceID,
	})
	if err != nil || (revision.Status != "ready" && revision.Status != "retired") || !revision.Endpoint.Valid ||
		revision.Endpoint.String != plan.Endpoint || revision.ArtifactID != item.ArtifactID {
		return nil, errors.New("gRPC revision is unavailable")
	}
	artifact, err := q.GetToolSourceArtifact(ctx, lwdb.GetToolSourceArtifactParams{
		ID: item.ArtifactID, WorkspaceID: item.WorkspaceID, SourceID: item.SourceID,
	})
	if err != nil {
		return nil, errors.New("gRPC descriptor artifact is unavailable")
	}
	compiled, err := adapter.descriptors.CompileArtifact(ctx, artifact, revision.TransportConfig)
	if err != nil {
		return nil, err
	}
	var operation grpcInvocationOperation
	if err := json.Unmarshal(plan.Operation, &operation); err != nil || operation.FullMethod == "" ||
		operation.FullMethod != plan.UpstreamName || operation.ArtifactSHA256 != artifact.Sha256 {
		return nil, errors.New("gRPC invocation operation is invalid")
	}
	method := compiled.methods[operation.FullMethod]
	if method == nil || string(method.Input().FullName()) != operation.InputType || string(method.Output().FullName()) != operation.OutputType {
		return nil, errors.New("gRPC method descriptor mismatch")
	}
	md, err := grpcRevisionMetadata(ctx, q, revision, adapter.secretCodec)
	if err != nil {
		return nil, err
	}
	conn, release, err := adapter.acquireConnection(ctx, item, revision)
	if err != nil {
		return nil, err
	}
	defer release()
	return invokeDynamicGRPC(ctx, conn, method, operation.FullMethod, arguments, md, adapter.config)
}

func invokeDynamicGRPC(
	ctx context.Context,
	conn grpc.ClientConnInterface,
	method protoreflect.MethodDescriptor,
	fullMethod string,
	arguments map[string]any,
	md metadata.MD,
	config Config,
) (*mcp.CallToolResult, error) {
	if conn == nil || method == nil || fullMethod == "" {
		return nil, errors.New("gRPC invocation is invalid")
	}
	inputJSON, err := json.Marshal(arguments)
	if err != nil || int64(len(inputJSON)) > config.MaxRequestBodyBytes {
		return nil, errors.New("gRPC request is invalid")
	}
	request := dynamicpb.NewMessage(method.Input())
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(inputJSON, request); err != nil {
		return nil, errors.New("gRPC request is invalid")
	}
	response := dynamicpb.NewMessage(method.Output())
	callCtx := metadata.NewOutgoingContext(ctx, md)
	if err := conn.Invoke(callCtx, fullMethod, request, response, grpc.WaitForReady(false)); err != nil {
		return nil, errors.Join(errors.New("gRPC tool call failed"), err)
	}
	encoded, err := (protojson.MarshalOptions{UseProtoNames: false}).Marshal(response)
	if err != nil || int64(len(encoded)) > config.MaxResponseBytes {
		return nil, errors.New("gRPC response is invalid or too large")
	}
	var structured any
	if err := json.Unmarshal(encoded, &structured); err != nil {
		return nil, errors.New("gRPC response is invalid")
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}, StructuredContent: structured,
	}, nil
}

func (adapter *GRPCAdapter) acquireConnection(ctx context.Context, item lwdb.ToolBundleItem, revision lwdb.ToolSourceRevision) (*grpc.ClientConn, func(), error) {
	endpoint, err := adapter.egress.ValidateGRPCEndpoint(ctx, revision.Endpoint.String)
	if err != nil {
		return nil, nil, err
	}
	key := util.UUIDToString(revision.ID) + "|" + endpoint.Target + "|" + util.UUIDToString(revision.SecretID)
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	if adapter.closed {
		return nil, nil, errors.New("gRPC adapter is closed")
	}
	if entry, ok := adapter.channels[key]; ok {
		if entry.idleTimer != nil {
			entry.idleTimer.Stop()
			entry.idleTimer = nil
		}
		entry.references++
		return entry.conn, adapter.releaseConnection(key, entry), nil
	}
	var transportCredentials credentials.TransportCredentials = insecure.NewCredentials()
	if endpoint.TLS {
		transportCredentials = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: endpoint.ServerName})
	}
	// The egress dialer is the connect-time DNS authority. The passthrough
	// resolver prevents gRPC from independently resolving the hostname before
	// that dialer can re-resolve, approve, and pin the actual destination IP.
	conn, err := grpc.NewClient("passthrough:///"+endpoint.Target,
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return adapter.egress.DialContext(ctx, "tcp", endpoint.Target)
		}),
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithNoProxy(), grpc.WithDisableServiceConfig(), grpc.WithDisableRetry(),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallSendMsgSize(int(adapter.config.MaxRequestBodyBytes)),
			grpc.MaxCallRecvMsgSize(int(adapter.config.MaxResponseBytes)),
		),
	)
	if err != nil {
		return nil, nil, errors.New("gRPC channel creation failed")
	}
	entry := &grpcChannelEntry{
		conn: conn, sourceID: util.UUIDToString(item.SourceID), revisionID: util.UUIDToString(item.SourceRevisionID), references: 1,
	}
	adapter.channels[key] = entry
	return conn, adapter.releaseConnection(key, entry), nil
}

func (adapter *GRPCAdapter) releaseConnection(key string, entry *grpcChannelEntry) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			adapter.mu.Lock()
			defer adapter.mu.Unlock()
			if adapter.channels[key] != entry || entry.references <= 0 {
				return
			}
			entry.references--
			if entry.references == 0 {
				idleTimeout := adapter.config.ChannelIdleTimeout
				if idleTimeout <= 0 {
					idleTimeout = defaultChannelIdleTimeout
				}
				entry.idleTimer = time.AfterFunc(idleTimeout, func() { adapter.retireIdleConnection(key, entry) })
			}
		})
	}
}

func (adapter *GRPCAdapter) retireIdleConnection(key string, entry *grpcChannelEntry) {
	adapter.mu.Lock()
	if adapter.channels[key] != entry || entry.references != 0 || adapter.closed {
		adapter.mu.Unlock()
		return
	}
	delete(adapter.channels, key)
	entry.idleTimer = nil
	adapter.mu.Unlock()
	_ = entry.conn.Close()
}

func grpcRevisionMetadata(ctx context.Context, q *lwdb.Queries, revision lwdb.ToolSourceRevision, codec *SourceSecretCodec) (metadata.MD, error) {
	headers, err := sourceRevisionHeaders(ctx, q, revision, codec)
	if err != nil {
		return nil, errors.New("gRPC credential is invalid")
	}
	md := metadata.MD{}
	for name, values := range headers {
		key := strings.ToLower(name)
		if strings.HasPrefix(key, "grpc-") || key == "te" || key == "content-type" || key == "user-agent" {
			return nil, errors.New("gRPC credential metadata is invalid")
		}
		for _, value := range values {
			md.Append(key, value)
		}
	}
	return md, nil
}

func (adapter *GRPCAdapter) InvalidateSource(sourceID string) {
	adapter.mu.Lock()
	var connections []*grpc.ClientConn
	for key, entry := range adapter.channels {
		if entry.sourceID == sourceID {
			if entry.idleTimer != nil {
				entry.idleTimer.Stop()
			}
			connections = append(connections, entry.conn)
			delete(adapter.channels, key)
		}
	}
	adapter.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

func (adapter *GRPCAdapter) Close() error {
	adapter.mu.Lock()
	if adapter.closed {
		adapter.mu.Unlock()
		return nil
	}
	adapter.closed = true
	connections := make([]*grpc.ClientConn, 0, len(adapter.channels))
	for key, entry := range adapter.channels {
		if entry.idleTimer != nil {
			entry.idleTimer.Stop()
		}
		connections = append(connections, entry.conn)
		delete(adapter.channels, key)
	}
	adapter.mu.Unlock()
	var result error
	for _, conn := range connections {
		result = errors.Join(result, conn.Close())
	}
	return result
}
