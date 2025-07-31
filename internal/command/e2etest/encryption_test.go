// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/opentofu/opentofu/internal/e2e"
)

type tofuResult struct {
	t *testing.T

	stdout string
	stderr string
	err    error
}

func (r tofuResult) Success() tofuResult {
	if r.stderr != "" {
		debug.PrintStack()
		r.t.Fatalf("unexpected stderr output:\n%s", r.stderr)
	}
	if r.err != nil {
		debug.PrintStack()
		r.t.Fatalf("unexpected error: %s", r.err)
	}

	return r
}

func (r tofuResult) Failure() tofuResult {
	if r.err == nil {
		debug.PrintStack()
		r.t.Fatal("expected error")
	}
	return r
}

func SanitizeStderr(msg string) string {
	// ANSI escape sequence regex for removing terminal color codes and control characters
	msg = stripAnsi(msg)
	// Pipe and carriage return replacement in order to correctly sanitze the stderr output
	msg = strings.ReplaceAll(
		strings.ReplaceAll(msg, "│", ""),
		"\n", "",
	)
	return msg
}

func (r tofuResult) StderrContains(msg string) tofuResult {
	stdErrSanitized := SanitizeStderr(r.stderr)
	if !strings.Contains(stdErrSanitized, msg) {
		debug.PrintStack()
		r.t.Fatalf("expected stderr output %q:\n%s", msg, stdErrSanitized)
	}
	return r
}

func (r tofuResult) Contains(msg string) tofuResult {
	if !strings.Contains(r.stdout, msg) {
		debug.PrintStack()
		r.t.Fatalf("expected output %q:\n%s", msg, r.stdout)
	}
	return r
}

// This test covers the scenario where a user migrates an existing project
// to having encryption enabled, uses it, then migrates back to encryption
// disabled
func TestEncryptionFlow(t *testing.T) {

	// This test reaches out to registry.opentofu.org to download the
	// mock provider, so it can only run if network access is allowed
	skipIfCannotAccessNetwork(t)

	// There is a lot of setup / helpers defined.  Actual test logic is below.

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

	run := func(args ...string) tofuResult {
		stdout, stderr, err := tf.Run(args...)
		return tofuResult{t, stdout, stderr, err}
	}
	apply := func(args ...string) tofuResult {
		iter += 1
		finalArgs := []string{"apply"}
		finalArgs = append(finalArgs, fmt.Sprintf("-var=iter=%v", iter), "-auto-approve")
		finalArgs = append(finalArgs, args...)
		return run(finalArgs...)
	}

	createPlan := func(planfile string, args ...string) tofuResult {
		iter += 1
		args = append([]string{"plan", fmt.Sprintf("-var=iter=%v", iter), "-out=" + planfile}, args...)
		return run(args...)
	}
	applyPlan := func(planfile string, args ...string) tofuResult {
		finalArgs := []string{"apply", "-auto-approve"}
		finalArgs = append(finalArgs, args...)
		finalArgs = append(finalArgs, planfile)
		return run(finalArgs...)
	}
	withVarArg := func(key, value string) string {
		return fmt.Sprintf(`-var=%s=%s`, key, value)
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

	with := func(path string, fn func()) {
		src := tf.Path(path + ".disabled")
		dst := tf.Path(path)

		err := os.Rename(src, dst)
		if err != nil {
			t.Fatalf("%s", err.Error())
		}

		fn()

		err = os.Rename(dst, src)
		if err != nil {
			t.Fatalf("%s", err.Error())
		}
	}

	// Actual test begins HERE
	// NOTE: state plans are still readable and tests the encryption state

	unencryptedPlan := "unencrypted.tfplan"
	encryptedPlan := "encrypted.tfplan"
	correctPassphrase := uuid.NewString()
	{
		// Everything works before adding encryption configuration
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireUnencryptedState()
		// Check read/write of state file
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireUnencryptedState()

		// Save an unencrypted plan
		createPlan(unencryptedPlan, withVarArg("passphrase", correctPassphrase)).Success()
		// Validate that OpenTofu does not allow different -var value for a variable between creation of the plan and its execution.
		applyPlan(unencryptedPlan, withVarArg("passphrase", "different-value-than-the-one-saved-in-the-planfile")).
			StderrContains(`Value saved in the plan file for variable "passphrase" is different from the one given to the current command`)
		// Validate unencrypted plan
		applyPlan(unencryptedPlan, withVarArg("passphrase", correctPassphrase)).Success()
		requireUnencryptedState()
	}

	with("required.tf", func() {
		// Can't switch directly to encryption, need to migrate
		apply(withVarArg("passphrase", correctPassphrase)).Failure().StderrContains("encountered unencrypted payload without unencrypted method")
		requireUnencryptedState()
	})

	with("migrateto.tf", func() {
		// Migrate to using encryption
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireEncryptedState()
		// Make changes and confirm it's still encrypted (even with migration enabled)
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireEncryptedState()

		// Save an encrypted plan
		createPlan(encryptedPlan, withVarArg("passphrase", correctPassphrase)).Success()

		// Apply encrypted plan (with migration active)
		applyPlan(encryptedPlan, withVarArg("passphrase", correctPassphrase)).Success()
		requireEncryptedState()
		// Apply unencrypted plan (with migration active)
		applyPlan(unencryptedPlan, withVarArg("passphrase", correctPassphrase)).StderrContains("Saved plan is stale")
		requireEncryptedState()
	})

	{
		// Unconfigured encryption clearly fails on encrypted state
		apply(withVarArg("passphrase", correctPassphrase)).Failure().StderrContains("can not be read without an encryption configuration")
	}

	with("required.tf", func() {
		// Encryption works with fallback removed
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireEncryptedState()

		// Can't apply unencrypted plan
		applyPlan(unencryptedPlan, withVarArg("passphrase", correctPassphrase)).Failure().StderrContains("encountered unencrypted payload without unencrypted method")
		requireEncryptedState()

		// Apply encrypted plan
		applyPlan(encryptedPlan, withVarArg("passphrase", correctPassphrase)).StderrContains("Saved plan is stale")
		requireEncryptedState()
	})

	with("required.tf", func() { // But with the wrong passphrase
		incorrectPassphrase := uuid.NewString()
		// Make sure changes to encryption keys notify the user correctly
		apply(withVarArg("passphrase", incorrectPassphrase)).Failure().StderrContains("decryption failed for state")
		requireEncryptedState()

		applyPlan(encryptedPlan, withVarArg("passphrase", incorrectPassphrase)).Failure().StderrContains("decryption failed: cipher: message authentication failed")

		requireEncryptedState()
	})

	with("migratefrom.tf", func() {
		// Apply migration from encrypted state
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireUnencryptedState()
		// Make changes and confirm it's still encrypted (even with migration enabled)
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireUnencryptedState()

		// Apply unencrypted plan (with migration active)
		applyPlan(unencryptedPlan, withVarArg("passphrase", correctPassphrase)).StderrContains("Saved plan is stale")
		requireUnencryptedState()

		// Apply encrypted plan (with migration active)
		applyPlan(encryptedPlan, withVarArg("passphrase", correctPassphrase)).StderrContains("Saved plan is stale")
		requireUnencryptedState()
	})

	{
		// Back to no encryption configuration with unencrypted state
		apply(withVarArg("passphrase", correctPassphrase)).Success()
		requireUnencryptedState()

		// Apply unencrypted plan
		applyPlan(unencryptedPlan, withVarArg("passphrase", correctPassphrase)).StderrContains("Saved plan is stale")
		requireUnencryptedState()
		// Can't apply encrypted plan
		applyPlan(encryptedPlan, withVarArg("passphrase", correctPassphrase)).Failure().StderrContains("the given plan file is encrypted and requires a valid encryption")
		requireUnencryptedState()
	}
}
