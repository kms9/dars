package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	ClientVersion  = "dev"
	ClientPlatform = "cli"
	ClientOS       = normalizeGOOS(runtime.GOOS)
)

func normalizeGOOS(goos string) string {
	switch goos {
	case "darwin":
		return "macos"
	case "windows", "linux":
		return goos
	default:
		return goos
	}
}

type APIClient struct {
	BaseURL     string
	WorkspaceID string
	Token       string
	HTTPClient  *http.Client
	Platform    string
	Version     string
	OS          string
}

type HTTPError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (err *HTTPError) Error() string {
	return fmt.Sprintf("%s %s returned %d: %s", err.Method, err.Path, err.StatusCode, strings.TrimSpace(err.Body))
}

func newHTTPError(method, path string, response *http.Response) *HTTPError {
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	return &HTTPError{Method: method, Path: path, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(data))}
}

const defaultHTTPTimeout = 30 * time.Second

func httpTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("DARS_HTTP_TIMEOUT"))
	if value == "" {
		return defaultHTTPTimeout
	}
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return defaultHTTPTimeout
}

func APITimeout() time.Duration {
	return AtLeastAPITimeout(0)
}

func AtLeastAPITimeout(minimum time.Duration) time.Duration {
	budget := httpTimeout() + 5*time.Second
	if minimum > budget {
		return minimum
	}
	return budget
}

func APIContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, APITimeout())
}

func NewAPIClient(baseURL, workspaceID, token string) *APIClient {
	return &APIClient{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		WorkspaceID: workspaceID,
		Token:       token,
		HTTPClient:  &http.Client{Timeout: httpTimeout()},
	}
}

func (client *APIClient) setHeaders(request *http.Request) {
	if client.Token != "" {
		request.Header.Set("Authorization", "Bearer "+client.Token)
	}
	if client.WorkspaceID != "" {
		request.Header.Set("X-Workspace-ID", client.WorkspaceID)
	}
	platform := client.Platform
	if platform == "" {
		platform = ClientPlatform
	}
	version := client.Version
	if version == "" {
		version = ClientVersion
	}
	osName := client.OS
	if osName == "" {
		osName = ClientOS
	}
	if platform != "" {
		request.Header.Set("X-Client-Platform", platform)
	}
	if version != "" {
		request.Header.Set("X-Client-Version", version)
	}
	if osName != "" {
		request.Header.Set("X-Client-OS", osName)
	}
}

func (client *APIClient) doJSON(ctx context.Context, method, path string, body, out any, headers map[string]string) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	client.setHeaders(request)
	response, err := client.HTTPClient.Do(request)
	if err = wrapTransport(request, err); err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return newHTTPError(method, path, response)
	}
	if out == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil && err != io.EOF {
		return err
	}
	return nil
}

func (client *APIClient) GetJSON(ctx context.Context, path string, out any) error {
	return client.doJSON(ctx, http.MethodGet, path, nil, out, nil)
}

func (client *APIClient) PostJSON(ctx context.Context, path string, body, out any) error {
	return client.PostJSONWithHeaders(ctx, path, body, out, nil)
}

func (client *APIClient) PostJSONWithHeaders(ctx context.Context, path string, body, out any, headers map[string]string) error {
	return client.doJSON(ctx, http.MethodPost, path, body, out, headers)
}

func (client *APIClient) PutJSON(ctx context.Context, path string, body, out any) error {
	return client.doJSON(ctx, http.MethodPut, path, body, out, nil)
}

func (client *APIClient) PatchJSON(ctx context.Context, path string, body, out any) error {
	return client.doJSON(ctx, http.MethodPatch, path, body, out, nil)
}

func (client *APIClient) DeleteJSON(ctx context.Context, path string) error {
	return client.doJSON(ctx, http.MethodDelete, path, nil, nil, nil)
}

func (client *APIClient) DeleteJSONWithBody(ctx context.Context, path string, body any) error {
	return client.doJSON(ctx, http.MethodDelete, path, body, nil, nil)
}

// ImportSkillFile POSTs a local .skill/.zip archive to /api/skills/import as multipart/form-data.
func (client *APIClient) ImportSkillFile(ctx context.Context, fileData []byte, filename, onConflict string, out any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	if onConflict != "" {
		if err := writer.WriteField("on_conflict", onConflict); err != nil {
			return fmt.Errorf("write on_conflict field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.BaseURL+"/api/skills/import", &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	client.setHeaders(request)

	httpClient := client.HTTPClient
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > httpClient.Timeout {
			clientCopy := *httpClient
			clientCopy.Timeout = remaining
			httpClient = &clientCopy
		}
	}
	response, err := httpClient.Do(request)
	if err = wrapTransport(request, err); err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		return newHTTPError(http.MethodPost, "/api/skills/import", response)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(out)
}
