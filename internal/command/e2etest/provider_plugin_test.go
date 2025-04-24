// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// TestProviderProtocols verifies that OpenTofu can execute provider plugins
// with both supported protocol versions.
func TestProviderProtocols(t *testing.T) {
	if !canRunGoBuild {
		// We're running in a separate-build-then-run context, so we can't
		// currently execute this test which depends on being able to build
		// new executable at runtime.
		//
		// (See the comment on canRunGoBuild's declaration for more information.)
		t.Skip("can't run without building a new provider executable")
	}
	t.Parallel()

	tf := e2e.NewBinary(t, tofuBin, "testdata/provider-plugin")

	// In order to do a decent end-to-end test for this case we will need a real
	// enough provider plugin to try to run and make sure we are able to
	// actually run it. Here will build the simple and simple6 (built with
	// protocol v6) providers.
	simple6Provider := filepath.Join(tf.WorkDir(), "terraform-provider-simple6")
	simple6ProviderExe := e2e.GoBuild("github.com/opentofu/opentofu/internal/provider-simple-v6/main", simple6Provider)

	simpleProvider := filepath.Join(tf.WorkDir(), "terraform-provider-simple")
	simpleProviderExe := e2e.GoBuild("github.com/opentofu/opentofu/internal/provider-simple/main", simpleProvider)

	// Move the provider binaries into a directory that we will point tofu
	// to using the -plugin-dir cli flag.
	platform := getproviders.CurrentPlatform.String()
	hashiDir := "cache/registry.opentofu.org/hashicorp/"
	if err := os.MkdirAll(tf.Path(hashiDir, "simple6/0.0.1/", platform), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(simple6ProviderExe, tf.Path(hashiDir, "simple6/0.0.1/", platform, "terraform-provider-simple6")); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(tf.Path(hashiDir, "simple/0.0.1/", platform), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(simpleProviderExe, tf.Path(hashiDir, "simple/0.0.1/", platform, "terraform-provider-simple")); err != nil {
		t.Fatal(err)
	}

	//// INIT
	_, stderr, err := tf.Run("init", "-plugin-dir=cache")
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

	if !strings.Contains(stdout, "Apply complete! Resources: 2 added, 0 changed, 0 destroyed.") {
		t.Fatalf("wrong output:\nstdout:%s\nstderr%s", stdout, stderr)
	}

	/// DESTROY
	stdout, stderr, err = tf.Run("destroy", "-auto-approve")
	if err != nil {
		t.Fatalf("unexpected apply error: %s\nstderr:\n%s", err, stderr)
	}

	if !strings.Contains(stdout, "Resources: 2 destroyed") {
		t.Fatalf("wrong destroy output\nstdout:%s\nstderr:%s", stdout, stderr)
	}
}

// This test is designed to simulate a *very* busy CI server that has multiple
// processes sharing a global provider cache. This exercises the locking in the
// "providercache" package, as well as simulating bad file hashes in the
// lock file.
func TestProviderGlobalCache(t *testing.T) {
	if !canAccessNetwork() {
		t.Skip("Requires provider download access for e2e provider interactions")
	}

	t.Parallel()

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	rcLoc := filepath.Join(tmpDir, ".tofurc")
	rcData := fmt.Sprintf(`plugin_cache_dir = "%s"`, tmpDir)
	err = os.WriteFile(rcLoc, []byte(rcData), 0600)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			tf := e2e.NewBinary(t, tofuBin, "testdata/provider-global-cache")
			tf.AddEnv(fmt.Sprintf("TF_CLI_CONFIG_FILE=%s", rcLoc))

			stdout, stderr, err := tf.Run("init")
			tofuResult{t, stdout, stderr, err}.Success()
			wg.Done()
		}()
	}

	wg.Wait()
}
