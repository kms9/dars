package agentconfigsecret

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kms9/dars/internal/util/secretbox"
)

func TestFacadeRoundTripsExistingDocuments(t *testing.T) {
	codec, err := New("primary", map[string][]byte{"primary": bytes.Repeat([]byte{5}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	encryptedEnv, err := codec.EncryptCustomEnv(map[string]string{"API_TOKEN": "sentinel-token"})
	if err != nil {
		t.Fatal(err)
	}
	decryptedEnv, err := codec.DecryptCustomEnv(encryptedEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decryptedEnv, map[string]string{"API_TOKEN": "sentinel-token"}) {
		t.Fatalf("custom env = %#v", decryptedEnv)
	}
	redacted, err := RedactedCustomEnv(encryptedEnv)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(redacted, map[string]string{"API_TOKEN": "****"}) || bytes.Contains(encryptedEnv, []byte("sentinel-token")) {
		t.Fatalf("redaction failed: redacted=%#v envelope=%s", redacted, encryptedEnv)
	}

	mcpDocument := json.RawMessage(`{"mcpServers":{"existing":{"url":"https://example.test","headers":{"Authorization":"sentinel-token"}}}}`)
	encryptedMCP, err := codec.EncryptMCPConfig(mcpDocument)
	if err != nil {
		t.Fatal(err)
	}
	decryptedMCP, err := codec.DecryptMCPConfig(encryptedMCP)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(decryptedMCP) || !bytes.Contains(decryptedMCP, []byte("sentinel-token")) {
		t.Fatalf("decrypted MCP document = %s", decryptedMCP)
	}
	if bytes.Contains(encryptedMCP, []byte("sentinel-token")) {
		t.Fatalf("plaintext leaked into MCP envelope: %s", encryptedMCP)
	}
}

func TestFacadeKeepsVersionedEnvelopeShape(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	codec, err := New("primary", map[string][]byte{"primary": key})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := codec.Seal([]byte("value"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"v", "kid", "nonce", "ciphertext"} {
		if _, ok := fields[field]; !ok {
			t.Errorf("missing envelope field %q: %s", field, encoded)
		}
	}

	// A pre-extraction envelope used AES-GCM with nil associated data. Prove
	// that the preserved facade can still read that exact historical shape.
	box, err := secretbox.New(key)
	if err != nil {
		t.Fatal(err)
	}
	nonce, ciphertext, err := box.SealDetached([]byte("historical-value"))
	if err != nil {
		t.Fatal(err)
	}
	historical := Envelope{
		V:          EnvelopeVersion,
		KID:        "primary",
		Nonce:      base64.RawStdEncoding.EncodeToString(nonce),
		Ciphertext: base64.RawStdEncoding.EncodeToString(ciphertext),
	}
	plaintext, err := codec.Open(historical)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "historical-value" {
		t.Fatalf("historical plaintext = %q", plaintext)
	}
}
