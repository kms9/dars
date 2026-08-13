package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/kms9/dars/internal/util"
	"github.com/kms9/dars/internal/versionedsecret"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxRemoteMCPTools       = 256
	maxRemoteMCPPages       = 16
	maxRemoteMCPSchemaBytes = 256 << 10
)

var remoteToolNamePart = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

type discoveredTool struct {
	PublicName        string
	UpstreamName      string
	Description       string
	InputSchema       json.RawMessage
	OutputSchema      json.RawMessage
	OperationMetadata json.RawMessage
}

// RemoteMCPAdapter validates and calls stateless, tools-only upstream MCP
// servers through the shared connect-time egress policy.
type RemoteMCPAdapter struct {
	config      Config
	egress      *EgressPolicy
	secretCodec *SourceSecretCodec
}

func NewRemoteMCPAdapter(config Config, egress *EgressPolicy, secretCodec *SourceSecretCodec) *RemoteMCPAdapter {
	return &RemoteMCPAdapter{config: config, egress: egress, secretCodec: secretCodec}
}

func (adapter *RemoteMCPAdapter) Discover(
	ctx context.Context,
	q *lwdb.Queries,
	source lwdb.ToolSource,
	revision lwdb.ToolSourceRevision,
) ([]discoveredTool, error) {
	if adapter == nil || adapter.egress == nil || source.Kind != "remote_mcp" || !revision.Endpoint.Valid || revision.ArtifactID.Valid {
		return nil, errors.New("remote MCP source is invalid")
	}
	endpoint, err := adapter.egress.ValidateEndpoint(ctx, revision.Endpoint.String)
	if err != nil {
		return nil, err
	}
	headers, err := adapter.revisionHeaders(ctx, q, revision)
	if err != nil {
		return nil, err
	}
	session, client, err := adapter.connect(ctx, endpoint.String(), headers)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = session.Close()
		client.CloseIdleConnections()
	}()
	if !toolsOnlyCapabilities(session.InitializeResult()) {
		return nil, errors.New("remote MCP server requires unsupported capabilities")
	}

	tools := make([]discoveredTool, 0)
	seenPublicNames := make(map[string]struct{})
	cursor := ""
	for page := 0; page < maxRemoteMCPPages; page++ {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, errors.Join(errors.New("remote MCP tool discovery failed"), err)
		}
		for _, tool := range result.Tools {
			if tool == nil || strings.TrimSpace(tool.Name) == "" || len(tools) >= maxRemoteMCPTools {
				return nil, errors.New("remote MCP tool catalog is invalid")
			}
			inputSchema, err := normalizeRemoteSchema(tool.InputSchema, true)
			if err != nil {
				return nil, err
			}
			outputSchema, err := normalizeRemoteSchema(tool.OutputSchema, false)
			if err != nil {
				return nil, err
			}
			publicName, err := remotePublicName(source.Name, tool.Name)
			if err != nil {
				return nil, err
			}
			if _, duplicate := seenPublicNames[publicName]; duplicate {
				return nil, errors.New("remote MCP tool names collide")
			}
			seenPublicNames[publicName] = struct{}{}
			operation, _ := json.Marshal(map[string]string{"upstream_name": tool.Name})
			tools = append(tools, discoveredTool{
				PublicName: publicName, UpstreamName: tool.Name, Description: strings.TrimSpace(tool.Description),
				InputSchema: inputSchema, OutputSchema: outputSchema, OperationMetadata: operation,
			})
		}
		if result.NextCursor == "" {
			break
		}
		if result.NextCursor == cursor || page == maxRemoteMCPPages-1 {
			return nil, errors.New("remote MCP pagination is invalid")
		}
		cursor = result.NextCursor
	}
	if len(tools) == 0 {
		return nil, errors.New("remote MCP server published no tools")
	}
	return tools, nil
}

func (adapter *RemoteMCPAdapter) Call(
	ctx context.Context,
	q *lwdb.Queries,
	item lwdb.ToolBundleItem,
	plan bundleInvocationPlan,
	arguments map[string]any,
) (*mcp.CallToolResult, error) {
	if adapter == nil || adapter.egress == nil || plan.Kind != "remote_mcp" || plan.Endpoint == "" || plan.UpstreamName == "" {
		return nil, errors.New("remote MCP invocation plan is invalid")
	}
	revision, err := q.GetToolSourceRevision(ctx, lwdb.GetToolSourceRevisionParams{
		ID: item.SourceRevisionID, WorkspaceID: item.WorkspaceID, SourceID: item.SourceID,
	})
	if err != nil || (revision.Status != "ready" && revision.Status != "retired") || !revision.Endpoint.Valid || revision.Endpoint.String != plan.Endpoint {
		return nil, errors.New("remote MCP revision is unavailable")
	}
	endpoint, err := adapter.egress.ValidateEndpoint(ctx, plan.Endpoint)
	if err != nil {
		return nil, err
	}
	headers, err := adapter.revisionHeaders(ctx, q, revision)
	if err != nil {
		return nil, err
	}
	session, client, err := adapter.connect(ctx, endpoint.String(), headers)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = session.Close()
		client.CloseIdleConnections()
	}()
	if !toolsOnlyCapabilities(session.InitializeResult()) {
		return nil, errors.New("remote MCP server capabilities changed")
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: plan.UpstreamName, Arguments: arguments})
	if err != nil {
		return nil, errors.New("remote MCP tool call failed")
	}
	if result == nil {
		return nil, errors.New("remote MCP tool result is empty")
	}
	return result, nil
}

func (adapter *RemoteMCPAdapter) connect(ctx context.Context, endpoint string, headers http.Header) (*mcp.ClientSession, *EgressHTTPClient, error) {
	client := adapter.egress.NewHTTPClient()
	base := client.standardClient()
	authorizedClient := &http.Client{
		Transport: &fixedHeaderRoundTripper{
			base: base.Transport, headers: headers, maxBytes: adapter.config.MaxResponseBytes,
			authority: endpointAuthority(endpoint),
		},
		CheckRedirect: base.CheckRedirect,
		Timeout:       base.Timeout,
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "dars-gateway-upstream", Version: "1"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{},
	})
	session, err := mcpClient.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: endpoint, HTTPClient: authorizedClient, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	if err != nil {
		client.CloseIdleConnections()
		return nil, nil, errors.Join(errors.New("remote MCP connection failed"), err)
	}
	return session, client, nil
}

func (adapter *RemoteMCPAdapter) revisionHeaders(ctx context.Context, q *lwdb.Queries, revision lwdb.ToolSourceRevision) (http.Header, error) {
	return sourceRevisionHeaders(ctx, q, revision, adapter.secretCodec)
}

func sourceRevisionHeaders(ctx context.Context, q *lwdb.Queries, revision lwdb.ToolSourceRevision, secretCodec *SourceSecretCodec) (http.Header, error) {
	headers := make(http.Header)
	if !revision.SecretID.Valid {
		return headers, nil
	}
	if secretCodec == nil || q == nil {
		return nil, errors.New("remote MCP credential codec is unavailable")
	}
	secret, err := q.GetToolSourceSecret(ctx, lwdb.GetToolSourceSecretParams{
		ID: revision.SecretID, WorkspaceID: revision.WorkspaceID, SourceID: revision.SourceID,
	})
	if err != nil {
		return nil, errors.New("remote MCP credential is unavailable")
	}
	var envelope versionedsecret.Envelope
	if err := json.Unmarshal(secret.Envelope, &envelope); err != nil {
		return nil, errors.New("remote MCP credential is invalid")
	}
	plaintext, err := secretCodec.Open(SourceSecretScope{
		WorkspaceID: util.UUIDToString(revision.WorkspaceID), SourceID: util.UUIDToString(revision.SourceID), RevisionID: util.UUIDToString(revision.ID),
	}, envelope)
	if err != nil {
		return nil, errors.New("remote MCP credential is invalid")
	}
	return parseSourceAuthHeaders(plaintext)
}

type sourceAuthDocument struct {
	Kind    string            `json:"kind"`
	Token   string            `json:"token,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func parseSourceAuthHeaders(raw []byte) (http.Header, error) {
	var document sourceAuthDocument
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, errors.New("source credential is invalid")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("source credential is invalid")
	}
	headers := make(http.Header)
	switch strings.ToLower(strings.TrimSpace(document.Kind)) {
	case "bearer":
		if strings.TrimSpace(document.Token) == "" || len(document.Headers) != 0 {
			return nil, errors.New("source credential is invalid")
		}
		headers.Set("Authorization", "Bearer "+document.Token)
	case "headers":
		if document.Token != "" || len(document.Headers) == 0 || len(document.Headers) > 32 {
			return nil, errors.New("source credential is invalid")
		}
		for name, value := range document.Headers {
			canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
			if canonical == "" || protectedUpstreamHeader(canonical) || strings.ContainsAny(value, "\r\n") || len(value) > 8192 {
				return nil, errors.New("source credential is invalid")
			}
			headers.Set(canonical, value)
		}
	default:
		return nil, errors.New("source credential is invalid")
	}
	return headers, nil
}

func protectedUpstreamHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "connection", "content-length", "transfer-encoding", "upgrade", "te", "trailer", "proxy-authorization", "proxy-connection":
		return true
	default:
		return false
	}
}

func toolsOnlyCapabilities(result *mcp.InitializeResult) bool {
	if result == nil || result.Capabilities == nil || result.Capabilities.Tools == nil {
		return false
	}
	capabilities := result.Capabilities
	return capabilities.Completions == nil && capabilities.Logging == nil && capabilities.Prompts == nil && capabilities.Resources == nil &&
		len(capabilities.Experimental) == 0 && len(capabilities.Extensions) == 0
}

func normalizeRemoteSchema(value any, input bool) (json.RawMessage, error) {
	if value == nil {
		if input {
			return json.RawMessage(`{"type":"object"}`), nil
		}
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > maxRemoteMCPSchemaBytes || !jsonObject(encoded) {
		return nil, errors.New("remote MCP schema is invalid")
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(encoded, &schema); err != nil || (input && schema.Type != "object") {
		return nil, errors.New("remote MCP schema is invalid")
	}
	if _, err := schema.Resolve(nil); err != nil {
		return nil, errors.New("remote MCP schema is invalid")
	}
	return encoded, nil
}

func remotePublicName(sourceName, upstreamName string) (string, error) {
	part := strings.Trim(remoteToolNamePart.ReplaceAllString(strings.TrimSpace(upstreamName), "_"), "_.-")
	if part == "" {
		return "", errors.New("remote MCP tool name is invalid")
	}
	name := sourceName + "." + part
	if len(name) > 128 {
		return "", errors.New("remote MCP tool name is too long")
	}
	return name, nil
}

type fixedHeaderRoundTripper struct {
	base      http.RoundTripper
	headers   http.Header
	maxBytes  int64
	authority string
}

func (transport *fixedHeaderRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if transport == nil || transport.base == nil || request == nil || transport.maxBytes <= 0 {
		return nil, errors.New("remote MCP transport is invalid")
	}
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	if transport.authority != "" && strings.EqualFold(request.URL.Host, transport.authority) {
		for name, values := range transport.headers {
			cloned.Header.Del(name)
			for _, value := range values {
				cloned.Header.Add(name, value)
			}
		}
	}
	response, err := transport.base.RoundTrip(cloned)
	if err != nil {
		return nil, err
	}
	if response.Body == nil {
		return response, nil
	}
	response.Body = &boundedReadCloser{body: response.Body, remaining: transport.maxBytes}
	return response, nil
}

func endpointAuthority(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}

type boundedReadCloser struct {
	body      io.ReadCloser
	remaining int64
}

func (body *boundedReadCloser) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if body.remaining == 0 {
		var probe [1]byte
		count, err := body.body.Read(probe[:])
		if count > 0 {
			return 0, ErrResponseTooBig
		}
		return 0, err
	}
	readLimit := int64(len(buffer))
	if readLimit > body.remaining+1 {
		readLimit = body.remaining + 1
	}
	count, err := body.body.Read(buffer[:int(readLimit)])
	if int64(count) > body.remaining {
		allowed := int(body.remaining)
		body.remaining = 0
		return allowed, ErrResponseTooBig
	}
	body.remaining -= int64(count)
	return count, err
}

func (body *boundedReadCloser) Close() error { return body.body.Close() }
