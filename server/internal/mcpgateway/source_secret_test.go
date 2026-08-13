package mcpgateway

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestSourceSecretCiphertextIsScopeBoundAndRedacted(t *testing.T) {
	codec, err := NewSourceSecretCodec("primary", map[string][]byte{"primary": bytes.Repeat([]byte{11}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	scope := SourceSecretScope{WorkspaceID: "workspace-a", SourceID: "source-a", RevisionID: "revision-a"}
	envelope, err := codec.Seal(scope, []byte("sentinel-upstream-credential"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("sentinel-upstream-credential")) {
		t.Fatalf("plaintext leaked into envelope: %s", encoded)
	}
	plaintext, err := codec.Open(scope, envelope)
	if err != nil {
		t.Fatal(err)
	}
	if string(plaintext) != "sentinel-upstream-credential" {
		t.Fatalf("plaintext = %q", plaintext)
	}

	wrongScopes := []SourceSecretScope{
		{WorkspaceID: "workspace-b", SourceID: "source-a", RevisionID: "revision-a"},
		{WorkspaceID: "workspace-a", SourceID: "source-b", RevisionID: "revision-a"},
		{WorkspaceID: "workspace-a", SourceID: "source-a", RevisionID: "revision-b"},
	}
	for _, wrongScope := range wrongScopes {
		_, err := codec.Open(wrongScope, envelope)
		if err == nil {
			t.Fatalf("scope swap unexpectedly decrypted: %+v", wrongScope)
		}
		if strings.Contains(err.Error(), "sentinel-upstream-credential") {
			t.Fatalf("decryption error leaked plaintext: %v", err)
		}
	}
	if codec.ActiveKeyID() != "primary" {
		t.Fatalf("active key ID = %q", codec.ActiveKeyID())
	}
}

func TestSourceSecretScopeMustBeComplete(t *testing.T) {
	codec, err := NewSourceSecretCodec("primary", map[string][]byte{"primary": bytes.Repeat([]byte{13}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codec.Seal(SourceSecretScope{WorkspaceID: "workspace"}, []byte("secret")); err == nil {
		t.Fatal("expected incomplete scope error")
	}
}
