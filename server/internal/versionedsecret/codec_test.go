package versionedsecret

import (
	"bytes"
	"errors"
	"testing"
)

func TestCodecRoundTripAndAssociatedData(t *testing.T) {
	codec, err := New("primary", map[string][]byte{"primary": bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := codec.Seal([]byte("sentinel-secret"), []byte("scope-a"))
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := codec.Open(envelope, []byte("scope-a"))
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "sentinel-secret" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	if _, err := codec.Open(envelope, []byte("scope-b")); err == nil {
		t.Fatal("expected associated-data authentication failure")
	}
}

func TestCodecRejectsUnknownKeyAndEnvelopeVersion(t *testing.T) {
	codec, err := New("primary", map[string][]byte{"primary": bytes.Repeat([]byte{3}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codec.Open(Envelope{V: EnvelopeVersion, KID: "retired"}, nil); !errors.Is(err, ErrUnknownKey) {
		t.Fatalf("unknown key error = %v", err)
	}
	if _, err := codec.Open(Envelope{V: EnvelopeVersion + 1, KID: "primary"}, nil); !errors.Is(err, ErrUnsupportedEnvelope) {
		t.Fatalf("version error = %v", err)
	}
}
