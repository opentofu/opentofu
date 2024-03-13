package e2etest

import (
	"path/filepath"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

func TestEncryptionFlow(t *testing.T) {

	// This test reaches out to registry.opentofu.org to download the
	// template provider, so it can only run if network access is allowed.
	// We intentionally don't try to stub this here, because there's already
	// a stubbed version of this in the "command" package and so the goal here
	// is to test the interaction with the real repository.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "encryption-enabled")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// Setup the local encryption
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	_, stderr, err = tf.Run("apply", `-var=iter=first`, `-auto-approve`)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	_, err = tf.LocalState()
	if err == nil || err.Error() != "Error reading statefile: Unsupported state file format: This state file is encrypted and can not be read without an encryption configuration" {
		t.Fatalf("failed to read state file: %q", err.Error())
	}

	_, stderr, err = tf.Run("apply", `-var=iter=second`, `-auto-approve`)
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	_, err = tf.LocalState()
	if err == nil || err.Error() != "Error reading statefile: Unsupported state file format: This state file is encrypted and can not be read without an encryption configuration" {
		t.Fatalf("failed to read state file: %q", err.Error())
	}

}
