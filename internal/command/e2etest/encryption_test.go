package e2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

// This test covers the scenario where a user migrates an existing project
// to having encryption enabled, uses it, then migrates back to encryption
// disabled
func TestEncryptionStateFlow(t *testing.T) {

	// This test reaches out to registry.opentofu.org to download the
	// mock provider, so it can only run if network access is allowed
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "encryption-flow")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// tofu init
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr output:\n%s", stderr)
	}

	iter := 0

	apply := func() (stdout, stderr string, err error) {
		iter += 1
		return tf.Run("apply", fmt.Sprintf("-var=iter=%v", iter), "-auto-approve")
	}
	applySuccess := func() {
		_, stderr, err := apply()
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if stderr != "" {
			t.Fatalf("unexpected stderr output:\n%s", stderr)
		}
	}
	applyFailure := func(msg string) {
		_, stderr, err := apply()
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(stderr, msg) {
			t.Fatalf("expected stderr output %q:\n%s", msg, stderr)
		}
	}

	requireUnencryptedState := func() {
		_, err = tf.LocalState()
		if err != nil {
			t.Fatalf("expected unencrypted state file: %q", err)
		}
	}
	requireEncryptedState := func() {
		_, err = tf.LocalState()
		if err == nil || err.Error() != "Error reading statefile: Unsupported state file format: This state file is encrypted and can not be read without an encryption configuration" {
			t.Fatalf("expected encrypted state file: %q", err)
		}
	}

	enable := func(path string) {
		src := tf.Path(path + ".disabled")
		dst := tf.Path(path)
		err := os.Rename(src, dst)
		if err != nil {
			t.Fatalf(err.Error())
		}
	}
	disable := func(path string) {
		src := tf.Path(path)
		dst := tf.Path(path + ".disabled")
		err := os.Rename(src, dst)
		if err != nil {
			t.Fatalf(err.Error())
		}
	}

	with := func(path string, fn func()) {
		enable(path)
		fn()
		disable(path)
	}

	// Actual test begins HERE

	{
		// Everything works before adding encryption configuration
		applySuccess()
		requireUnencryptedState()
		// Check read/write of state file
		applySuccess()
		requireUnencryptedState()
	}

	with("required.tf", func() {
		// Can't switch directly to encryption, need to migrate
		applyFailure("decrypted payload provided without fallback specified")
		requireUnencryptedState()
	})

	with("migrateto.tf", func() {
		// Migrate to using encryption
		applySuccess()
		requireEncryptedState()
		// Make changes and confirm it's still encrypted (even with migration enabled)
		applySuccess()
		requireEncryptedState()
	})

	{
		// Unconfigured encryption clearly fails on encrypted state
		applyFailure("can not be read without an encryption configuration")
	}

	with("required.tf", func() {
		// Encryption works with fallback removed
		applySuccess()
		requireEncryptedState()
	})

	with("migratefrom.tf", func() {
		// Apply migration from encrypted state
		applySuccess()
		requireUnencryptedState()
		// Make changes and confirm it's still encrypted (even with migration enabled)
		applySuccess()
		requireUnencryptedState()
	})

	{
		// Back to no encryption configuration with unencrypted state
		applySuccess()
		requireUnencryptedState()
	}
}
