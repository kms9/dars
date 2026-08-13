package mcpgateway

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	lwdb "github.com/kms9/dars/pkg/lightweightdb"
)

//go:embed builtin/*.proto
var embeddedProtoSources embed.FS

const (
	maxDescriptorFiles       = 64
	maxDescriptorMethods     = 256
	maxDescriptorFields      = 2048
	maxDescriptorSchemaDepth = 16
)

type compiledDescriptors struct {
	digest  string
	files   *protoregistry.Files
	methods map[string]protoreflect.MethodDescriptor
}

// DescriptorRegistry normalizes all embedded and uploaded Proto inputs into
// one immutable descriptor representation cached only by content digest.
type DescriptorRegistry struct {
	maxBytes int64
	mu       sync.RWMutex
	cache    map[string]*compiledDescriptors
	embedded map[string]lwdb.ToolSourceArtifact
}

func NewDescriptorRegistry(maxBytes int64) (*DescriptorRegistry, error) {
	if maxBytes <= 0 {
		return nil, errors.New("descriptor registry byte limit must be positive")
	}
	registry := &DescriptorRegistry{
		maxBytes: maxBytes, cache: make(map[string]*compiledDescriptors), embedded: make(map[string]lwdb.ToolSourceArtifact),
	}
	entries, err := embeddedProtoSources.ReadDir("builtin")
	if err != nil {
		return nil, errors.New("read embedded Proto catalog")
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".proto") {
			continue
		}
		content, err := embeddedProtoSources.ReadFile("builtin/" + entry.Name())
		if err != nil || int64(len(content)) > maxBytes {
			return nil, errors.New("read embedded Proto descriptor")
		}
		root := "builtin/" + entry.Name()
		set, err := compileProtoSources(context.Background(), map[string]string{root: string(content)}, []string{root})
		if err != nil {
			return nil, errors.New("compile embedded Proto descriptor")
		}
		encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(set)
		if err != nil || int64(len(encoded)) > maxBytes {
			return nil, errors.New("normalize embedded Proto descriptor")
		}
		digest := sha256.Sum256(encoded)
		name := strings.TrimSuffix(entry.Name(), ".proto")
		registry.embedded[name] = lwdb.ToolSourceArtifact{
			Sha256: hex.EncodeToString(digest[:]), MediaType: "application/x-protobuf-descriptor-set",
			SizeBytes: int64(len(encoded)), Content: encoded,
		}
	}
	if len(registry.embedded) == 0 {
		return nil, errors.New("embedded Proto catalog is empty")
	}
	return registry, nil
}

func (registry *DescriptorRegistry) EmbeddedArtifact(name string) (lwdb.ToolSourceArtifact, bool) {
	if registry == nil {
		return lwdb.ToolSourceArtifact{}, false
	}
	registry.mu.RLock()
	artifact, ok := registry.embedded[strings.TrimSpace(name)]
	registry.mu.RUnlock()
	if !ok {
		return lwdb.ToolSourceArtifact{}, false
	}
	artifact.Content = append([]byte(nil), artifact.Content...)
	return artifact, true
}

func (registry *DescriptorRegistry) CompileArtifact(ctx context.Context, artifact lwdb.ToolSourceArtifact, transport json.RawMessage) (*compiledDescriptors, error) {
	if registry == nil || registry.maxBytes <= 0 || artifact.SizeBytes <= 0 || artifact.SizeBytes > registry.maxBytes ||
		int64(len(artifact.Content)) != artifact.SizeBytes {
		return nil, errors.New("Proto artifact is invalid")
	}
	digest := sha256.Sum256(artifact.Content)
	digestText := hex.EncodeToString(digest[:])
	if digestText != artifact.Sha256 {
		return nil, errors.New("Proto artifact digest mismatch")
	}
	registry.mu.RLock()
	cached := registry.cache[digestText]
	registry.mu.RUnlock()
	if cached != nil {
		return cached, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	set, err := registry.compileContent(ctx, artifact.MediaType, artifact.Content, transport)
	if err != nil {
		return nil, errors.New("Proto artifact validation failed")
	}
	compiled, err := normalizeDescriptorSet(set, digestText)
	if err != nil {
		return nil, errors.New("Proto descriptor validation failed")
	}
	registry.mu.Lock()
	if existing := registry.cache[digestText]; existing != nil {
		compiled = existing
	} else {
		registry.cache[digestText] = compiled
	}
	registry.mu.Unlock()
	return compiled, nil
}

func (registry *DescriptorRegistry) compileContent(ctx context.Context, mediaType string, content []byte, transport json.RawMessage) (*descriptorpb.FileDescriptorSet, error) {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mediaType, ";")[0])) {
	case "application/x-protobuf-descriptor-set", "application/protobuf", "application/octet-stream":
		var set descriptorpb.FileDescriptorSet
		if err := proto.Unmarshal(content, &set); err != nil || len(set.File) == 0 {
			return nil, errors.New("invalid descriptor set")
		}
		return &set, nil
	case "text/x-proto", "application/x-protobuf-source", "text/plain":
		name := "source.proto"
		var options struct {
			RootFile string `json:"root_file"`
		}
		if len(transport) > 0 && json.Unmarshal(transport, &options) == nil && validProtoPath(options.RootFile) {
			name = options.RootFile
		}
		return compileProtoSources(ctx, map[string]string{name: string(content)}, []string{name})
	case "application/zip", "application/x-protobuf-archive":
		sources, err := readProtoArchive(content, registry.maxBytes)
		if err != nil {
			return nil, err
		}
		roots := make([]string, 0, len(sources))
		for name := range sources {
			roots = append(roots, name)
		}
		slices.Sort(roots)
		return compileProtoSources(ctx, sources, roots)
	default:
		return nil, errors.New("unsupported Proto artifact media type")
	}
}

func readProtoArchive(content []byte, maxBytes int64) (map[string]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil || len(reader.File) == 0 || len(reader.File) > maxDescriptorFiles {
		return nil, errors.New("invalid Proto archive")
	}
	sources := make(map[string]string, len(reader.File))
	var expanded int64
	for _, file := range reader.File {
		name := path.Clean(file.Name)
		if file.FileInfo().IsDir() || !validProtoPath(name) || file.UncompressedSize64 > uint64(maxBytes) {
			return nil, errors.New("invalid Proto archive entry")
		}
		expanded += int64(file.UncompressedSize64)
		if expanded > maxBytes {
			return nil, errors.New("Proto archive expansion limit exceeded")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(io.LimitReader(stream, maxBytes+1))
		_ = stream.Close()
		if readErr != nil || int64(len(body)) > maxBytes || strings.TrimSpace(string(body)) == "" {
			return nil, errors.New("invalid Proto archive entry")
		}
		if _, duplicate := sources[name]; duplicate {
			return nil, errors.New("duplicate Proto archive path")
		}
		sources[name] = string(body)
	}
	return sources, nil
}

func validProtoPath(name string) bool {
	return name != "" && name == path.Clean(name) && !strings.HasPrefix(name, "/") && !strings.HasPrefix(name, "../") &&
		!strings.Contains(name, "\\") && strings.HasSuffix(strings.ToLower(name), ".proto")
}

func compileProtoSources(ctx context.Context, sources map[string]string, roots []string) (*descriptorpb.FileDescriptorSet, error) {
	if len(sources) == 0 || len(sources) > maxDescriptorFiles || len(roots) == 0 {
		return nil, errors.New("invalid Proto source set")
	}
	compiler := protocompile.Compiler{
		Resolver:       protocompile.WithStandardImports(&protocompile.SourceResolver{Accessor: protocompile.SourceAccessorFromMap(sources)}),
		MaxParallelism: 4,
	}
	files, err := compiler.Compile(ctx, roots...)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]*descriptorpb.FileDescriptorProto)
	var collect func(protoreflect.FileDescriptor)
	collect = func(file protoreflect.FileDescriptor) {
		if file == nil {
			return
		}
		name := file.Path()
		if _, exists := byName[name]; exists {
			return
		}
		byName[name] = protodesc.ToFileDescriptorProto(file)
		imports := file.Imports()
		for index := 0; index < imports.Len(); index++ {
			collect(imports.Get(index).FileDescriptor)
		}
	}
	for _, file := range files {
		collect(file)
	}
	if len(byName) == 0 || len(byName) > maxDescriptorFiles {
		return nil, errors.New("compiled Proto file count exceeded")
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	slices.Sort(names)
	set := &descriptorpb.FileDescriptorSet{File: make([]*descriptorpb.FileDescriptorProto, 0, len(names))}
	for _, name := range names {
		set.File = append(set.File, byName[name])
	}
	return set, nil
}

func normalizeDescriptorSet(set *descriptorpb.FileDescriptorSet, digest string) (*compiledDescriptors, error) {
	if set == nil || len(set.File) == 0 || len(set.File) > maxDescriptorFiles {
		return nil, errors.New("descriptor file count is invalid")
	}
	slices.SortFunc(set.File, func(left, right *descriptorpb.FileDescriptorProto) int {
		return strings.Compare(left.GetName(), right.GetName())
	})
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, err
	}
	methods := make(map[string]protoreflect.MethodDescriptor)
	files.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		services := file.Services()
		for serviceIndex := 0; serviceIndex < services.Len(); serviceIndex++ {
			service := services.Get(serviceIndex)
			for methodIndex := 0; methodIndex < service.Methods().Len(); methodIndex++ {
				method := service.Methods().Get(methodIndex)
				if method.IsStreamingClient() || method.IsStreamingServer() {
					err = errors.New("streaming gRPC methods are unsupported")
					return false
				}
				fullMethod := "/" + string(service.FullName()) + "/" + string(method.Name())
				if _, duplicate := methods[fullMethod]; duplicate || len(methods) >= maxDescriptorMethods {
					err = errors.New("gRPC method catalog is invalid")
					return false
				}
				methods[fullMethod] = method
			}
		}
		return err == nil
	})
	if err != nil || len(methods) == 0 {
		return nil, errors.New("descriptor set has no supported unary methods")
	}
	return &compiledDescriptors{digest: digest, files: files, methods: methods}, nil
}

func (compiled *compiledDescriptors) Discover(sourceName, artifactDigest string) ([]discoveredTool, error) {
	methodNames := make([]string, 0, len(compiled.methods))
	for name := range compiled.methods {
		methodNames = append(methodNames, name)
	}
	slices.Sort(methodNames)
	tools := make([]discoveredTool, 0, len(methodNames))
	seen := make(map[string]struct{}, len(methodNames))
	for _, fullMethod := range methodNames {
		method := compiled.methods[fullMethod]
		input, err := protoMessageSchema(method.Input(), map[protoreflect.FullName]bool{}, 0, new(int))
		if err != nil {
			return nil, err
		}
		output, err := protoMessageSchema(method.Output(), map[protoreflect.FullName]bool{}, 0, new(int))
		if err != nil {
			return nil, err
		}
		inputJSON, _ := json.Marshal(input)
		outputJSON, _ := json.Marshal(output)
		part := strings.Trim(remoteToolNamePart.ReplaceAllString(strings.TrimPrefix(fullMethod, "/"), "_"), "_.-")
		publicName, err := remotePublicName(sourceName, part)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[publicName]; duplicate {
			return nil, errors.New("gRPC public tool names collide")
		}
		seen[publicName] = struct{}{}
		operation, _ := json.Marshal(map[string]string{
			"full_method": fullMethod, "input_type": string(method.Input().FullName()),
			"output_type": string(method.Output().FullName()), "artifact_sha256": artifactDigest,
		})
		tools = append(tools, discoveredTool{
			PublicName: publicName, UpstreamName: fullMethod, Description: "Unary gRPC " + fullMethod,
			InputSchema: inputJSON, OutputSchema: outputJSON, OperationMetadata: operation,
		})
	}
	return tools, nil
}

func protoMessageSchema(message protoreflect.MessageDescriptor, stack map[protoreflect.FullName]bool, depth int, fieldsSeen *int) (map[string]any, error) {
	if message == nil || depth > maxDescriptorSchemaDepth || stack[message.FullName()] {
		return nil, errors.New("recursive or excessively deep Proto schema is unsupported")
	}
	if special := wellKnownMessageSchema(message.FullName()); special != nil {
		return special, nil
	}
	stack[message.FullName()] = true
	defer delete(stack, message.FullName())
	properties := make(map[string]any)
	required := make([]string, 0)
	fields := message.Fields()
	for index := 0; index < fields.Len(); index++ {
		*fieldsSeen++
		if *fieldsSeen > maxDescriptorFields {
			return nil, errors.New("Proto field count exceeded")
		}
		field := fields.Get(index)
		fieldSchema, err := protoFieldSchema(field, stack, depth+1, fieldsSeen)
		if err != nil {
			return nil, err
		}
		name := field.JSONName()
		properties[name] = fieldSchema
		if field.Cardinality() == protoreflect.Required {
			required = append(required, name)
		}
	}
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		slices.Sort(required)
		schema["required"] = required
	}
	return schema, nil
}

func protoFieldSchema(field protoreflect.FieldDescriptor, stack map[protoreflect.FullName]bool, depth int, fieldsSeen *int) (map[string]any, error) {
	if field.IsMap() {
		value, err := protoSingularSchema(field.MapValue(), stack, depth, fieldsSeen)
		if err != nil {
			return nil, err
		}
		return map[string]any{"type": "object", "additionalProperties": value}, nil
	}
	value, err := protoSingularSchema(field, stack, depth, fieldsSeen)
	if err != nil {
		return nil, err
	}
	if field.IsList() {
		return map[string]any{"type": "array", "items": value}, nil
	}
	return value, nil
}

func protoSingularSchema(field protoreflect.FieldDescriptor, stack map[protoreflect.FullName]bool, depth int, fieldsSeen *int) (map[string]any, error) {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return map[string]any{"type": "boolean"}, nil
	case protoreflect.StringKind:
		return map[string]any{"type": "string"}, nil
	case protoreflect.BytesKind:
		return map[string]any{"type": "string", "contentEncoding": "base64"}, nil
	case protoreflect.FloatKind, protoreflect.DoubleKind:
		return map[string]any{"type": "number"}, nil
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind,
		protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return map[string]any{"type": "integer"}, nil
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind,
		protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "string", "pattern": "^-?[0-9]+$"}}}, nil
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		names := make([]string, 0, values.Len())
		for index := 0; index < values.Len(); index++ {
			names = append(names, string(values.Get(index).Name()))
		}
		return map[string]any{"type": "string", "enum": names}, nil
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return protoMessageSchema(field.Message(), stack, depth, fieldsSeen)
	default:
		return nil, errors.New("unsupported Proto field kind")
	}
}

func wellKnownMessageSchema(name protoreflect.FullName) map[string]any {
	switch name {
	case "google.protobuf.Timestamp":
		return map[string]any{"type": "string", "format": "date-time"}
	case "google.protobuf.Duration", "google.protobuf.FieldMask":
		return map[string]any{"type": "string"}
	case "google.protobuf.Empty":
		return map[string]any{"type": "object", "additionalProperties": false}
	case "google.protobuf.Struct":
		return map[string]any{"type": "object"}
	case "google.protobuf.Value", "google.protobuf.Any":
		return map[string]any{}
	case "google.protobuf.StringValue":
		return map[string]any{"type": "string"}
	case "google.protobuf.BoolValue":
		return map[string]any{"type": "boolean"}
	case "google.protobuf.BytesValue":
		return map[string]any{"type": "string", "contentEncoding": "base64"}
	case "google.protobuf.DoubleValue", "google.protobuf.FloatValue":
		return map[string]any{"type": "number"}
	case "google.protobuf.Int32Value", "google.protobuf.UInt32Value":
		return map[string]any{"type": "integer"}
	case "google.protobuf.Int64Value", "google.protobuf.UInt64Value":
		return map[string]any{"oneOf": []any{map[string]any{"type": "integer"}, map[string]any{"type": "string", "pattern": "^-?[0-9]+$"}}}
	default:
		return nil
	}
}
