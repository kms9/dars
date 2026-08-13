package mcpgateway

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	lwdb "github.com/kms9/dars/pkg/lightweightdb"
	"github.com/oklog/ulid/v2"
)

const maxControlBodyBytes = int64(1 << 20)

type ControlPlaneOptions struct {
	SecretCodec       *SourceSecretCodec
	ProviderAllowlist map[string]bool
	MaxArtifactBytes  int64
	ValidationWorkers int
	ValidationTimeout time.Duration
	RemoteMCP         *RemoteMCPAdapter
	GRPC              *GRPCAdapter
	OpenAPI           *OpenAPIAdapter
	Observer          Observer
}

// ControlPlane owns the backend-only Human administration API. It never
// returns secret plaintext or artifact contents from read endpoints.
type ControlPlane struct {
	pool              *pgxpool.Pool
	q                 *lwdb.Queries
	secretCodec       *SourceSecretCodec
	providerAllowlist map[string]bool
	maxArtifactBytes  int64
	remoteMCP         *RemoteMCPAdapter
	grpc              *GRPCAdapter
	openAPI           *OpenAPIAdapter
	observer          Observer
	validationWorkers *keyedGate
	validationTimeout time.Duration
}

func NewControlPlane(pool *pgxpool.Pool, options ControlPlaneOptions) *ControlPlane {
	maxArtifactBytes := options.MaxArtifactBytes
	if maxArtifactBytes <= 0 {
		maxArtifactBytes = defaultMaxArtifactBytes
	}
	validationWorkers := options.ValidationWorkers
	if validationWorkers <= 0 {
		validationWorkers = defaultSourceConcurrency
	}
	validationTimeout := options.ValidationTimeout
	if validationTimeout <= 0 {
		validationTimeout = defaultInvocationTimeout
	}
	allowlist := make(map[string]bool, len(options.ProviderAllowlist))
	for provider, allowed := range options.ProviderAllowlist {
		allowlist[strings.ToLower(strings.TrimSpace(provider))] = allowed
	}
	return &ControlPlane{
		pool: pool, q: lwdb.New(pool), secretCodec: options.SecretCodec,
		providerAllowlist: allowlist, maxArtifactBytes: maxArtifactBytes,
		remoteMCP:         options.RemoteMCP,
		grpc:              options.GRPC,
		openAPI:           options.OpenAPI,
		observer:          observerOrNop(options.Observer),
		validationWorkers: newKeyedGate(validationWorkers),
		validationTimeout: validationTimeout,
	}
}

type stagedToolInput struct {
	PublicName        string          `json:"public_name"`
	UpstreamName      string          `json:"upstream_name"`
	Description       string          `json:"description"`
	InputSchema       json.RawMessage `json:"input_schema"`
	OutputSchema      json.RawMessage `json:"output_schema,omitempty"`
	OperationMetadata json.RawMessage `json:"operation_metadata,omitempty"`
}

type createToolSourceRequest struct {
	Name            string            `json:"name"`
	Kind            string            `json:"kind"`
	Endpoint        *string           `json:"endpoint,omitempty"`
	TransportConfig json.RawMessage   `json:"transport_config,omitempty"`
	ArtifactID      *string           `json:"artifact_id,omitempty"`
	Auth            json.RawMessage   `json:"auth,omitempty"`
	Tools           []stagedToolInput `json:"tools,omitempty"`
}

type updateToolSourceRequest struct {
	Endpoint        *string           `json:"endpoint,omitempty"`
	TransportConfig json.RawMessage   `json:"transport_config,omitempty"`
	ArtifactID      *string           `json:"artifact_id,omitempty"`
	Auth            json.RawMessage   `json:"auth,omitempty"`
	Tools           []stagedToolInput `json:"tools,omitempty"`
}

type validateToolSourceRequest struct {
	RevisionID string `json:"revision_id"`
}

type publishToolBundleItemRequest struct {
	ToolDefinitionID string `json:"tool_definition_id"`
	ExportedName     string `json:"exported_name"`
}

type publishToolBundleRequest struct {
	Items []publishToolBundleItemRequest `json:"items"`
}

type uploadToolSourceArtifactRequest struct {
	MediaType     string `json:"media_type"`
	ContentBase64 string `json:"content_base64"`
}

type importToolSourceResponse struct {
	Source   sourceDTO `json:"source"`
	Tools    []toolDTO `json:"tools"`
	Artifact struct {
		ID        string `json:"id"`
		SHA256    string `json:"sha256"`
		MediaType string `json:"media_type"`
		SizeBytes int64  `json:"size_bytes"`
	} `json:"artifact"`
}

type sourceDTO struct {
	ID              string       `json:"id"`
	WorkspaceID     string       `json:"workspace_id"`
	Name            string       `json:"name"`
	Kind            string       `json:"kind"`
	Enabled         bool         `json:"enabled"`
	CurrentRevision *string      `json:"current_revision,omitempty"`
	Revision        *revisionDTO `json:"revision,omitempty"`
	CreatedAt       string       `json:"created_at"`
	UpdatedAt       string       `json:"updated_at"`
}

type revisionDTO struct {
	ID               string          `json:"id"`
	Revision         int32           `json:"revision"`
	Status           string          `json:"status"`
	Endpoint         *string         `json:"endpoint,omitempty"`
	TransportConfig  json.RawMessage `json:"transport_config"`
	ArtifactID       *string         `json:"artifact_id,omitempty"`
	SecretConfigured bool            `json:"secret_configured"`
	SecretKeyID      *string         `json:"secret_key_id,omitempty"`
	ValidationCode   *string         `json:"validation_code,omitempty"`
	CreatedAt        string          `json:"created_at"`
	PublishedAt      *string         `json:"published_at,omitempty"`
}

type toolDTO struct {
	ID                string          `json:"id"`
	PublicName        string          `json:"public_name"`
	UpstreamName      string          `json:"upstream_name"`
	Description       string          `json:"description"`
	InputSchema       json.RawMessage `json:"input_schema"`
	OutputSchema      json.RawMessage `json:"output_schema,omitempty"`
	OperationMetadata json.RawMessage `json:"operation_metadata"`
	Enabled           bool            `json:"enabled"`
}

type bundleDTO struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id"`
	ManifestHash string          `json:"manifest_hash"`
	Status       string          `json:"status"`
	Items        []bundleItemDTO `json:"items"`
	CreatedAt    string          `json:"created_at"`
	RevokedAt    *string         `json:"revoked_at,omitempty"`
}

type bundleItemDTO struct {
	Ordinal             int32           `json:"ordinal"`
	ExportedName        string          `json:"exported_name"`
	CanonicalPublicName string          `json:"canonical_public_name"`
	SourceID            string          `json:"source_id"`
	SourceRevisionID    string          `json:"source_revision_id"`
	ToolDefinitionID    string          `json:"tool_definition_id"`
	Definition          json.RawMessage `json:"definition"`
}

func (control *ControlPlane) ListSources(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	rows, err := control.q.ListToolSources(r.Context(), lwdb.ListToolSourcesParams{WorkspaceID: workspaceID, PageLimit: 100})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	response := make([]sourceDTO, 0, len(rows))
	for _, source := range rows {
		var revision *lwdb.ToolSourceRevision
		if revisions, revisionErr := control.q.ListToolSourceRevisions(r.Context(), lwdb.ListToolSourceRevisionsParams{
			WorkspaceID: workspaceID,
			SourceID:    source.ID,
		}); revisionErr == nil && len(revisions) > 0 {
			revision = &revisions[0]
		}
		response = append(response, sourceResponse(source, revision, nil))
	}
	writeControlJSON(w, http.StatusOK, response)
}

func (control *ControlPlane) GetSource(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	sourceID, err := parseControlUUID(chi.URLParam(r, "sourceId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	source, err := control.q.GetToolSource(r.Context(), lwdb.GetToolSourceParams{ID: sourceID, WorkspaceID: workspaceID})
	if err != nil {
		writeControlError(w, notFoundControlError())
		return
	}
	var revision *lwdb.ToolSourceRevision
	var secret *lwdb.GetToolSourceSecretMetadataRow
	if revisions, revisionErr := control.q.ListToolSourceRevisions(r.Context(), lwdb.ListToolSourceRevisionsParams{
		WorkspaceID: workspaceID,
		SourceID:    source.ID,
	}); revisionErr == nil && len(revisions) > 0 {
		revision = &revisions[0]
		if revision.SecretID.Valid {
			metadata, metadataErr := control.q.GetToolSourceSecretMetadata(r.Context(), lwdb.GetToolSourceSecretMetadataParams{
				ID: revision.SecretID, WorkspaceID: workspaceID, SourceID: source.ID,
			})
			if metadataErr == nil {
				secret = &metadata
			}
		}
	}
	writeControlJSON(w, http.StatusOK, sourceResponse(source, revision, secret))
}

func (control *ControlPlane) CreateSource(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	var request createToolSourceRequest
	if err := decodeControlBody(r, &request); err != nil {
		writeControlError(w, invalidControlError("tool_source_invalid"))
		return
	}
	request.Name = normalizeSourceName(request.Name)
	request.Kind = strings.ToLower(strings.TrimSpace(request.Kind))
	if request.Name == "" || !validSourceKind(request.Kind) || (request.Kind != "server_local" && len(request.Tools) > 0) {
		writeControlError(w, invalidControlError("tool_source_invalid"))
		return
	}
	tx, err := control.pool.Begin(r.Context())
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	defer tx.Rollback(r.Context())
	q := lwdb.New(tx)
	source, err := q.CreateToolSource(r.Context(), lwdb.CreateToolSourceParams{
		WorkspaceID: workspaceID, Name: request.Name, Kind: request.Kind, CreatedBy: actorID,
	})
	if err != nil {
		writeControlError(w, conflictControlError("tool_source_conflict"))
		return
	}
	revision, secret, err := control.stageRevision(r.Context(), q, source, actorID, updateToolSourceRequest{
		Endpoint: request.Endpoint, TransportConfig: request.TransportConfig,
		ArtifactID: request.ArtifactID, Auth: request.Auth, Tools: request.Tools,
	})
	if err != nil {
		writeControlError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	writeControlJSON(w, http.StatusCreated, sourceResponse(source, &revision, secret))
}

func (control *ControlPlane) UpdateSource(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	sourceID, err := parseControlUUID(chi.URLParam(r, "sourceId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	var request updateToolSourceRequest
	if err := decodeControlBody(r, &request); err != nil {
		writeControlError(w, invalidControlError("tool_source_invalid"))
		return
	}
	tx, err := control.pool.Begin(r.Context())
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	defer tx.Rollback(r.Context())
	q := lwdb.New(tx)
	source, err := q.GetToolSource(r.Context(), lwdb.GetToolSourceParams{ID: sourceID, WorkspaceID: workspaceID})
	if err != nil || (source.Kind != "server_local" && len(request.Tools) > 0) {
		writeControlError(w, notFoundControlError())
		return
	}
	revision, secret, err := control.stageRevision(r.Context(), q, source, actorID, request)
	if err != nil {
		writeControlError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	writeControlJSON(w, http.StatusCreated, sourceResponse(source, &revision, secret))
}

func (control *ControlPlane) ValidateSource(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	sourceID, err := parseControlUUID(chi.URLParam(r, "sourceId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	var request validateToolSourceRequest
	if err := decodeControlBody(r, &request); err != nil {
		writeControlError(w, invalidControlError("tool_source_invalid"))
		return
	}
	revisionID, err := parseControlUUID(request.RevisionID)
	if err != nil {
		writeControlError(w, err)
		return
	}
	validationCtx, cancelValidation := context.WithTimeout(r.Context(), control.validationTimeout)
	defer cancelValidation()
	releaseValidation, err := control.validationWorkers.acquire(validationCtx, "source-validation")
	if err != nil {
		writeControlError(w, conflictControlError("tool_source_validation_limited"))
		return
	}
	defer releaseValidation()
	tx, err := control.pool.Begin(validationCtx)
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	defer tx.Rollback(validationCtx)
	q := lwdb.New(tx)
	source, err := q.GetToolSource(validationCtx, lwdb.GetToolSourceParams{ID: sourceID, WorkspaceID: workspaceID})
	if err != nil {
		writeControlError(w, notFoundControlError())
		return
	}
	revision, err := q.GetToolSourceRevision(validationCtx, lwdb.GetToolSourceRevisionParams{ID: revisionID, WorkspaceID: workspaceID, SourceID: sourceID})
	if err != nil || revision.Status != "validating" {
		writeControlError(w, conflictControlError("tool_source_revision_conflict"))
		return
	}
	validationCode, err := control.discoverSourceRevision(validationCtx, q, source, revision)
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	if validationCode != "" {
		if _, markErr := q.MarkToolSourceRevisionFailed(validationCtx, lwdb.MarkToolSourceRevisionFailedParams{
			ID: revisionID, WorkspaceID: workspaceID, SourceID: sourceID,
			ValidationCode: pgtype.Text{String: validationCode, Valid: true},
		}); markErr != nil {
			writeControlError(w, internalControlError())
			return
		}
		if err := tx.Commit(validationCtx); err != nil {
			writeControlError(w, internalControlError())
			return
		}
		control.observer.RecordSourceValidation(source.Kind, validationCode)
		control.recordControlActivity(r.Context(), workspaceID, actorID, "mcp_gateway_source_validated", map[string]any{
			"source_id": uuidString(sourceID), "source_revision_id": uuidString(revisionID),
			"source_kind": source.Kind, "outcome": validationCode,
		})
		writeControlError(w, invalidControlError(validationCode))
		return
	}
	ready, err := q.MarkToolSourceRevisionReady(validationCtx, lwdb.MarkToolSourceRevisionReadyParams{ID: revisionID, WorkspaceID: workspaceID, SourceID: sourceID})
	if err != nil {
		writeControlError(w, conflictControlError("tool_source_revision_conflict"))
		return
	}
	updated, err := q.AdvanceToolSourceCurrentRevision(validationCtx, lwdb.AdvanceToolSourceCurrentRevisionParams{
		SourceID: sourceID, WorkspaceID: workspaceID, RevisionID: revisionID,
	})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	if err := tx.Commit(validationCtx); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	control.observer.RecordSourceValidation(source.Kind, "ready")
	control.recordControlActivity(r.Context(), workspaceID, actorID, "mcp_gateway_source_validated", map[string]any{
		"source_id": uuidString(sourceID), "source_revision_id": uuidString(revisionID),
		"source_kind": source.Kind, "outcome": "ready",
	})
	writeControlJSON(w, http.StatusOK, sourceResponse(updated, &ready, nil))
}

func (control *ControlPlane) discoverSourceRevision(
	ctx context.Context,
	q *lwdb.Queries,
	source lwdb.ToolSource,
	revision lwdb.ToolSourceRevision,
) (string, error) {
	validationCode := ""
	var discovered []discoveredTool
	var discoverErr error
	switch source.Kind {
	case "remote_mcp":
		if control.remoteMCP == nil {
			validationCode = "tool_source_validator_unavailable"
		} else {
			discovered, discoverErr = control.remoteMCP.Discover(ctx, q, source, revision)
			if discoverErr != nil {
				validationCode = remoteMCPValidationCode(discoverErr)
			}
		}
	case "grpc":
		if control.grpc == nil {
			validationCode = "tool_source_validator_unavailable"
		} else {
			discovered, discoverErr = control.grpc.Discover(ctx, q, source, revision)
			if discoverErr != nil {
				validationCode = grpcValidationCode(discoverErr)
			}
		}
	case "openapi":
		if control.openAPI == nil {
			validationCode = "tool_source_validator_unavailable"
		} else {
			discovered, discoverErr = control.openAPI.Discover(ctx, q, source, revision)
			if discoverErr != nil {
				validationCode = openAPIValidationCode(discoverErr)
			}
		}
	}
	if discoverErr == nil && len(discovered) > 0 {
		if err := createDiscoveredToolDefinitions(ctx, q, source, revision, discovered); err != nil {
			return "", err
		}
	}
	definitions, err := q.ListToolDefinitionsByRevision(ctx, lwdb.ListToolDefinitionsByRevisionParams{
		WorkspaceID:      source.WorkspaceID,
		SourceID:         source.ID,
		SourceRevisionID: revision.ID,
	})
	if err != nil {
		return "", err
	}
	if validationCode == "" {
		validationCode = validateStagedRevision(source, revision, definitions)
	}
	return validationCode, nil
}

func (control *ControlPlane) EnableSource(w http.ResponseWriter, r *http.Request) {
	control.setSourceEnabled(w, r, true)
}

func (control *ControlPlane) DisableSource(w http.ResponseWriter, r *http.Request) {
	control.setSourceEnabled(w, r, false)
}

func (control *ControlPlane) setSourceEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	sourceID, err := parseControlUUID(chi.URLParam(r, "sourceId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	source, err := control.q.SetToolSourceEnabled(r.Context(), lwdb.SetToolSourceEnabledParams{Enabled: enabled, ID: sourceID, WorkspaceID: workspaceID})
	if err != nil {
		writeControlError(w, conflictControlError("tool_source_state_conflict"))
		return
	}
	if !enabled && control.grpc != nil {
		control.grpc.InvalidateSource(uuidString(sourceID))
	}
	writeControlJSON(w, http.StatusOK, sourceResponse(source, nil, nil))
}

func (control *ControlPlane) DeleteSource(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	sourceID, err := parseControlUUID(chi.URLParam(r, "sourceId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	tx, err := control.pool.Begin(r.Context())
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	defer tx.Rollback(r.Context())
	q := lwdb.New(tx)
	retainers, err := q.CountToolSourceBundleRetainers(r.Context(), lwdb.CountToolSourceBundleRetainersParams{WorkspaceID: workspaceID, SourceID: sourceID})
	if err != nil || retainers > 0 {
		writeControlError(w, conflictControlError("tool_source_retained"))
		return
	}
	if _, err := q.ClearToolSourceCurrentRevision(r.Context(), lwdb.ClearToolSourceCurrentRevisionParams{SourceID: sourceID, WorkspaceID: workspaceID}); err != nil {
		writeControlError(w, notFoundControlError())
		return
	}
	cleanup := []func() error{
		func() error {
			_, err := q.DeleteToolDefinitionsBySource(r.Context(), lwdb.DeleteToolDefinitionsBySourceParams{WorkspaceID: workspaceID, SourceID: sourceID})
			return err
		},
		func() error {
			_, err := q.DeleteToolSourceRevisionsBySource(r.Context(), lwdb.DeleteToolSourceRevisionsBySourceParams{WorkspaceID: workspaceID, SourceID: sourceID})
			return err
		},
		func() error {
			_, err := q.DeleteToolSourceArtifactsBySource(r.Context(), lwdb.DeleteToolSourceArtifactsBySourceParams{WorkspaceID: workspaceID, SourceID: sourceID})
			return err
		},
		func() error {
			_, err := q.DeleteToolSourceSecretsBySource(r.Context(), lwdb.DeleteToolSourceSecretsBySourceParams{WorkspaceID: workspaceID, SourceID: sourceID})
			return err
		},
		func() error {
			_, err := q.DeleteToolSource(r.Context(), lwdb.DeleteToolSourceParams{SourceID: sourceID, WorkspaceID: workspaceID})
			return err
		},
	}
	for _, step := range cleanup {
		if err := step(); err != nil {
			writeControlError(w, conflictControlError("tool_source_retained"))
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	if control.grpc != nil {
		control.grpc.InvalidateSource(uuidString(sourceID))
	}
	w.WriteHeader(http.StatusNoContent)
}

func (control *ControlPlane) ListSourceTools(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	sourceID, err := parseControlUUID(chi.URLParam(r, "sourceId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	source, err := control.q.GetToolSource(r.Context(), lwdb.GetToolSourceParams{ID: sourceID, WorkspaceID: workspaceID})
	if err != nil || !source.CurrentRevision.Valid {
		writeControlError(w, notFoundControlError())
		return
	}
	definitions, err := control.q.ListToolDefinitionsByRevision(r.Context(), lwdb.ListToolDefinitionsByRevisionParams{
		WorkspaceID: workspaceID, SourceID: sourceID, SourceRevisionID: source.CurrentRevision,
	})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	response := make([]toolDTO, 0, len(definitions))
	for _, definition := range definitions {
		response = append(response, toolResponse(definition))
	}
	writeControlJSON(w, http.StatusOK, response)
}

func (control *ControlPlane) ImportSource(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, control.maxArtifactBytes+maxControlBodyBytes)
	if err := r.ParseMultipartForm(maxControlBodyBytes); err != nil || r.MultipartForm == nil {
		writeControlError(w, invalidControlError("tool_source_artifact_invalid"))
		return
	}
	defer r.MultipartForm.RemoveAll()
	allowedFields := map[string]bool{
		"name": true, "kind": true, "endpoint": true, "transport_config": true,
		"auth": true, "media_type": true, "source_id": true,
	}
	for field := range r.MultipartForm.Value {
		if !allowedFields[field] || len(r.MultipartForm.Value[field]) != 1 {
			writeControlError(w, invalidControlError("tool_source_invalid"))
			return
		}
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["file"]) != 1 {
		writeControlError(w, invalidControlError("tool_source_artifact_invalid"))
		return
	}
	name := normalizeSourceName(r.FormValue("name"))
	kind := strings.ToLower(strings.TrimSpace(r.FormValue("kind")))
	if name == "" || (kind != "openapi" && kind != "grpc") {
		writeControlError(w, invalidControlError("tool_source_invalid"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeControlError(w, invalidControlError("tool_source_artifact_invalid"))
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, control.maxArtifactBytes+1))
	if err != nil || len(content) == 0 || int64(len(content)) > control.maxArtifactBytes {
		writeControlError(w, invalidControlError("tool_source_artifact_invalid"))
		return
	}
	mediaType := importArtifactMediaType(kind, r.FormValue("media_type"), header)
	if mediaType == "" {
		writeControlError(w, invalidControlError("tool_source_artifact_invalid"))
		return
	}
	transportConfig := json.RawMessage(strings.TrimSpace(r.FormValue("transport_config")))
	if len(transportConfig) == 0 {
		transportConfig = json.RawMessage(`{}`)
	}
	if !jsonObject(transportConfig) {
		writeControlError(w, invalidControlError("tool_source_invalid"))
		return
	}
	auth := json.RawMessage(strings.TrimSpace(r.FormValue("auth")))
	if len(auth) > 0 && !jsonObject(auth) {
		writeControlError(w, invalidControlError("tool_source_secret_invalid"))
		return
	}
	var endpoint *string
	if value := strings.TrimSpace(r.FormValue("endpoint")); value != "" {
		endpoint = &value
	}
	var existingSourceID pgtype.UUID
	if value := strings.TrimSpace(r.FormValue("source_id")); value != "" {
		existingSourceID, err = parseControlUUID(value)
		if err != nil {
			writeControlError(w, err)
			return
		}
	}

	validationCtx, cancelValidation := context.WithTimeout(r.Context(), control.validationTimeout)
	defer cancelValidation()
	releaseValidation, err := control.validationWorkers.acquire(validationCtx, "source-import")
	if err != nil {
		writeControlError(w, conflictControlError("tool_source_validation_limited"))
		return
	}
	defer releaseValidation()
	tx, err := control.pool.Begin(validationCtx)
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	defer tx.Rollback(validationCtx)
	q := lwdb.New(tx)
	var source lwdb.ToolSource
	if existingSourceID.Valid {
		source, err = q.GetToolSource(validationCtx, lwdb.GetToolSourceParams{
			ID: existingSourceID, WorkspaceID: workspaceID,
		})
		if err != nil {
			writeControlError(w, notFoundControlError())
			return
		}
		if source.Name != name || source.Kind != kind {
			writeControlError(w, conflictControlError("tool_source_conflict"))
			return
		}
	} else {
		source, err = q.CreateToolSource(validationCtx, lwdb.CreateToolSourceParams{
			WorkspaceID: workspaceID,
			Name:        name,
			Kind:        kind,
			CreatedBy:   actorID,
		})
		if err != nil {
			writeControlError(w, conflictControlError("tool_source_conflict"))
			return
		}
	}
	digest := sha256.Sum256(content)
	artifact, err := q.CreateToolSourceArtifact(validationCtx, lwdb.CreateToolSourceArtifactParams{
		WorkspaceID: workspaceID,
		SourceID:    source.ID,
		Sha256:      hex.EncodeToString(digest[:]),
		MediaType:   mediaType,
		SizeBytes:   int64(len(content)),
		Content:     content,
	})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	artifactID := uuidString(artifact.ID)
	revision, secret, err := control.stageRevision(validationCtx, q, source, actorID, updateToolSourceRequest{
		Endpoint:        endpoint,
		TransportConfig: transportConfig,
		ArtifactID:      &artifactID,
		Auth:            auth,
	})
	if err != nil {
		writeControlError(w, err)
		return
	}
	validationCode, err := control.discoverSourceRevision(validationCtx, q, source, revision)
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	if validationCode != "" {
		control.observer.RecordSourceValidation(kind, validationCode)
		writeControlError(w, invalidControlError(validationCode))
		return
	}
	ready, err := q.MarkToolSourceRevisionReady(validationCtx, lwdb.MarkToolSourceRevisionReadyParams{
		ID: revision.ID, WorkspaceID: workspaceID, SourceID: source.ID,
	})
	if err != nil {
		writeControlError(w, conflictControlError("tool_source_revision_conflict"))
		return
	}
	updated, err := q.AdvanceToolSourceCurrentRevision(validationCtx, lwdb.AdvanceToolSourceCurrentRevisionParams{
		SourceID: source.ID, WorkspaceID: workspaceID, RevisionID: revision.ID,
	})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	definitions, err := q.ListToolDefinitionsByRevision(validationCtx, lwdb.ListToolDefinitionsByRevisionParams{
		WorkspaceID: workspaceID, SourceID: source.ID, SourceRevisionID: revision.ID,
	})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	if err := tx.Commit(validationCtx); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	response := importToolSourceResponse{
		Source: sourceResponse(updated, &ready, secret),
		Tools:  make([]toolDTO, 0, len(definitions)),
	}
	response.Artifact.ID = artifactID
	response.Artifact.SHA256 = artifact.Sha256
	response.Artifact.MediaType = artifact.MediaType
	response.Artifact.SizeBytes = artifact.SizeBytes
	for _, definition := range definitions {
		response.Tools = append(response.Tools, toolResponse(definition))
	}
	control.observer.RecordSourceValidation(kind, "ready")
	control.recordControlActivity(r.Context(), workspaceID, actorID, "mcp_gateway_source_imported", map[string]any{
		"source_id": uuidString(source.ID), "source_revision_id": uuidString(revision.ID),
		"source_kind": kind, "item_count": len(definitions), "outcome": "ready",
	})
	writeControlJSON(w, http.StatusCreated, response)
}

func importArtifactMediaType(kind, requested string, header *multipart.FileHeader) string {
	mediaType := strings.TrimSpace(strings.Split(requested, ";")[0])
	if mediaType == "" && header != nil {
		mediaType = strings.TrimSpace(strings.Split(header.Header.Get("Content-Type"), ";")[0])
	}
	filename := ""
	if header != nil {
		filename = strings.ToLower(strings.TrimSpace(header.Filename))
	}
	if kind == "openapi" {
		if mediaType == "" || mediaType == "application/octet-stream" {
			if strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml") {
				return "application/yaml"
			}
			return "application/json"
		}
		return mediaType
	}
	if mediaType == "" || mediaType == "application/octet-stream" || mediaType == "text/plain" {
		switch {
		case strings.HasSuffix(filename, ".proto"):
			return "text/x-proto"
		case strings.HasSuffix(filename, ".zip"):
			return "application/zip"
		default:
			return "application/x-protobuf-descriptor-set"
		}
	}
	return mediaType
}

func (control *ControlPlane) UploadSourceArtifact(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	sourceID, err := parseControlUUID(chi.URLParam(r, "sourceId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	if _, err := control.q.GetToolSource(r.Context(), lwdb.GetToolSourceParams{ID: sourceID, WorkspaceID: workspaceID}); err != nil {
		writeControlError(w, notFoundControlError())
		return
	}
	var request uploadToolSourceArtifactRequest
	if err := decodeControlBody(r, &request); err != nil {
		writeControlError(w, invalidControlError("tool_source_artifact_invalid"))
		return
	}
	content, err := base64.StdEncoding.DecodeString(request.ContentBase64)
	if err != nil || len(content) == 0 || int64(len(content)) > control.maxArtifactBytes || strings.TrimSpace(request.MediaType) == "" {
		writeControlError(w, invalidControlError("tool_source_artifact_invalid"))
		return
	}
	digest := sha256.Sum256(content)
	artifact, err := control.q.CreateToolSourceArtifact(r.Context(), lwdb.CreateToolSourceArtifactParams{
		WorkspaceID: workspaceID, SourceID: sourceID, Sha256: hex.EncodeToString(digest[:]),
		MediaType: strings.TrimSpace(request.MediaType), SizeBytes: int64(len(content)), Content: content,
	})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	writeControlJSON(w, http.StatusCreated, map[string]any{
		"id": uuidString(artifact.ID), "sha256": artifact.Sha256,
		"media_type": artifact.MediaType, "size_bytes": artifact.SizeBytes,
	})
}

func (control *ControlPlane) GetBundle(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	bundleID := strings.TrimSpace(chi.URLParam(r, "bundleId"))
	if !ValidBundleID(bundleID) {
		writeControlError(w, notFoundControlError())
		return
	}
	bundle, err := control.q.GetToolBundle(r.Context(), lwdb.GetToolBundleParams{ID: bundleID, WorkspaceID: workspaceID})
	if err != nil {
		writeControlError(w, notFoundControlError())
		return
	}
	items, err := control.q.ListToolBundleItems(r.Context(), lwdb.ListToolBundleItemsParams{WorkspaceID: workspaceID, BundleID: bundleID})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	writeControlJSON(w, http.StatusOK, bundleResponse(bundle, items))
}

func (control *ControlPlane) GetAgentBundle(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	agentID, err := parseControlUUID(chi.URLParam(r, "agentId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	if _, err := control.q.GetAgent(r.Context(), lwdb.GetAgentParams{ID: agentID, WorkspaceID: workspaceID}); err != nil {
		writeControlError(w, notFoundControlError())
		return
	}
	head, err := control.q.GetAgentToolBundleHead(r.Context(), lwdb.GetAgentToolBundleHeadParams{
		WorkspaceID: workspaceID,
		AgentID:     agentID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeControlJSON(w, http.StatusOK, map[string]any{"bundle": nil})
		return
	}
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	bundle, err := control.q.GetToolBundle(r.Context(), lwdb.GetToolBundleParams{ID: head.BundleID, WorkspaceID: workspaceID})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	items, err := control.q.ListToolBundleItems(r.Context(), lwdb.ListToolBundleItemsParams{WorkspaceID: workspaceID, BundleID: bundle.ID})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	writeControlJSON(w, http.StatusOK, map[string]any{"bundle": bundleResponse(bundle, items)})
}

func (control *ControlPlane) ClearAgentBundle(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	agentID, err := parseControlUUID(chi.URLParam(r, "agentId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	tx, err := control.pool.Begin(r.Context())
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	defer tx.Rollback(r.Context())
	q := lwdb.New(tx)
	if _, err := q.LockAgent(r.Context(), lwdb.LockAgentParams{ID: agentID, WorkspaceID: workspaceID}); err != nil {
		writeControlError(w, notFoundControlError())
		return
	}
	if _, err := q.DeleteAgentToolBundleHead(r.Context(), lwdb.DeleteAgentToolBundleHeadParams{
		WorkspaceID: workspaceID,
		AgentID:     agentID,
	}); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	control.observer.RecordBundlePublication("cleared", 0)
	control.recordControlActivity(r.Context(), workspaceID, actorID, "mcp_gateway_bundle_head_cleared", map[string]any{
		"agent_id": uuidString(agentID), "item_count": 0, "outcome": "cleared",
	})
	writeControlJSON(w, http.StatusOK, map[string]any{"bundle": nil})
}

func (control *ControlPlane) PublishAgentBundle(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	agentID, err := parseControlUUID(chi.URLParam(r, "agentId"))
	if err != nil {
		writeControlError(w, err)
		return
	}
	var request publishToolBundleRequest
	if err := decodeControlBody(r, &request); err != nil {
		writeControlError(w, invalidControlError("tool_bundle_invalid"))
		return
	}
	tx, err := control.pool.Begin(r.Context())
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	defer tx.Rollback(r.Context())
	q := lwdb.New(tx)
	agent, err := q.LockAgent(r.Context(), lwdb.LockAgentParams{ID: agentID, WorkspaceID: workspaceID})
	if err != nil || agent.ArchivedAt.Valid {
		writeControlError(w, notFoundControlError())
		return
	}
	if len(request.Items) == 0 {
		writeControlError(w, invalidControlError("tool_bundle_invalid"))
		return
	}
	if !agent.RuntimeID.Valid {
		writeControlError(w, conflictControlError("provider_mcp_unsupported"))
		return
	}
	runtime, err := q.GetAgentRuntime(r.Context(), lwdb.GetAgentRuntimeParams{ID: agent.RuntimeID, WorkspaceID: workspaceID})
	if err != nil || !control.providerAllowlist[strings.ToLower(runtime.Provider)] {
		writeControlError(w, conflictControlError("provider_mcp_unsupported"))
		return
	}
	publication, err := control.buildBundlePublication(r.Context(), q, workspaceID, request.Items)
	if err != nil {
		writeControlError(w, err)
		return
	}
	if head, headErr := q.GetAgentToolBundleHead(r.Context(), lwdb.GetAgentToolBundleHeadParams{WorkspaceID: workspaceID, AgentID: agentID}); headErr == nil {
		if current, currentErr := q.GetToolBundle(r.Context(), lwdb.GetToolBundleParams{ID: head.BundleID, WorkspaceID: workspaceID}); currentErr == nil && current.Status == "active" && current.ManifestHash == publication.ManifestHash {
			items, _ := q.ListToolBundleItems(r.Context(), lwdb.ListToolBundleItemsParams{WorkspaceID: workspaceID, BundleID: current.ID})
			if err := tx.Commit(r.Context()); err != nil {
				writeControlError(w, internalControlError())
				return
			}
			control.observer.RecordBundlePublication("no_op", len(items))
			control.recordControlActivity(r.Context(), workspaceID, actorID, "mcp_gateway_bundle_published", map[string]any{
				"agent_id": uuidString(agentID), "bundle_id": current.ID,
				"item_count": len(items), "tool_names": bundleAuditNames(items), "outcome": "no_op",
			})
			writeControlJSON(w, http.StatusOK, bundleResponse(current, items))
			return
		}
	}
	bundleID := "tb_" + ulid.Make().String()
	bundle, err := q.CreateToolBundle(r.Context(), lwdb.CreateToolBundleParams{
		ID: bundleID, WorkspaceID: workspaceID, ManifestHash: publication.ManifestHash, CreatedBy: actorID,
	})
	if err != nil {
		writeControlError(w, internalControlError())
		return
	}
	items := make([]lwdb.ToolBundleItem, 0, len(publication.Items))
	for ordinal, item := range publication.Items {
		created, err := q.CreateToolBundleItem(r.Context(), lwdb.CreateToolBundleItemParams{
			WorkspaceID: workspaceID, BundleID: bundleID, Ordinal: int32(ordinal), PublicName: item.PublicName,
			SourceID: item.SourceID, SourceRevisionID: item.SourceRevisionID, ToolDefinitionID: item.ToolDefinitionID,
			ArtifactID: item.ArtifactID, DefinitionSnapshot: item.DefinitionSnapshot, InvocationPlan: item.InvocationPlan,
		})
		if err != nil {
			writeControlError(w, conflictControlError("tool_bundle_invalid"))
			return
		}
		items = append(items, created)
	}
	if _, err := q.UpsertAgentToolBundleHead(r.Context(), lwdb.UpsertAgentToolBundleHeadParams{
		WorkspaceID: workspaceID, AgentID: agentID, BundleID: bundleID, UpdatedBy: actorID,
	}); err != nil {
		writeControlError(w, conflictControlError("tool_bundle_invalid"))
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeControlError(w, internalControlError())
		return
	}
	control.observer.RecordBundlePublication("created", len(items))
	control.recordControlActivity(r.Context(), workspaceID, actorID, "mcp_gateway_bundle_published", map[string]any{
		"agent_id": uuidString(agentID), "bundle_id": bundle.ID,
		"item_count": len(items), "tool_names": bundleAuditNames(items), "outcome": "created",
	})
	writeControlJSON(w, http.StatusCreated, bundleResponse(bundle, items))
}

func (control *ControlPlane) RevokeBundle(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, err := controlIdentity(r)
	if err != nil {
		writeControlError(w, err)
		return
	}
	bundleID := strings.TrimSpace(chi.URLParam(r, "bundleId"))
	if !ValidBundleID(bundleID) {
		writeControlError(w, notFoundControlError())
		return
	}
	bundle, err := control.q.RevokeToolBundle(r.Context(), lwdb.RevokeToolBundleParams{ID: bundleID, WorkspaceID: workspaceID})
	if err != nil {
		writeControlError(w, conflictControlError("tool_bundle_revoked"))
		return
	}
	items, _ := control.q.ListToolBundleItems(r.Context(), lwdb.ListToolBundleItemsParams{WorkspaceID: workspaceID, BundleID: bundleID})
	control.observer.RecordBundlePublication("revoked", len(items))
	control.recordControlActivity(r.Context(), workspaceID, actorID, "mcp_gateway_bundle_revoked", map[string]any{
		"bundle_id": bundle.ID, "item_count": len(items), "outcome": "revoked",
	})
	writeControlJSON(w, http.StatusOK, bundleResponse(bundle, items))
}

func (control *ControlPlane) recordControlActivity(
	requestCtx context.Context,
	workspaceID pgtype.UUID,
	actorID pgtype.UUID,
	action string,
	details map[string]any,
) {
	encoded, err := json.Marshal(details)
	if err != nil {
		return
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), time.Second)
	defer cancel()
	_, _ = control.q.CreateActivity(auditCtx, lwdb.CreateActivityParams{
		WorkspaceID: workspaceID,
		ActorType:   "member",
		ActorID:     actorID,
		Action:      action,
		Details:     encoded,
	})
}

func (control *ControlPlane) stageRevision(ctx context.Context, q *lwdb.Queries, source lwdb.ToolSource, actorID pgtype.UUID, request updateToolSourceRequest) (lwdb.ToolSourceRevision, *lwdb.GetToolSourceSecretMetadataRow, error) {
	revisionID := newPGUUID()
	var secretID pgtype.UUID
	var secretMetadata *lwdb.GetToolSourceSecretMetadataRow
	var sourceAuthHeaders http.Header
	if len(request.Auth) > 0 && string(request.Auth) != "null" {
		if control.secretCodec == nil || !jsonObject(request.Auth) {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_secret_invalid")
		}
		var err error
		sourceAuthHeaders, err = parseSourceAuthHeaders(request.Auth)
		if err != nil {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_secret_invalid")
		}
		secretID = newPGUUID()
		envelope, err := control.secretCodec.Seal(SourceSecretScope{
			WorkspaceID: uuidString(source.WorkspaceID), SourceID: uuidString(source.ID), RevisionID: uuidString(revisionID),
		}, request.Auth)
		if err != nil {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_secret_invalid")
		}
		envelopeJSON, _ := json.Marshal(envelope)
		secret, err := q.CreateToolSourceSecret(ctx, lwdb.CreateToolSourceSecretParams{
			ID: secretID, WorkspaceID: source.WorkspaceID, SourceID: source.ID,
			Envelope: envelopeJSON, KeyID: control.secretCodec.ActiveKeyID(),
		})
		if err != nil {
			return lwdb.ToolSourceRevision{}, nil, internalControlError()
		}
		secretMetadata = &lwdb.GetToolSourceSecretMetadataRow{
			ID: secret.ID, WorkspaceID: secret.WorkspaceID, SourceID: secret.SourceID,
			KeyID: secret.KeyID, CreatedAt: secret.CreatedAt, UpdatedAt: secret.UpdatedAt,
		}
	}
	transportConfig := request.TransportConfig
	if len(transportConfig) == 0 {
		transportConfig = json.RawMessage(`{}`)
	}
	if !jsonObject(transportConfig) {
		return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_invalid")
	}
	artifactID, err := optionalControlUUID(request.ArtifactID)
	if err != nil {
		return lwdb.ToolSourceRevision{}, nil, err
	}
	if source.Kind == "grpc" {
		var descriptorConfig struct {
			BuiltinDescriptor string `json:"builtin_descriptor"`
		}
		if err := json.Unmarshal(transportConfig, &descriptorConfig); err != nil {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_invalid")
		}
		descriptorConfig.BuiltinDescriptor = strings.TrimSpace(descriptorConfig.BuiltinDescriptor)
		if descriptorConfig.BuiltinDescriptor != "" {
			if artifactID.Valid || control.grpc == nil {
				return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_artifact_invalid")
			}
			embedded, ok := control.grpc.EmbeddedArtifact(descriptorConfig.BuiltinDescriptor)
			if !ok {
				return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_artifact_invalid")
			}
			artifact, err := q.CreateToolSourceArtifact(ctx, lwdb.CreateToolSourceArtifactParams{
				WorkspaceID: source.WorkspaceID, SourceID: source.ID, Sha256: embedded.Sha256,
				MediaType: embedded.MediaType, SizeBytes: embedded.SizeBytes, Content: embedded.Content,
			})
			if err != nil {
				return lwdb.ToolSourceRevision{}, nil, internalControlError()
			}
			artifactID = artifact.ID
		}
	}
	if source.Kind == "openapi" && !artifactID.Valid {
		var documentConfig struct {
			DocumentURL string `json:"document_url"`
		}
		if json.Unmarshal(transportConfig, &documentConfig) != nil || strings.TrimSpace(documentConfig.DocumentURL) == "" || control.openAPI == nil {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_artifact_invalid")
		}
		content, mediaType, fetchErr := control.openAPI.FetchDocument(ctx, documentConfig.DocumentURL, sourceAuthHeaders)
		if fetchErr != nil {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError(openAPIValidationCode(fetchErr))
		}
		digest := sha256.Sum256(content)
		artifact, err := q.CreateToolSourceArtifact(ctx, lwdb.CreateToolSourceArtifactParams{
			WorkspaceID: source.WorkspaceID, SourceID: source.ID, Sha256: hex.EncodeToString(digest[:]),
			MediaType: mediaType, SizeBytes: int64(len(content)), Content: content,
		})
		if err != nil {
			return lwdb.ToolSourceRevision{}, nil, internalControlError()
		}
		artifactID = artifact.ID
	}
	if artifactID.Valid {
		if _, err := q.GetToolSourceArtifact(ctx, lwdb.GetToolSourceArtifactParams{ID: artifactID, WorkspaceID: source.WorkspaceID, SourceID: source.ID}); err != nil {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_artifact_invalid")
		}
	}
	endpoint, err := normalizeSourceEndpoint(request.Endpoint)
	if err != nil {
		return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_invalid")
	}
	revision, err := q.CreateToolSourceRevision(ctx, lwdb.CreateToolSourceRevisionParams{
		ID: revisionID, WorkspaceID: source.WorkspaceID, SourceID: source.ID,
		Endpoint: endpoint, TransportConfig: transportConfig,
		ArtifactID: artifactID, SecretID: secretID, CreatedBy: actorID,
	})
	if err != nil {
		return lwdb.ToolSourceRevision{}, nil, conflictControlError("tool_source_revision_conflict")
	}
	seen := make(map[string]struct{}, len(request.Tools))
	for _, tool := range request.Tools {
		tool.PublicName = strings.TrimSpace(tool.PublicName)
		tool.UpstreamName = strings.TrimSpace(tool.UpstreamName)
		if tool.PublicName == "" || tool.UpstreamName == "" || !jsonObject(tool.InputSchema) {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_invalid")
		}
		if _, duplicate := seen[tool.PublicName]; duplicate {
			return lwdb.ToolSourceRevision{}, nil, conflictControlError("tool_name_conflict")
		}
		seen[tool.PublicName] = struct{}{}
		operationMetadata := tool.OperationMetadata
		if len(operationMetadata) == 0 {
			operationMetadata = json.RawMessage(`{}`)
		}
		if !jsonObject(operationMetadata) || (len(tool.OutputSchema) > 0 && string(tool.OutputSchema) != "null" && !jsonObject(tool.OutputSchema)) {
			return lwdb.ToolSourceRevision{}, nil, invalidControlError("tool_source_invalid")
		}
		if _, err := q.CreateToolDefinition(ctx, lwdb.CreateToolDefinitionParams{
			WorkspaceID: source.WorkspaceID, SourceID: source.ID, SourceRevisionID: revision.ID,
			PublicName: tool.PublicName, UpstreamName: tool.UpstreamName, Description: strings.TrimSpace(tool.Description),
			InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema, OperationMetadata: operationMetadata,
		}); err != nil {
			return lwdb.ToolSourceRevision{}, nil, conflictControlError("tool_name_conflict")
		}
	}
	return revision, secretMetadata, nil
}

type bundlePublication struct {
	ManifestHash string
	Items        []bundlePublicationItem
}

type bundlePublicationItem struct {
	PublicName          string
	CanonicalPublicName string
	SourceID            pgtype.UUID
	SourceRevisionID    pgtype.UUID
	ToolDefinitionID    pgtype.UUID
	ArtifactID          pgtype.UUID
	DefinitionSnapshot  []byte
	InvocationPlan      []byte
}

func (control *ControlPlane) buildBundlePublication(ctx context.Context, q *lwdb.Queries, workspaceID pgtype.UUID, requestedItems []publishToolBundleItemRequest) (bundlePublication, error) {
	normalized := append([]publishToolBundleItemRequest(nil), requestedItems...)
	for index := range normalized {
		normalized[index].ToolDefinitionID = strings.TrimSpace(normalized[index].ToolDefinitionID)
		normalized[index].ExportedName = strings.TrimSpace(normalized[index].ExportedName)
	}
	sort.Slice(normalized, func(left, right int) bool {
		return normalized[left].ToolDefinitionID < normalized[right].ToolDefinitionID
	})
	items := make([]bundlePublicationItem, 0, len(normalized))
	manifest := make([]json.RawMessage, 0, len(normalized))
	seenIDs := make(map[string]struct{}, len(normalized))
	seenNames := make(map[string]struct{}, len(normalized))
	for _, requestedItem := range normalized {
		if !validExportedToolName(requestedItem.ExportedName) {
			return bundlePublication{}, invalidControlError("tool_bundle_invalid")
		}
		definitionID, err := parseControlUUID(requestedItem.ToolDefinitionID)
		if err != nil {
			return bundlePublication{}, invalidControlError("tool_bundle_invalid")
		}
		id := uuidString(definitionID)
		if _, duplicate := seenIDs[id]; duplicate {
			return bundlePublication{}, invalidControlError("tool_bundle_invalid")
		}
		seenIDs[id] = struct{}{}
		row, err := q.GetBundlePublicationToolDefinition(ctx, lwdb.GetBundlePublicationToolDefinitionParams{ID: definitionID, WorkspaceID: workspaceID})
		if err != nil {
			return bundlePublication{}, invalidControlError("tool_bundle_invalid")
		}
		if _, duplicate := seenNames[requestedItem.ExportedName]; duplicate {
			return bundlePublication{}, conflictControlError("tool_name_conflict")
		}
		seenNames[requestedItem.ExportedName] = struct{}{}
		definitionSnapshot, _ := json.Marshal(map[string]any{
			"name": requestedItem.ExportedName, "canonicalName": row.PublicName, "description": row.Description,
			"inputSchema": json.RawMessage(row.InputSchema), "outputSchema": nullableJSON(row.OutputSchema),
		})
		invocationPlan, _ := json.Marshal(map[string]any{
			"kind": row.SourceKind, "endpoint": nullableText(row.Endpoint),
			"upstreamName": row.UpstreamName, "operation": json.RawMessage(row.OperationMetadata),
			"transport": json.RawMessage(row.TransportConfig),
		})
		canonical, _ := json.Marshal(map[string]any{
			"exportedName": requestedItem.ExportedName, "canonicalPublicName": row.PublicName, "sourceId": uuidString(row.SourceID),
			"sourceRevisionId": uuidString(row.SourceRevisionID), "toolDefinitionId": uuidString(row.ID),
			"artifactId": nullableUUID(row.ArtifactID), "definition": json.RawMessage(definitionSnapshot),
			"invocationPlan": json.RawMessage(invocationPlan),
		})
		manifest = append(manifest, canonical)
		items = append(items, bundlePublicationItem{
			PublicName: requestedItem.ExportedName, CanonicalPublicName: row.PublicName,
			SourceID: row.SourceID, SourceRevisionID: row.SourceRevisionID,
			ToolDefinitionID: row.ID, ArtifactID: row.ArtifactID,
			DefinitionSnapshot: definitionSnapshot, InvocationPlan: invocationPlan,
		})
	}
	canonicalManifest, _ := json.Marshal(manifest)
	digest := sha256.Sum256(canonicalManifest)
	return bundlePublication{ManifestHash: hex.EncodeToString(digest[:]), Items: items}, nil
}

func validExportedToolName(name string) bool {
	if len(name) == 0 || len(name) > 128 || remoteToolNamePart.MatchString(name) {
		return false
	}
	first := name[0]
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || (first >= '0' && first <= '9')
}

func validateStagedRevision(source lwdb.ToolSource, revision lwdb.ToolSourceRevision, definitions []lwdb.ToolDefinition) string {
	if source.Kind != "server_local" && source.Kind != "remote_mcp" && source.Kind != "grpc" && source.Kind != "openapi" {
		return "tool_source_validator_unavailable"
	}
	if len(definitions) == 0 ||
		(source.Kind == "server_local" && (revision.Endpoint.Valid || revision.ArtifactID.Valid)) ||
		(source.Kind == "remote_mcp" && (!revision.Endpoint.Valid || revision.ArtifactID.Valid)) ||
		(source.Kind == "grpc" && (!revision.Endpoint.Valid || !revision.ArtifactID.Valid)) ||
		(source.Kind == "openapi" && (!revision.Endpoint.Valid || !revision.ArtifactID.Valid)) {
		return "tool_source_invalid"
	}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.PublicName) == "" || strings.TrimSpace(definition.UpstreamName) == "" ||
			!jsonObject(definition.InputSchema) || !jsonObject(definition.OperationMetadata) {
			return "tool_source_invalid"
		}
		var inputSchema jsonschema.Schema
		if err := json.Unmarshal(definition.InputSchema, &inputSchema); err != nil || inputSchema.Type != "object" {
			return "tool_source_invalid"
		}
		if _, err := inputSchema.Resolve(nil); err != nil {
			return "tool_source_invalid"
		}
		if source.Kind == "server_local" && !validServerLocalOperation(definition.OperationMetadata) {
			return "tool_source_invalid"
		}
		if source.Kind == "remote_mcp" {
			var operation struct {
				UpstreamName string `json:"upstream_name"`
			}
			expectedName, nameErr := remotePublicName(source.Name, definition.UpstreamName)
			if nameErr != nil || expectedName != definition.PublicName ||
				json.Unmarshal(definition.OperationMetadata, &operation) != nil || operation.UpstreamName != definition.UpstreamName {
				return "tool_source_invalid"
			}
		}
		if source.Kind == "grpc" {
			var operation grpcInvocationOperation
			part := strings.Trim(remoteToolNamePart.ReplaceAllString(strings.TrimPrefix(definition.UpstreamName, "/"), "_"), "_.-")
			expectedName, nameErr := remotePublicName(source.Name, part)
			if nameErr != nil || expectedName != definition.PublicName ||
				json.Unmarshal(definition.OperationMetadata, &operation) != nil || operation.FullMethod != definition.UpstreamName ||
				operation.InputType == "" || operation.OutputType == "" || len(operation.ArtifactSHA256) != 64 {
				return "tool_source_invalid"
			}
		}
		if source.Kind == "openapi" {
			var operation openAPIOperationPlan
			expectedName, nameErr := remotePublicName(source.Name, definition.UpstreamName)
			if nameErr != nil || expectedName != definition.PublicName ||
				json.Unmarshal(definition.OperationMetadata, &operation) != nil || operation.Method == "" ||
				operation.BaseURL == "" || !strings.HasPrefix(operation.Path, "/") {
				return "tool_source_invalid"
			}
		}
		if len(definition.OutputSchema) > 0 && string(definition.OutputSchema) != "null" {
			var outputSchema jsonschema.Schema
			if err := json.Unmarshal(definition.OutputSchema, &outputSchema); err != nil {
				return "tool_source_invalid"
			}
			if _, err := outputSchema.Resolve(nil); err != nil {
				return "tool_source_invalid"
			}
		}
	}
	return ""
}

func createDiscoveredToolDefinitions(
	ctx context.Context,
	q *lwdb.Queries,
	source lwdb.ToolSource,
	revision lwdb.ToolSourceRevision,
	tools []discoveredTool,
) error {
	for _, tool := range tools {
		if _, err := q.CreateToolDefinition(ctx, lwdb.CreateToolDefinitionParams{
			WorkspaceID: source.WorkspaceID, SourceID: source.ID, SourceRevisionID: revision.ID,
			PublicName: tool.PublicName, UpstreamName: tool.UpstreamName, Description: tool.Description,
			InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema, OperationMetadata: tool.OperationMetadata,
		}); err != nil {
			return err
		}
	}
	return nil
}

func remoteMCPValidationCode(err error) string {
	switch {
	case errors.Is(err, ErrEgressDenied):
		return "egress_forbidden"
	case errors.Is(err, ErrEgressResolve), errors.Is(err, ErrEgressConnect),
		errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "tool_source_unreachable"
	default:
		return "tool_source_invalid"
	}
}

func grpcValidationCode(err error) string {
	switch {
	case errors.Is(err, ErrEgressDenied):
		return "egress_forbidden"
	case errors.Is(err, ErrEgressResolve), errors.Is(err, ErrEgressConnect),
		errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "tool_source_unreachable"
	default:
		return "tool_source_invalid"
	}
}

func openAPIValidationCode(err error) string {
	switch {
	case errors.Is(err, ErrEgressDenied):
		return "egress_forbidden"
	case errors.Is(err, ErrEgressResolve), errors.Is(err, ErrEgressConnect),
		errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "tool_source_unreachable"
	default:
		return "tool_source_invalid"
	}
}

func sourceResponse(source lwdb.ToolSource, revision *lwdb.ToolSourceRevision, secret *lwdb.GetToolSourceSecretMetadataRow) sourceDTO {
	response := sourceDTO{
		ID: uuidString(source.ID), WorkspaceID: uuidString(source.WorkspaceID), Name: source.Name,
		Kind: source.Kind, Enabled: source.Enabled, CurrentRevision: uuidPointer(source.CurrentRevision),
		CreatedAt: timeString(source.CreatedAt), UpdatedAt: timeString(source.UpdatedAt),
	}
	if revision != nil {
		revisionResponse := revisionDTO{
			ID: uuidString(revision.ID), Revision: revision.Revision, Status: revision.Status,
			Endpoint: textPointer(revision.Endpoint), TransportConfig: copyJSON(revision.TransportConfig),
			ArtifactID: uuidPointer(revision.ArtifactID), SecretConfigured: revision.SecretID.Valid,
			ValidationCode: textPointer(revision.ValidationCode), CreatedAt: timeString(revision.CreatedAt),
			PublishedAt: timePointer(revision.PublishedAt),
		}
		if secret != nil {
			revisionResponse.SecretKeyID = &secret.KeyID
		}
		response.Revision = &revisionResponse
	}
	return response
}

func toolResponse(tool lwdb.ToolDefinition) toolDTO {
	return toolDTO{
		ID: uuidString(tool.ID), PublicName: tool.PublicName, UpstreamName: tool.UpstreamName,
		Description: tool.Description, InputSchema: copyJSON(tool.InputSchema), OutputSchema: copyJSON(tool.OutputSchema),
		OperationMetadata: copyJSON(tool.OperationMetadata), Enabled: tool.Enabled,
	}
}

func bundleResponse(bundle lwdb.ToolBundle, items []lwdb.ToolBundleItem) bundleDTO {
	response := bundleDTO{
		ID: bundle.ID, WorkspaceID: uuidString(bundle.WorkspaceID), ManifestHash: bundle.ManifestHash,
		Status: bundle.Status, CreatedAt: timeString(bundle.CreatedAt), RevokedAt: timePointer(bundle.RevokedAt),
		Items: make([]bundleItemDTO, 0, len(items)),
	}
	for _, item := range items {
		canonicalName := item.PublicName
		var snapshot bundleToolSnapshot
		if json.Unmarshal(item.DefinitionSnapshot, &snapshot) == nil && strings.TrimSpace(snapshot.CanonicalName) != "" {
			canonicalName = snapshot.CanonicalName
		}
		response.Items = append(response.Items, bundleItemDTO{
			Ordinal: item.Ordinal, ExportedName: item.PublicName, CanonicalPublicName: canonicalName, SourceID: uuidString(item.SourceID),
			SourceRevisionID: uuidString(item.SourceRevisionID), ToolDefinitionID: uuidString(item.ToolDefinitionID),
			Definition: copyJSON(item.DefinitionSnapshot),
		})
	}
	return response
}

func bundleAuditNames(items []lwdb.ToolBundleItem) []map[string]string {
	names := make([]map[string]string, 0, len(items))
	for _, item := range items {
		canonicalName := item.PublicName
		var snapshot bundleToolSnapshot
		if json.Unmarshal(item.DefinitionSnapshot, &snapshot) == nil && strings.TrimSpace(snapshot.CanonicalName) != "" {
			canonicalName = snapshot.CanonicalName
		}
		names = append(names, map[string]string{"exported_name": item.PublicName, "canonical_public_name": canonicalName})
	}
	return names
}

type controlError struct {
	Status int
	Code   string
}

func (err controlError) Error() string { return err.Code }

func invalidControlError(code string) error {
	return controlError{Status: http.StatusUnprocessableEntity, Code: code}
}
func conflictControlError(code string) error {
	return controlError{Status: http.StatusConflict, Code: code}
}
func notFoundControlError() error {
	return controlError{Status: http.StatusNotFound, Code: "not_found"}
}
func internalControlError() error {
	return controlError{Status: http.StatusInternalServerError, Code: "internal_error"}
}

func writeControlError(w http.ResponseWriter, err error) {
	var typed controlError
	if !errors.As(err, &typed) {
		typed = controlError{Status: http.StatusInternalServerError, Code: "internal_error"}
	}
	writeControlJSON(w, typed.Status, map[string]string{"code": typed.Code})
}

func writeControlJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func decodeControlBody(r *http.Request, target any) error {
	reader := io.LimitReader(r.Body, maxControlBodyBytes+1)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func controlIdentity(r *http.Request) (pgtype.UUID, pgtype.UUID, error) {
	workspaceID, err := parseControlUUID(r.Header.Get("X-Workspace-ID"))
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, controlError{Status: http.StatusUnauthorized, Code: "unauthenticated"}
	}
	actorID, err := parseControlUUID(r.Header.Get("X-User-ID"))
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, controlError{Status: http.StatusUnauthorized, Code: "unauthenticated"}
	}
	return workspaceID, actorID, nil
}

func parseControlUUID(value string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return pgtype.UUID{}, invalidControlError("invalid_argument")
	}
	return pgtype.UUID{Bytes: parsed, Valid: true}, nil
}

func optionalControlUUID(value *string) (pgtype.UUID, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.UUID{}, nil
	}
	return parseControlUUID(*value)
}

func newPGUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}

func normalizeSourceName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	lastSeparator := false
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			result.WriteRune(character)
			lastSeparator = false
		} else if result.Len() > 0 && !lastSeparator {
			result.WriteByte('-')
			lastSeparator = true
		}
	}
	return strings.Trim(result.String(), "-")
}

func validSourceKind(kind string) bool {
	switch kind {
	case "server_local", "remote_mcp", "grpc", "openapi":
		return true
	default:
		return false
	}
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	trimmed := strings.TrimSpace(*value)
	return pgtype.Text{String: trimmed, Valid: trimmed != ""}
}

func normalizeSourceEndpoint(value *string) (pgtype.Text, error) {
	endpoint := optionalText(value)
	if !endpoint.Valid {
		return endpoint, nil
	}
	parsed, err := url.Parse(endpoint.String)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return pgtype.Text{}, errors.New("endpoint must be an absolute credential-free URL")
	}
	return endpoint, nil
}

func jsonObject(value []byte) bool {
	var decoded map[string]any
	return len(value) > 0 && json.Unmarshal(value, &decoded) == nil && decoded != nil
}

func copyJSON(value []byte) json.RawMessage {
	if len(value) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}

func nullableJSON(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return json.RawMessage(value)
}

func nullableText(value pgtype.Text) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullableUUID(value pgtype.UUID) any {
	if !value.Valid {
		return nil
	}
	return uuidString(value)
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func uuidPointer(value pgtype.UUID) *string {
	if !value.Valid {
		return nil
	}
	formatted := uuidString(value)
	return &formatted
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timeString(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func timePointer(value pgtype.Timestamptz) *string {
	if !value.Valid {
		return nil
	}
	formatted := timeString(value)
	return &formatted
}
