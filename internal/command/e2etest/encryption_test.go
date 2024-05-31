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

func (r tofuResult) StderrContains(msg string) tofuResult {
	if !strings.Contains(r.stderr, msg) {
		debug.PrintStack()
		r.t.Fatalf("expected stderr output %q:\n%s", msg, r.stderr)
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
	apply := func() tofuResult {
		iter += 1
		return run("apply", fmt.Sprintf("-var=iter=%v", iter), "-auto-approve")
	}

	createPlan := func(planfile string) tofuResult {
		iter += 1
		return run("plan", fmt.Sprintf("-var=iter=%v", iter), "-out="+planfile)
	}
	applyPlan := func(planfile string) tofuResult {
		return run("apply", "-auto-approve", planfile)
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
			t.Fatalf(err.Error())
		}

		fn()

		err = os.Rename(dst, src)
		if err != nil {
			t.Fatalf(err.Error())
		}
	}

	// Actual test begins HERE
	// NOTE: state plans are still readable and tests the encryption state

	unencryptedPlan := "unencrypted.tfplan"
	encryptedPlan := "encrypted.tfplan"

	{
		// Everything works before adding encryption configuration
		apply().Success()
		requireUnencryptedState()
		// Check read/write of state file
		apply().Success()
		requireUnencryptedState()

		// Save an unencrypted plan
		createPlan(unencryptedPlan).Success()
		// Validate unencrypted plan
		applyPlan(unencryptedPlan).Success()
		requireUnencryptedState()
	}

	with("required.tf", func() {
		// Can't switch directly to encryption, need to migrate
		apply().Failure().StderrContains("encountered unencrypted payload without unencrypted method")
		requireUnencryptedState()
	})

	with("migrateto.tf", func() {
		// Migrate to using encryption
		apply().Success()
		requireEncryptedState()
		// Make changes and confirm it's still encrypted (even with migration enabled)
		apply().Success()
		requireEncryptedState()

		// Save an encrypted plan
		createPlan(encryptedPlan).Success()

		// Apply encrypted plan (with migration active)
		applyPlan(encryptedPlan).Success()
		requireEncryptedState()
		// Apply unencrypted plan (with migration active)
		applyPlan(unencryptedPlan).StderrContains("Saved plan is stale")
		requireEncryptedState()
	})

	{
		// Unconfigured encryption clearly fails on encrypted state
		apply().Failure().StderrContains("can not be read without an encryption configuration")
	}

	with("required.tf", func() {
		// Encryption works with fallback removed
		apply().Success()
		requireEncryptedState()

		// Can't apply unencrypted plan
		applyPlan(unencryptedPlan).Failure().StderrContains("encountered unencrypted payload without unencrypted method")
		requireEncryptedState()

		// Apply encrypted plan
		applyPlan(encryptedPlan).StderrContains("Saved plan is stale")
		requireEncryptedState()
	})

	with("broken.tf", func() {
		// Make sure changes to encryption keys notify the user correctly
		apply().Failure().StderrContains("decryption failed for state")
		requireEncryptedState()

		applyPlan(encryptedPlan).Failure().StderrContains("decryption failed: cipher: message authentication failed")
		requireEncryptedState()
	})

	with("migratefrom.tf", func() {
		// Apply migration from encrypted state
		apply().Success()
		requireUnencryptedState()
		// Make changes and confirm it's still encrypted (even with migration enabled)
		apply().Success()
		requireUnencryptedState()

		// Apply unencrypted plan (with migration active)
		applyPlan(unencryptedPlan).StderrContains("Saved plan is stale")
		requireUnencryptedState()

		// Apply encrypted plan (with migration active)
		applyPlan(encryptedPlan).StderrContains("Saved plan is stale")
		requireUnencryptedState()
	})

	{
		// Back to no encryption configuration with unencrypted state
		apply().Success()
		requireUnencryptedState()

		// Apply unencrypted plan
		applyPlan(unencryptedPlan).StderrContains("Saved plan is stale")
		requireUnencryptedState()
		// Can't apply encrypted plan
		applyPlan(encryptedPlan).Failure().StderrContains("the given plan file is encrypted and requires a valid encryption")
		requireUnencryptedState()
	}
}
