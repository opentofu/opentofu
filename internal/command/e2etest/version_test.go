// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/version"
)

func TestVersion(t *testing.T) {
	// Along with testing the "version" command in particular, this serves
	// as a good smoke test for whether the OpenTofu binary can even be
	// compiled and run, since it doesn't require any external network access
	// to do its job.

	t.Parallel()

	fixturePath := filepath.Join("testdata", "empty")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	stdout, stderr, err := tf.Run("version")
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	// Check for actual errors instead of just non-empty stderr
	if containsRealError(stderr) {
		t.Errorf("unexpected error or warning in stderr output:\n%s", stderr)
	}

	wantVersion := fmt.Sprintf("OpenTofu v%s", version.String())
	if !strings.Contains(stdout, wantVersion) {
		t.Errorf("output does not contain our current version %q:\n%s", wantVersion, stdout)
	}
}

func TestVersionWithProvider(t *testing.T) {
	// This is a more elaborate use of "version" that shows the selected
	// versions of plugins too.
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "template-provider")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	// Initial run (before "init") should work without error but will not
	// include the provider version, since we've not "locked" one yet.
	{
		stdout, stderr, err := tf.Run("version")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}

		// Check for actual errors instead of just non-empty stderr
		if containsRealError(stderr) {
			t.Errorf("unexpected error or warning in stderr output:\n%s", stderr)
		}

		wantVersion := fmt.Sprintf("OpenTofu v%s", version.String())
		if !strings.Contains(stdout, wantVersion) {
			t.Errorf("output does not contain our current version %q:\n%s", wantVersion, stdout)
		}
	}

	{
		_, _, err := tf.Run("init")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}
	}

	// After running init, we additionally include information about the
	// selected version of the "template" provider.
	{
		stdout, stderr, err := tf.Run("version")
		if err != nil {
			t.Errorf("unexpected error: %s", err)
		}

		// Check for actual errors instead of just non-empty stderr
		if containsRealError(stderr) {
			t.Errorf("unexpected error or warning in stderr output:\n%s", stderr)
		}

		wantMsg := "+ provider registry.opentofu.org/hashicorp/template v" // we don't know which version we'll get here
		if !strings.Contains(stdout, wantMsg) {
			t.Errorf("output does not contain provider information %q:\n%s", wantMsg, stdout)
		}
	}
}
