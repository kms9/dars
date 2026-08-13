package mcpgateway

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"google.golang.org/protobuf/proto"
)

const unaryFixtureProto = `syntax = "proto3";
package fixture.v1;
message EchoRequest { string message = 1; int64 count = 2; }
message EchoResponse { string echoed = 1; }
service EchoService { rpc Echo(EchoRequest) returns (EchoResponse); }
`

func TestDescriptorRegistryCompilesSourceAndDescriptorSetEquivalently(t *testing.T) {
	registry, err := NewDescriptorRegistry(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	sourceArtifact := descriptorTestArtifact("text/x-proto", []byte(unaryFixtureProto))
	fromSource, err := registry.CompileArtifact(context.Background(), sourceArtifact, json.RawMessage(`{"root_file":"fixture.proto"}`))
	if err != nil {
		t.Fatal(err)
	}
	sourceTools, err := fromSource.Discover("proto-fixture", sourceArtifact.Sha256)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceTools) != 1 || sourceTools[0].UpstreamName != "/fixture.v1.EchoService/Echo" ||
		sourceTools[0].PublicName != "proto-fixture.fixture.v1.EchoService_Echo" {
		t.Fatalf("source tools = %+v", sourceTools)
	}
	if !strings.Contains(string(sourceTools[0].InputSchema), `"message"`) || !strings.Contains(string(sourceTools[0].InputSchema), `"count"`) {
		t.Fatalf("input schema = %s", sourceTools[0].InputSchema)
	}

	set, err := compileProtoSources(context.Background(), map[string]string{"fixture.proto": unaryFixtureProto}, []string{"fixture.proto"})
	if err != nil {
		t.Fatal(err)
	}
	setBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	setArtifact := descriptorTestArtifact("application/x-protobuf-descriptor-set", setBytes)
	fromSet, err := registry.CompileArtifact(context.Background(), setArtifact, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	setTools, err := fromSet.Discover("proto-fixture", setArtifact.Sha256)
	if err != nil {
		t.Fatal(err)
	}
	if len(setTools) != 1 || !bytes.Equal(sourceTools[0].InputSchema, setTools[0].InputSchema) ||
		!bytes.Equal(sourceTools[0].OutputSchema, setTools[0].OutputSchema) || sourceTools[0].UpstreamName != setTools[0].UpstreamName {
		t.Fatalf("source/set tools differ:\n%+v\n%+v", sourceTools, setTools)
	}
}

func TestDescriptorRegistryLoadsBuildTimeCatalogAsImmutableArtifact(t *testing.T) {
	registry, err := NewDescriptorRegistry(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	artifact, ok := registry.EmbeddedArtifact("dars_gateway_v1")
	if !ok || artifact.MediaType != "application/x-protobuf-descriptor-set" || artifact.Sha256 == "" {
		t.Fatalf("embedded artifact = %+v ok=%v", artifact, ok)
	}
	compiled, err := registry.CompileArtifact(context.Background(), artifact, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	tools, err := compiled.Discover("builtin", artifact.Sha256)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].UpstreamName != "/dars.gateway.v1.GatewayUtility/Echo" {
		t.Fatalf("embedded tools = %+v", tools)
	}
	artifact.Content[0] ^= 0xff
	second, _ := registry.EmbeddedArtifact("dars_gateway_v1")
	if bytes.Equal(artifact.Content, second.Content) {
		t.Fatal("caller mutation changed the embedded descriptor artifact")
	}
}

func TestDescriptorRegistryRejectsStreamingMissingImportsAndArchiveExpansion(t *testing.T) {
	registry, err := NewDescriptorRegistry(512)
	if err != nil {
		t.Fatal(err)
	}
	streaming := descriptorTestArtifact("text/x-proto", []byte(`syntax="proto3"; package bad; message M{} service S{ rpc Watch(M) returns (stream M); }`))
	compiled, err := registry.CompileArtifact(context.Background(), streaming, json.RawMessage(`{}`))
	if err == nil {
		_, err = compiled.Discover("bad", streaming.Sha256)
	}
	if err == nil {
		t.Fatal("streaming-only descriptor was accepted")
	}

	missing := descriptorTestArtifact("text/x-proto", []byte(`syntax="proto3"; import "missing.proto"; message M{ Missing value=1; }`))
	if _, err := registry.CompileArtifact(context.Background(), missing, json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing Proto import was accepted")
	}

	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	entry, err := writer.Create("large.proto")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte(strings.Repeat("x", 1024)))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	artifact := descriptorTestArtifact("application/zip", archive.Bytes())
	if _, err := registry.CompileArtifact(context.Background(), artifact, json.RawMessage(`{}`)); err == nil {
		t.Fatal("expanded Proto archive exceeding the bound was accepted")
	}
}

func descriptorTestArtifact(mediaType string, content []byte) lwdb.ToolSourceArtifact {
	digest := sha256.Sum256(content)
	return lwdb.ToolSourceArtifact{
		Sha256: hex.EncodeToString(digest[:]), MediaType: mediaType, SizeBytes: int64(len(content)), Content: content,
	}
}
