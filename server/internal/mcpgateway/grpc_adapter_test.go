package mcpgateway

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type dynamicEchoService interface{}

func TestDynamicGRPCInvokerUsesControlledEgressMetadataAndNoRetry(t *testing.T) {
	privateAddress, ok := privateInterfaceAddress()
	if !ok {
		t.Skip("no private non-loopback interface is available")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		t.Skipf("private interface cannot host a fixture: %v", err)
	}

	registry, err := NewDescriptorRegistry(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	artifact := descriptorTestArtifact("text/x-proto", []byte(unaryFixtureProto))
	compiled, err := registry.CompileArtifact(context.Background(), artifact, []byte(`{"root_file":"fixture.proto"}`))
	if err != nil {
		t.Fatal(err)
	}
	method := compiled.methods["/fixture.v1.EchoService/Echo"]
	if method == nil {
		t.Fatal("fixture method is missing")
	}
	var calls atomic.Int32
	var cancellationObserved atomic.Bool
	grpcServer := grpc.NewServer()
	grpcServer.RegisterService(&grpc.ServiceDesc{
		ServiceName: "fixture.v1.EchoService", HandlerType: (*dynamicEchoService)(nil),
		Methods: []grpc.MethodDesc{{MethodName: "Echo", Handler: dynamicEchoHandler(method, &calls, &cancellationObserved)}},
	}, struct{}{})
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)

	port := uint16(listener.Addr().(*net.TCPAddr).Port)
	resolver := &staticResolver{answers: map[string][][]netip.Addr{"grpc-fixture.internal": {{privateAddress}}}}
	config := Config{
		AllowedPorts: []uint16{port}, PrivateCIDRAllowlist: []netip.Prefix{netip.PrefixFrom(privateAddress, privateAddress.BitLen())},
		ConnectTimeout: time.Second, InvocationTimeout: 2 * time.Second,
		ChannelIdleTimeout:  25 * time.Millisecond,
		MaxRequestBodyBytes: 1 << 20, MaxResponseBytes: 1 << 20,
	}
	adapter := NewGRPCAdapter(config, NewEgressPolicy(config, EgressPolicyOptions{Resolver: resolver}), registry, nil)
	t.Cleanup(func() { _ = adapter.Close() })
	sourceID := newPGUUID()
	revisionID := newPGUUID()
	item := lwdb.ToolBundleItem{SourceID: sourceID, SourceRevisionID: revisionID}
	revision := lwdb.ToolSourceRevision{
		ID: revisionID, SourceID: sourceID,
		Endpoint: pgtype.Text{String: "grpc://grpc-fixture.internal:" + strconv.Itoa(int(port)), Valid: true},
	}
	conn, release, err := adapter.acquireConnection(context.Background(), item, revision)
	if err != nil {
		t.Fatal(err)
	}
	result, err := invokeDynamicGRPC(context.Background(), conn, method, "/fixture.v1.EchoService/Echo",
		map[string]any{"message": "hello", "count": "7"}, metadata.Pairs("authorization", "Bearer sentinel"), config)
	if err != nil || result.IsError {
		t.Fatalf("dynamic call result=%+v err=%v", result, err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["echoed"] != "hello:7" {
		t.Fatalf("structured response = %#v", result.StructuredContent)
	}
	beforeInvalid := calls.Load()
	if _, err := invokeDynamicGRPC(context.Background(), conn, method, "/fixture.v1.EchoService/Echo",
		map[string]any{"unknown": true}, metadata.Pairs("authorization", "Bearer sentinel"), config); err == nil {
		t.Fatal("unknown ProtoJSON field was accepted")
	}
	if calls.Load() != beforeInvalid {
		t.Fatal("invalid ProtoJSON reached the upstream service")
	}
	smallResponseConfig := config
	smallResponseConfig.MaxResponseBytes = 32
	if _, err := invokeDynamicGRPC(context.Background(), conn, method, "/fixture.v1.EchoService/Echo",
		map[string]any{"message": strings.Repeat("x", 128)}, metadata.Pairs("authorization", "Bearer sentinel"), smallResponseConfig); err == nil {
		t.Fatal("oversized dynamic gRPC response was accepted")
	}

	beforeFailure := calls.Load()
	if _, err := invokeDynamicGRPC(context.Background(), conn, method, "/fixture.v1.EchoService/Echo",
		map[string]any{"message": "fail"}, metadata.Pairs("authorization", "Bearer sentinel"), config); err == nil {
		t.Fatal("upstream failure was accepted")
	}
	if calls.Load()-beforeFailure != 1 {
		t.Fatalf("non-idempotent failing call count = %d, want 1", calls.Load()-beforeFailure)
	}

	cancelCtx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	_, _ = invokeDynamicGRPC(cancelCtx, conn, method, "/fixture.v1.EchoService/Echo",
		map[string]any{"message": "wait"}, metadata.Pairs("authorization", "Bearer sentinel"), config)
	deadline := time.Now().Add(time.Second)
	for !cancellationObserved.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !cancellationObserved.Load() {
		t.Fatal("gRPC cancellation did not reach the upstream handler")
	}

	release()
	reused, releaseReused, err := adapter.acquireConnection(context.Background(), item, revision)
	if err != nil || reused != conn {
		t.Fatalf("channel was not reused: reused=%p original=%p err=%v", reused, conn, err)
	}
	releaseReused()
	time.Sleep(75 * time.Millisecond)
	replaced, releaseReplaced, err := adapter.acquireConnection(context.Background(), item, revision)
	if err != nil || replaced == conn {
		t.Fatalf("idle channel was not retired: replaced=%p original=%p err=%v", replaced, conn, err)
	}
	releaseReplaced()
	adapter.InvalidateSource(uuidString(sourceID))
	afterInvalidation, releaseAfterInvalidation, err := adapter.acquireConnection(context.Background(), item, revision)
	if err != nil || afterInvalidation == replaced {
		t.Fatalf("invalidated channel was not replaced: next=%p previous=%p err=%v", afterInvalidation, replaced, err)
	}
	releaseAfterInvalidation()

	tlsRevision := revision
	tlsRevision.ID = newPGUUID()
	tlsRevision.Endpoint = pgtype.Text{String: "grpcs://grpc-fixture.internal:" + strconv.Itoa(int(port)), Valid: true}
	tlsItem := item
	tlsItem.SourceRevisionID = tlsRevision.ID
	tlsConn, tlsRelease, err := adapter.acquireConnection(context.Background(), tlsItem, tlsRevision)
	if err != nil {
		t.Fatal(err)
	}
	tlsCtx, tlsCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer tlsCancel()
	if _, err := invokeDynamicGRPC(tlsCtx, tlsConn, method, "/fixture.v1.EchoService/Echo",
		map[string]any{"message": "tls"}, metadata.Pairs("authorization", "Bearer sentinel"), config); err == nil {
		t.Fatal("gRPC TLS mismatch against plaintext fixture was accepted")
	}
	tlsRelease()

	unavailableListener, err := net.Listen("tcp4", net.JoinHostPort(privateAddress.String(), "0"))
	if err != nil {
		t.Fatal(err)
	}
	unavailablePort := uint16(unavailableListener.Addr().(*net.TCPAddr).Port)
	_ = unavailableListener.Close()
	unavailableConfig := config
	unavailableConfig.AllowedPorts = append(append([]uint16(nil), config.AllowedPorts...), unavailablePort)
	unavailableAdapter := NewGRPCAdapter(unavailableConfig,
		NewEgressPolicy(unavailableConfig, EgressPolicyOptions{Resolver: resolver}), registry, nil)
	t.Cleanup(func() { _ = unavailableAdapter.Close() })
	unavailableRevision := revision
	unavailableRevision.ID = newPGUUID()
	unavailableRevision.Endpoint = pgtype.Text{String: "grpc://grpc-fixture.internal:" + strconv.Itoa(int(unavailablePort)), Valid: true}
	unavailableItem := item
	unavailableItem.SourceRevisionID = unavailableRevision.ID
	unavailableConn, unavailableRelease, err := unavailableAdapter.acquireConnection(context.Background(), unavailableItem, unavailableRevision)
	if err != nil {
		t.Fatal(err)
	}
	unavailableCtx, unavailableCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer unavailableCancel()
	if _, err := invokeDynamicGRPC(unavailableCtx, unavailableConn, method, "/fixture.v1.EchoService/Echo",
		map[string]any{"message": "unavailable"}, metadata.Pairs("authorization", "Bearer sentinel"), unavailableConfig); err == nil {
		t.Fatal("unavailable gRPC endpoint was accepted")
	}
	unavailableRelease()
}

func dynamicEchoHandler(method protoreflect.MethodDescriptor, calls *atomic.Int32, cancellationObserved *atomic.Bool) grpc.MethodHandler {
	return func(
		srv any,
		ctx context.Context,
		decode func(any) error,
		interceptor grpc.UnaryServerInterceptor,
	) (any, error) {
		request := dynamicpb.NewMessage(method.Input())
		if err := decode(request); err != nil {
			return nil, err
		}
		handler := func(ctx context.Context, raw any) (any, error) {
			calls.Add(1)
			incoming, _ := metadata.FromIncomingContext(ctx)
			if values := incoming.Get("authorization"); len(values) != 1 || values[0] != "Bearer sentinel" {
				return nil, status.Error(codes.Unauthenticated, "missing fixture metadata")
			}
			message := raw.(*dynamicpb.Message)
			messageField := method.Input().Fields().ByName("message")
			countField := method.Input().Fields().ByName("count")
			value := message.Get(messageField).String()
			switch value {
			case "fail":
				return nil, status.Error(codes.Unavailable, "fixture failure")
			case "wait":
				<-ctx.Done()
				cancellationObserved.Store(true)
				return nil, ctx.Err()
			}
			response := dynamicpb.NewMessage(method.Output())
			response.Set(method.Output().Fields().ByName("echoed"), protoreflect.ValueOfString(value+":"+strconv.FormatInt(message.Get(countField).Int(), 10)))
			return response, nil
		}
		if interceptor == nil {
			return handler(ctx, request)
		}
		return interceptor(ctx, request, &grpc.UnaryServerInfo{Server: srv, FullMethod: "/fixture.v1.EchoService/Echo"}, handler)
	}
}
