// Package versionedsecret provides a reusable versioned AEAD envelope with
// key identifiers and optional associated data.
package versionedsecret

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/kms9/dars/internal/util/secretbox"
)

const EnvelopeVersion = 1

var (
	ErrUnsupportedEnvelope = errors.New("versioned secret: unsupported envelope")
	ErrUnknownKey          = errors.New("versioned secret: unknown key id")
	keyIDPattern           = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

type Envelope struct {
	V          int    `json:"v"`
	KID        string `json:"kid"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

// Codec is safe for concurrent use after construction.
type Codec struct {
	activeKID string
	boxes     map[string]*secretbox.Box
}

func New(activeKID string, keys map[string][]byte) (*Codec, error) {
	activeKID = strings.TrimSpace(activeKID)
	if !keyIDPattern.MatchString(activeKID) {
		return nil, fmt.Errorf("versioned secret: invalid active key id")
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("versioned secret: no keys configured")
	}
	boxes := make(map[string]*secretbox.Box, len(keys))
	for kid, key := range keys {
		if !keyIDPattern.MatchString(kid) {
			return nil, fmt.Errorf("versioned secret: invalid key id")
		}
		box, err := secretbox.New(key)
		if err != nil {
			return nil, fmt.Errorf("versioned secret: key %q: %w", kid, err)
		}
		boxes[kid] = box
	}
	if boxes[activeKID] == nil {
		return nil, fmt.Errorf("versioned secret: active key id is not configured")
	}
	return &Codec{activeKID: activeKID, boxes: boxes}, nil
}

func (codec *Codec) ActiveKeyID() string {
	return codec.activeKID
}

func (codec *Codec) Seal(plaintext, associatedData []byte) (Envelope, error) {
	box := codec.boxes[codec.activeKID]
	if box == nil {
		return Envelope{}, ErrUnknownKey
	}
	nonce, ciphertext, err := box.SealDetachedWithAAD(plaintext, associatedData)
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		V:          EnvelopeVersion,
		KID:        codec.activeKID,
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}, nil
}

func (codec *Codec) Open(envelope Envelope, associatedData []byte) ([]byte, error) {
	if envelope.V != EnvelopeVersion || !keyIDPattern.MatchString(envelope.KID) {
		return nil, ErrUnsupportedEnvelope
	}
	box := codec.boxes[envelope.KID]
	if box == nil {
		return nil, ErrUnknownKey
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, fmt.Errorf("versioned secret: invalid nonce: %w", err)
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("versioned secret: invalid ciphertext: %w", err)
	}
	plaintext, err := box.OpenDetachedWithAAD(nonce, ciphertext, associatedData)
	if err != nil {
		return nil, fmt.Errorf("versioned secret: decrypt: %w", err)
	}
	return plaintext, nil
}
