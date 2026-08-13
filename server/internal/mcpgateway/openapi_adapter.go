package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

const (
	maxOpenAPIOperations  = 256
	maxOpenAPIRefs        = 16
	maxOpenAPISchemaDepth = 16
)

type openAPIParameterPlan struct {
	Group string `json:"group"`
	Name  string `json:"name"`
}

type openAPIOperationPlan struct {
	Method      string                 `json:"method"`
	BaseURL     string                 `json:"base_url"`
	Path        string                 `json:"path"`
	Parameters  []openAPIParameterPlan `json:"parameters,omitempty"`
	RequestBody bool                   `json:"request_body,omitempty"`
	ContentType string                 `json:"content_type,omitempty"`
}

// OpenAPIAdapter compiles a bounded OpenAPI document once at revision
// validation and executes only the fixed request plans stored in Bundle items.
type OpenAPIAdapter struct {
	config      Config
	egress      *EgressPolicy
	secretCodec *SourceSecretCodec
}

func NewOpenAPIAdapter(config Config, egress *EgressPolicy, secretCodec *SourceSecretCodec) *OpenAPIAdapter {
	return &OpenAPIAdapter{config: config, egress: egress, secretCodec: secretCodec}
}

func (adapter *OpenAPIAdapter) FetchDocument(ctx context.Context, rawURL string, headers http.Header) ([]byte, string, error) {
	endpoint, err := adapter.egress.ValidateEndpoint(ctx, rawURL)
	if err != nil {
		return nil, "", err
	}
	response, err := adapter.do(ctx, http.MethodGet, endpoint.String(), nil, headers, nil)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", errors.New("OpenAPI document fetch failed")
	}
	body, err := ReadBoundedResponse(response, adapter.config.MaxArtifactBytes)
	if err != nil || len(body) == 0 {
		return nil, "", errors.New("OpenAPI document fetch failed")
	}
	mediaType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mediaType == "" {
		mediaType = "application/vnd.oai.openapi"
	}
	return body, mediaType, nil
}

func (adapter *OpenAPIAdapter) Discover(
	ctx context.Context,
	q *lwdb.Queries,
	source lwdb.ToolSource,
	revision lwdb.ToolSourceRevision,
) ([]discoveredTool, error) {
	if adapter == nil || adapter.egress == nil || q == nil || source.Kind != "openapi" ||
		!revision.Endpoint.Valid || !revision.ArtifactID.Valid {
		return nil, errors.New("OpenAPI Source is invalid")
	}
	baseURL, err := adapter.egress.ValidateEndpoint(ctx, revision.Endpoint.String)
	if err != nil {
		return nil, err
	}
	if baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, ErrEgressDenied
	}
	artifact, err := q.GetToolSourceArtifact(ctx, lwdb.GetToolSourceArtifactParams{
		ID: revision.ArtifactID, WorkspaceID: revision.WorkspaceID, SourceID: revision.SourceID,
	})
	if err != nil || artifact.SizeBytes <= 0 || artifact.SizeBytes > adapter.config.MaxArtifactBytes || int64(len(artifact.Content)) != artifact.SizeBytes {
		return nil, errors.New("OpenAPI artifact is unavailable")
	}
	headers, err := sourceRevisionHeaders(ctx, q, revision, adapter.secretCodec)
	if err != nil {
		return nil, err
	}
	documentURL := ""
	var transport struct {
		DocumentURL string `json:"document_url"`
	}
	if len(revision.TransportConfig) > 0 && json.Unmarshal(revision.TransportConfig, &transport) != nil {
		return nil, errors.New("OpenAPI transport config is invalid")
	}
	documentURL = strings.TrimSpace(transport.DocumentURL)
	document, err := adapter.loadDocument(ctx, artifact.Content, documentURL, headers)
	if err != nil {
		return nil, err
	}
	return compileOpenAPIDocument(source.Name, baseURL.String(), document)
}

func (adapter *OpenAPIAdapter) loadDocument(ctx context.Context, content []byte, documentURL string, headers http.Header) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.Context = ctx
	loader.IsExternalRefsAllowed = true
	readCount := 0
	totalBytes := int64(len(content))
	loader.ReadFromURIFunc = func(_ *openapi3.Loader, location *url.URL) ([]byte, error) {
		if location == nil || (location.Scheme != "http" && location.Scheme != "https") || readCount >= maxOpenAPIRefs {
			return nil, ErrEgressDenied
		}
		readCount++
		response, err := adapter.do(ctx, http.MethodGet, location.String(), nil, headers, nil)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, errors.New("OpenAPI reference fetch failed")
		}
		remaining := adapter.config.MaxArtifactBytes - totalBytes
		if remaining <= 0 {
			return nil, ErrResponseTooBig
		}
		body, err := ReadBoundedResponse(response, remaining)
		if err != nil {
			return nil, err
		}
		totalBytes += int64(len(body))
		return body, nil
	}
	var location *url.URL
	if documentURL != "" {
		var parseErr error
		location, parseErr = adapter.egress.ValidateEndpoint(ctx, documentURL)
		if parseErr != nil {
			return nil, parseErr
		}
	}
	document, err := loadOpenAPIDocument(loader, content, location)
	if err != nil || document == nil {
		return nil, errors.New("OpenAPI document is invalid")
	}
	if err := document.Validate(ctx); err != nil {
		return nil, errors.New("OpenAPI document is invalid")
	}
	return document, nil
}

func loadOpenAPIDocument(loader *openapi3.Loader, content []byte, location *url.URL) (*openapi3.T, error) {
	var version struct {
		OpenAPI string `json:"openapi" yaml:"openapi"`
		Swagger string `json:"swagger" yaml:"swagger"`
	}
	if err := yaml.Unmarshal(content, &version); err != nil {
		return nil, err
	}
	if strings.TrimSpace(version.Swagger) == "2.0" {
		var raw any
		if err := yaml.Unmarshal(content, &raw); err != nil {
			return nil, err
		}
		normalized, err := json.Marshal(raw)
		if err != nil {
			return nil, err
		}
		var documentV2 openapi2.T
		if err := json.Unmarshal(normalized, &documentV2); err != nil {
			return nil, err
		}
		if location != nil {
			return openapi2conv.ToV3WithLoader(&documentV2, loader, location)
		}
		return openapi2conv.ToV3(&documentV2)
	}
	if strings.TrimSpace(version.OpenAPI) == "" {
		return nil, errors.New("OpenAPI version is missing")
	}
	if location != nil {
		return loader.LoadFromDataWithPath(content, location)
	}
	return loader.LoadFromData(content)
}

func compileOpenAPIDocument(sourceName, baseURL string, document *openapi3.T) ([]discoveredTool, error) {
	if document == nil || document.Paths == nil {
		return nil, errors.New("OpenAPI paths are missing")
	}
	paths := document.Paths.Map()
	pathNames := make([]string, 0, len(paths))
	for pathName := range paths {
		pathNames = append(pathNames, pathName)
	}
	slices.Sort(pathNames)
	tools := make([]discoveredTool, 0)
	seen := make(map[string]struct{})
	for _, pathName := range pathNames {
		pathItem := paths[pathName]
		if pathItem == nil || !strings.HasPrefix(pathName, "/") {
			return nil, errors.New("OpenAPI path is invalid")
		}
		operations := pathItem.Operations()
		methods := make([]string, 0, len(operations))
		for method := range operations {
			methods = append(methods, method)
		}
		slices.Sort(methods)
		for _, method := range methods {
			if len(tools) >= maxOpenAPIOperations {
				return nil, errors.New("OpenAPI operation count exceeded")
			}
			operation := operations[method]
			tool, err := compileOpenAPIOperation(sourceName, baseURL, pathName, method, pathItem, operation)
			if err != nil {
				return nil, err
			}
			if _, duplicate := seen[tool.PublicName]; duplicate {
				return nil, errors.New("OpenAPI operation names collide")
			}
			seen[tool.PublicName] = struct{}{}
			tools = append(tools, tool)
		}
	}
	if len(tools) == 0 {
		return nil, errors.New("OpenAPI document has no operations")
	}
	return tools, nil
}

func compileOpenAPIOperation(sourceName, baseURL, pathName, method string, pathItem *openapi3.PathItem, operation *openapi3.Operation) (discoveredTool, error) {
	if operation == nil || len(operation.Callbacks) > 0 {
		return discoveredTool{}, errors.New("OpenAPI callbacks are unsupported")
	}
	operationKey := strings.TrimSpace(operation.OperationID)
	if operationKey == "" {
		operationKey = strings.ToLower(method) + "_" + strings.Trim(remoteToolNamePart.ReplaceAllString(pathName, "_"), "_.-")
	}
	publicName, err := remotePublicName(sourceName, operationKey)
	if err != nil {
		return discoveredTool{}, err
	}
	input := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	properties := input["properties"].(map[string]any)
	groups := map[string]map[string]any{}
	requiredGroups := map[string]bool{}
	parameters := append(append(openapi3.Parameters(nil), pathItem.Parameters...), operation.Parameters...)
	parameterPlans := make([]openAPIParameterPlan, 0, len(parameters))
	seenParameters := make(map[string]struct{})
	for _, reference := range parameters {
		if reference == nil || reference.Value == nil || reference.Value.Schema == nil {
			return discoveredTool{}, errors.New("OpenAPI parameter is invalid")
		}
		parameter := reference.Value
		group := parameter.In
		if group != openapi3.ParameterInPath && group != openapi3.ParameterInQuery && group != openapi3.ParameterInHeader {
			return discoveredTool{}, errors.New("OpenAPI parameter location is unsupported")
		}
		if parameter.Name == "" || (group == openapi3.ParameterInHeader && protectedToolHeader(parameter.Name)) {
			return discoveredTool{}, errors.New("OpenAPI parameter is invalid")
		}
		identity := group + "\x00" + strings.ToLower(parameter.Name)
		if _, duplicate := seenParameters[identity]; duplicate {
			return discoveredTool{}, errors.New("OpenAPI parameters collide")
		}
		seenParameters[identity] = struct{}{}
		schema, err := compileOpenAPISchema(parameter.Schema, map[*openapi3.Schema]bool{}, 0)
		if err != nil {
			return discoveredTool{}, err
		}
		groupSchema := groups[group]
		if groupSchema == nil {
			groupSchema = map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
			groups[group] = groupSchema
			properties[group] = groupSchema
		}
		groupSchema["properties"].(map[string]any)[parameter.Name] = schema
		if parameter.Required || group == openapi3.ParameterInPath {
			required, _ := groupSchema["required"].([]string)
			groupSchema["required"] = append(required, parameter.Name)
			requiredGroups[group] = true
		}
		parameterPlans = append(parameterPlans, openAPIParameterPlan{Group: group, Name: parameter.Name})
	}
	requestBody := false
	contentType := ""
	if operation.RequestBody != nil {
		if operation.RequestBody.Value == nil {
			return discoveredTool{}, errors.New("OpenAPI request body is invalid")
		}
		media := operation.RequestBody.Value.Content.Get("application/json")
		if media == nil || media.Schema == nil {
			return discoveredTool{}, errors.New("only JSON OpenAPI request bodies are supported")
		}
		bodySchema, err := compileOpenAPISchema(media.Schema, map[*openapi3.Schema]bool{}, 0)
		if err != nil {
			return discoveredTool{}, err
		}
		properties["body"] = bodySchema
		requestBody = true
		contentType = "application/json"
		if operation.RequestBody.Value.Required {
			requiredGroups["body"] = true
		}
	}
	if len(requiredGroups) > 0 {
		required := make([]string, 0, len(requiredGroups))
		for group := range requiredGroups {
			required = append(required, group)
		}
		slices.Sort(required)
		input["required"] = required
	}
	slices.SortFunc(parameterPlans, func(left, right openAPIParameterPlan) int {
		return strings.Compare(left.Group+"\x00"+left.Name, right.Group+"\x00"+right.Name)
	})
	plan := openAPIOperationPlan{
		Method: strings.ToUpper(method), BaseURL: baseURL, Path: pathName, Parameters: parameterPlans,
		RequestBody: requestBody, ContentType: contentType,
	}
	operationJSON, _ := json.Marshal(plan)
	inputJSON, _ := json.Marshal(input)
	outputJSON, _ := json.Marshal(map[string]any{
		"type": "object", "properties": map[string]any{"status": map[string]any{"type": "integer"}, "body": map[string]any{}},
		"required": []string{"status", "body"}, "additionalProperties": false,
	})
	description := strings.TrimSpace(operation.Summary)
	if description == "" {
		description = strings.TrimSpace(operation.Description)
	}
	return discoveredTool{
		PublicName: publicName, UpstreamName: operationKey, Description: description,
		InputSchema: inputJSON, OutputSchema: outputJSON, OperationMetadata: operationJSON,
	}, nil
}

func compileOpenAPISchema(reference *openapi3.SchemaRef, stack map[*openapi3.Schema]bool, depth int) (map[string]any, error) {
	if reference == nil || reference.Value == nil || depth > maxOpenAPISchemaDepth || stack[reference.Value] {
		return nil, errors.New("recursive or unresolved OpenAPI schema is unsupported")
	}
	schema := reference.Value
	stack[schema] = true
	defer delete(stack, schema)
	result := make(map[string]any)
	if schema.Type != nil && !schema.Type.IsEmpty() {
		types := schema.Type.Slice()
		if schema.Nullable && !slices.Contains(types, "null") {
			types = append(types, "null")
		}
		if len(types) == 1 {
			result["type"] = types[0]
		} else {
			result["type"] = types
		}
	}
	if schema.Format != "" {
		result["format"] = schema.Format
	}
	if schema.Description != "" {
		result["description"] = schema.Description
	}
	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}
	if schema.Pattern != "" {
		result["pattern"] = schema.Pattern
	}
	if schema.Min != nil {
		result["minimum"] = *schema.Min
	}
	if schema.Max != nil {
		result["maximum"] = *schema.Max
	}
	if schema.MinLength > 0 {
		result["minLength"] = schema.MinLength
	}
	if schema.MaxLength != nil {
		result["maxLength"] = *schema.MaxLength
	}
	if schema.Items != nil {
		items, err := compileOpenAPISchema(schema.Items, stack, depth+1)
		if err != nil {
			return nil, err
		}
		result["items"] = items
	}
	if len(schema.Properties) > 0 {
		properties := make(map[string]any, len(schema.Properties))
		names := make([]string, 0, len(schema.Properties))
		for name := range schema.Properties {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			property, err := compileOpenAPISchema(schema.Properties[name], stack, depth+1)
			if err != nil {
				return nil, err
			}
			properties[name] = property
		}
		result["properties"] = properties
	}
	if len(schema.Required) > 0 {
		required := append([]string(nil), schema.Required...)
		slices.Sort(required)
		result["required"] = required
	}
	if schema.AdditionalProperties.Has != nil {
		result["additionalProperties"] = *schema.AdditionalProperties.Has
	} else if schema.AdditionalProperties.Schema != nil {
		additional, err := compileOpenAPISchema(schema.AdditionalProperties.Schema, stack, depth+1)
		if err != nil {
			return nil, err
		}
		result["additionalProperties"] = additional
	}
	for keyword, values := range map[string]openapi3.SchemaRefs{"oneOf": schema.OneOf, "anyOf": schema.AnyOf, "allOf": schema.AllOf} {
		if len(values) == 0 {
			continue
		}
		compiled := make([]any, 0, len(values))
		for _, value := range values {
			part, err := compileOpenAPISchema(value, stack, depth+1)
			if err != nil {
				return nil, err
			}
			compiled = append(compiled, part)
		}
		result[keyword] = compiled
	}
	return result, nil
}

func (adapter *OpenAPIAdapter) Call(
	ctx context.Context,
	q *lwdb.Queries,
	item lwdb.ToolBundleItem,
	plan bundleInvocationPlan,
	arguments map[string]any,
) (*mcp.CallToolResult, error) {
	if adapter == nil || q == nil || plan.Kind != "openapi" || plan.Endpoint == "" {
		return nil, errors.New("OpenAPI invocation plan is invalid")
	}
	revision, err := q.GetToolSourceRevision(ctx, lwdb.GetToolSourceRevisionParams{
		ID: item.SourceRevisionID, WorkspaceID: item.WorkspaceID, SourceID: item.SourceID,
	})
	if err != nil || (revision.Status != "ready" && revision.Status != "retired") || !revision.Endpoint.Valid || revision.Endpoint.String != plan.Endpoint {
		return nil, errors.New("OpenAPI revision is unavailable")
	}
	endpoint, err := adapter.egress.ValidateEndpoint(ctx, plan.Endpoint)
	if err != nil {
		return nil, err
	}
	var operation openAPIOperationPlan
	if err := json.Unmarshal(plan.Operation, &operation); err != nil || operation.BaseURL != endpoint.String() ||
		operation.Method == "" || !strings.HasPrefix(operation.Path, "/") || plan.UpstreamName == "" {
		return nil, errors.New("OpenAPI operation plan is invalid")
	}
	authHeaders, err := sourceRevisionHeaders(ctx, q, revision, adapter.secretCodec)
	if err != nil {
		return nil, err
	}
	return adapter.invoke(ctx, operation, arguments, authHeaders)
}

func (adapter *OpenAPIAdapter) invoke(
	ctx context.Context,
	operation openAPIOperationPlan,
	arguments map[string]any,
	authHeaders http.Header,
) (*mcp.CallToolResult, error) {
	endpoint, err := adapter.egress.ValidateEndpoint(ctx, operation.BaseURL)
	if err != nil {
		return nil, err
	}
	requestURL := *endpoint
	plainPath := operation.Path
	escapedPath := operation.Path
	query := requestURL.Query()
	requestHeaders := make(http.Header)
	for _, parameter := range operation.Parameters {
		group, _ := arguments[parameter.Group].(map[string]any)
		value, present := group[parameter.Name]
		if !present {
			continue
		}
		switch parameter.Group {
		case openapi3.ParameterInPath:
			placeholder := "{" + parameter.Name + "}"
			if !strings.Contains(plainPath, placeholder) {
				return nil, errors.New("OpenAPI path parameter mismatch")
			}
			plainValue := openAPIString(value)
			plainPath = strings.ReplaceAll(plainPath, placeholder, plainValue)
			escapedPath = strings.ReplaceAll(escapedPath, placeholder, url.PathEscape(plainValue))
		case openapi3.ParameterInQuery:
			for _, encoded := range openAPIValues(value) {
				query.Add(parameter.Name, encoded)
			}
		case openapi3.ParameterInHeader:
			if protectedToolHeader(parameter.Name) {
				return nil, errors.New("OpenAPI protected header override")
			}
			requestHeaders.Set(parameter.Name, openAPIString(value))
		default:
			return nil, errors.New("OpenAPI parameter location is invalid")
		}
	}
	if strings.Contains(plainPath, "{") || strings.Contains(plainPath, "}") {
		return nil, errors.New("OpenAPI path parameters are incomplete")
	}
	requestURL.Path = strings.TrimRight(endpoint.Path, "/") + plainPath
	requestURL.RawPath = strings.TrimRight(endpoint.EscapedPath(), "/") + escapedPath
	requestURL.RawQuery = query.Encode()
	var body io.Reader
	if operation.RequestBody {
		bodyValue, present := arguments["body"]
		if present {
			encoded, err := json.Marshal(bodyValue)
			if err != nil || int64(len(encoded)) > adapter.config.MaxRequestBodyBytes {
				return nil, errors.New("OpenAPI request body is invalid")
			}
			body = strings.NewReader(string(encoded))
			requestHeaders.Set("Content-Type", operation.ContentType)
		}
	}
	response, err := adapter.do(ctx, operation.Method, requestURL.String(), body, authHeaders, requestHeaders)
	if err != nil {
		return nil, errors.New("OpenAPI tool call failed")
	}
	defer response.Body.Close()
	responseBody, err := ReadBoundedResponse(response, adapter.config.MaxResponseBytes)
	if err != nil {
		return nil, errors.New("OpenAPI response is too large")
	}
	var bodyValue any = string(responseBody)
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "json") && len(responseBody) > 0 {
		if err := json.Unmarshal(responseBody, &bodyValue); err != nil {
			return nil, errors.New("OpenAPI JSON response is invalid")
		}
	}
	structured := map[string]any{"status": response.StatusCode, "body": bodyValue}
	encoded, _ := json.Marshal(structured)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}, StructuredContent: structured,
		IsError: response.StatusCode >= 400,
	}, nil
}

func (adapter *OpenAPIAdapter) do(
	ctx context.Context,
	method string,
	rawURL string,
	body io.Reader,
	authHeaders http.Header,
	requestHeaders http.Header,
) (*http.Response, error) {
	client := adapter.egress.NewHTTPClient()
	defer client.CloseIdleConnections()
	base := client.standardClient()
	credentialNames := make(map[string]struct{}, len(authHeaders))
	for name := range authHeaders {
		credentialNames[http.CanonicalHeaderKey(name)] = struct{}{}
	}
	authorized := &http.Client{Transport: base.Transport, Timeout: base.Timeout}
	authorized.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if err := base.CheckRedirect(request, via); err != nil {
			return err
		}
		if len(via) > 0 && !sameURLAuthority(via[len(via)-1].URL, request.URL) {
			for name := range credentialNames {
				request.Header.Del(name)
			}
		}
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	for name, values := range requestHeaders {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	for name, values := range authHeaders {
		request.Header.Del(name)
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}
	if err := adapter.egress.ValidateURL(ctx, request.URL); err != nil {
		return nil, err
	}
	return authorized.Do(request)
}

func protectedToolHeader(name string) bool {
	canonical := http.CanonicalHeaderKey(strings.TrimSpace(name))
	if canonical == "" || protectedUpstreamHeader(canonical) {
		return true
	}
	switch strings.ToLower(canonical) {
	case "authorization", "cookie", "proxy-authorization", "forwarded", "x-forwarded-for", "x-forwarded-host", "x-forwarded-proto":
		return true
	default:
		return false
	}
}

func openAPIValues(value any) []string {
	switch values := value.(type) {
	case []any:
		result := make([]string, 0, len(values))
		for _, item := range values {
			result = append(result, openAPIString(item))
		}
		return result
	default:
		return []string{openAPIString(value)}
	}
}

func openAPIString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}
