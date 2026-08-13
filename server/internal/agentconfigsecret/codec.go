// Package agentconfigsecret encrypts Agent custom_env values independently and
// an entire mcp_config document as one authenticated envelope before either is
// persisted in PostgreSQL JSONB.
package agentconfigsecret

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/kms9/dars/internal/util/secretbox"
	"github.com/kms9/dars/internal/versionedsecret"
)

const (
	EnvelopeVersion = versionedsecret.EnvelopeVersion
	defaultKeyID    = "primary"
)

var (
	ErrUnsupportedEnvelope = versionedsecret.ErrUnsupportedEnvelope
	ErrUnknownKey          = versionedsecret.ErrUnknownKey
)

type Envelope = versionedsecret.Envelope

type Codec struct {
	inner *versionedsecret.Codec
}

func New(activeKID string, keys map[string][]byte) (*Codec, error) {
	inner, err := versionedsecret.New(activeKID, keys)
	if err != nil {
		return nil, err
	}
	return &Codec{inner: inner}, nil
}

func LoadFromEnv() (*Codec, error) {
	key, err := secretbox.LoadKey("DARS_AGENT_SECRET_KEY")
	if err != nil {
		return nil, err
	}
	kid := strings.TrimSpace(os.Getenv("DARS_AGENT_SECRET_KEY_ID"))
	if kid == "" {
		kid = defaultKeyID
	}
	return New(kid, map[string][]byte{kid: key})
}

func (codec *Codec) Seal(plaintext []byte) (Envelope, error) {
	return codec.inner.Seal(plaintext, nil)
}

func (codec *Codec) Open(envelope Envelope) ([]byte, error) {
	return codec.inner.Open(envelope, nil)
}

func (codec *Codec) EncryptCustomEnv(values map[string]string) ([]byte, error) {
	encrypted := make(map[string]Envelope, len(values))
	for key, value := range values {
		envelope, err := codec.Seal([]byte(value))
		if err != nil {
			return nil, fmt.Errorf("encrypt custom_env key %q: %w", key, err)
		}
		encrypted[key] = envelope
	}
	return json.Marshal(encrypted)
}

func (codec *Codec) DecryptCustomEnv(document []byte) (map[string]string, error) {
	var encrypted map[string]Envelope
	if err := decodeSingleJSON(document, &encrypted); err != nil {
		return nil, fmt.Errorf("decode custom_env envelope: %w", err)
	}
	if encrypted == nil {
		encrypted = map[string]Envelope{}
	}
	values := make(map[string]string, len(encrypted))
	for key, envelope := range encrypted {
		plaintext, err := codec.Open(envelope)
		if err != nil {
			return nil, fmt.Errorf("decrypt custom_env key %q: %w", key, err)
		}
		values[key] = string(plaintext)
	}
	return values, nil
}

func (codec *Codec) EncryptMCPConfig(document json.RawMessage) ([]byte, error) {
	var normalized any
	if err := decodeSingleJSON(document, &normalized); err != nil {
		return nil, fmt.Errorf("decode mcp_config: %w", err)
	}
	plaintext, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("normalize mcp_config: %w", err)
	}
	envelope, err := codec.Seal(plaintext)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func (codec *Codec) DecryptMCPConfig(document []byte) (json.RawMessage, error) {
	var envelope Envelope
	if err := decodeSingleJSON(document, &envelope); err != nil {
		return nil, fmt.Errorf("decode mcp_config envelope: %w", err)
	}
	plaintext, err := codec.Open(envelope)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := decodeSingleJSON(plaintext, &decoded); err != nil {
		return nil, fmt.Errorf("decrypted mcp_config is invalid JSON: %w", err)
	}
	return json.Marshal(decoded)
}

func RedactedCustomEnv(document []byte) (map[string]string, error) {
	var encrypted map[string]Envelope
	if err := decodeSingleJSON(document, &encrypted); err != nil {
		return nil, err
	}
	redacted := make(map[string]string, len(encrypted))
	for key := range encrypted {
		redacted[key] = "****"
	}
	return redacted, nil
}

func CustomEnvKeys(document []byte) ([]string, error) {
	var encrypted map[string]Envelope
	if err := decodeSingleJSON(document, &encrypted); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(encrypted))
	for key := range encrypted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func decodeSingleJSON(document []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(document)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("multiple JSON values")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
