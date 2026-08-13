package mcpgateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kms9/dars/internal/util/secretbox"
	"github.com/kms9/dars/internal/versionedsecret"
)

const sourceSecretDomain = "dars/tool-source-secret/v1"

// SourceSecretScope is authenticated but not encrypted. It binds ciphertext
// to one Workspace, Source, and immutable revision so rows cannot be swapped.
type SourceSecretScope struct {
	WorkspaceID string `json:"workspace_id"`
	SourceID    string `json:"source_id"`
	RevisionID  string `json:"revision_id"`
}

type SourceSecretCodec struct {
	inner *versionedsecret.Codec
}

func LoadSourceSecretCodecFromEnv() (*SourceSecretCodec, error) {
	key, err := secretbox.LoadKey("DARS_AGENT_SECRET_KEY")
	if err != nil {
		return nil, err
	}
	keyID := strings.TrimSpace(os.Getenv("DARS_AGENT_SECRET_KEY_ID"))
	if keyID == "" {
		keyID = "primary"
	}
	return NewSourceSecretCodec(keyID, map[string][]byte{keyID: key})
}

func NewSourceSecretCodec(activeKID string, keys map[string][]byte) (*SourceSecretCodec, error) {
	inner, err := versionedsecret.New(activeKID, keys)
	if err != nil {
		return nil, err
	}
	return &SourceSecretCodec{inner: inner}, nil
}

func (codec *SourceSecretCodec) ActiveKeyID() string {
	return codec.inner.ActiveKeyID()
}

func (codec *SourceSecretCodec) Seal(scope SourceSecretScope, plaintext []byte) (versionedsecret.Envelope, error) {
	associatedData, err := sourceSecretAssociatedData(scope)
	if err != nil {
		return versionedsecret.Envelope{}, err
	}
	return codec.inner.Seal(plaintext, associatedData)
}

func (codec *SourceSecretCodec) Open(scope SourceSecretScope, envelope versionedsecret.Envelope) ([]byte, error) {
	associatedData, err := sourceSecretAssociatedData(scope)
	if err != nil {
		return nil, err
	}
	return codec.inner.Open(envelope, associatedData)
}

func sourceSecretAssociatedData(scope SourceSecretScope) ([]byte, error) {
	if strings.TrimSpace(scope.WorkspaceID) == "" || strings.TrimSpace(scope.SourceID) == "" || strings.TrimSpace(scope.RevisionID) == "" {
		return nil, errors.New("tool source secret scope is incomplete")
	}
	encoded, err := json.Marshal(struct {
		Domain string            `json:"domain"`
		Scope  SourceSecretScope `json:"scope"`
	}{Domain: sourceSecretDomain, Scope: scope})
	if err != nil {
		return nil, fmt.Errorf("encode tool source secret scope: %w", err)
	}
	return encoded, nil
}
