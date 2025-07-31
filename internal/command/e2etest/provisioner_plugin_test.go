// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
)

// TestProvisionerPlugin is a test that tofu can execute a 3rd party
// provisioner plugin.
func TestProvisionerPlugin(t *testing.T) {
	if !canRunGoBuild {
		// We're running in a separate-build-then-run context, so we can't
		// currently execute this test which depends on being able to build
		// new executable at runtime.
		//
		// (See the comment on canRunGoBuild's declaration for more information.)
		t.Skip("can't run without building a new provisioner executable")
	}
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// template and null providers, so it can only run if network access is
	// allowed.
	skipIfCannotAccessNetwork(t)

	tf := e2e.NewBinary(t, tofuBin, "testdata/provisioner-plugin")

	// In order to do a decent end-to-end test for this case we will need a
	// real enough provisioner plugin to try to run and make sure we are able
	// to actually run it. Here will build the local-exec provisioner into a
	// binary called test-provisioner
	provisionerExePrefix := filepath.Join(tf.WorkDir(), "terraform-provisioner-test_")
	provisionerExe := e2e.GoBuild("github.com/opentofu/opentofu/internal/provisioner-local-exec/main", provisionerExePrefix)

	extension := ""
	if runtime.GOOS == "windows" {
		extension = ".exe"
	}

	// provisioners must use the old binary name format, so rename this binary
	newExe := filepath.Join(tf.WorkDir(), "terraform-provisioner-test") + extension
	if _, err := os.Stat(newExe); !os.IsNotExist(err) {
		t.Fatalf("%q already exists", newExe)
	}
	if err := os.Rename(provisionerExe, newExe); err != nil {
		t.Fatalf("error renaming provisioner binary: %v", err)
	}
	provisionerExe = newExe

	t.Logf("temporary provisioner executable is %s", provisionerExe)

	//// INIT
	_, stderr, err := tf.Run("init")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}

	//// PLAN
	_, stderr, err = tf.Run("plan", "-out=tfplan")
	if err != nil {
		t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
	}

	//// APPLY
	stdout, stderr, err := tf.Run("apply", "tfplan")
	if err != nil {
		t.Fatalf("unexpected apply error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "HelloProvisioner") {
		t.Fatalf("missing provisioner output:\n%s", stdout)
	}
}
