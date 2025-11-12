// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// TestProviderTampering tests various ways that the provider plugins in the
// local cache directory might be modified after an initial "tofu init",
// which other OpenTofu commands which use those plugins should catch and
// report early.
func TestProviderTampering(t *testing.T) {
	// General setup: we'll do a one-off init of a test directory as our
	// starting point, and then we'll clone that result for each test so
	// that we can save the cost of a repeated re-init with the same
	// provider.
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// null provider, so it can only run if network access is allowed.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "provider-tampering-base")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "Installing hashicorp/null v") {
		t.Errorf("null provider download message is missing from init output:\n%s", stdout)
		t.Logf("(this can happen if you have a copy of the plugin in one of the global plugin search dirs)")
	}

	seedDir := tf.WorkDir()
	const providerVersion = "3.1.0" // must match the version in the fixture config
	pluginDir := filepath.Join(".terraform", "providers", "registry.opentofu.org", "hashicorp", "null", providerVersion, getproviders.CurrentPlatform.String())
	pluginExe := filepath.Join(pluginDir, "terraform-provider-null_v"+providerVersion+"_x5")
	if getproviders.CurrentPlatform.OS == "windows" {
		pluginExe += ".exe" // ugh
	}

	// filepath.Join here to make sure we get the right path separator
	// for whatever OS we're running these tests on.
	providerCacheDir := filepath.Join(".terraform", "providers")

	t.Run("cache dir totally gone", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		err := os.RemoveAll(filepath.Join(workDir, ".terraform"))
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("plan")
		if err == nil {
			t.Fatalf("unexpected plan success\nstdout:\n%s", stdout)
		}
		if want := `registry.opentofu.org/hashicorp/null: there is no package for registry.opentofu.org/hashicorp/null 3.1.0 cached in ` + providerCacheDir; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
		if want := `tofu init`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}

		// Running init as suggested resolves the problem
		_, stderr, err = tf.Run("init")
		if err != nil {
			t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
		}
		_, stderr, err = tf.Run("plan")
		if err != nil {
			t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
		}
	})
	t.Run("cache dir totally gone, explicit backend", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		err := os.WriteFile(filepath.Join(workDir, "backend.tf"), []byte(localBackendConfig), 0600)
		if err != nil {
			t.Fatal(err)
		}

		err = os.RemoveAll(filepath.Join(workDir, ".terraform"))
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("plan")
		if err == nil {
			t.Fatalf("unexpected plan success\nstdout:\n%s", stdout)
		}
		if want := `Initial configuration of the requested backend "local"`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
		if want := `tofu init`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}

		// Running init as suggested resolves the problem
		_, stderr, err = tf.Run("init")
		if err != nil {
			t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
		}
		_, stderr, err = tf.Run("plan")
		if err != nil {
			t.Fatalf("unexpected plan error: %s\nstderr:\n%s", err, stderr)
		}
	})
	t.Run("null plugin package modified before plan", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		err := os.WriteFile(filepath.Join(workDir, pluginExe), []byte("tamper"), 0600)
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("plan")
		if err == nil {
			t.Fatalf("unexpected plan success\nstdout:\n%s", stdout)
		}
		if want := `registry.opentofu.org/hashicorp/null: the cached package for registry.opentofu.org/hashicorp/null 3.1.0 (in ` + providerCacheDir + `) does not match any of the checksums recorded in the dependency lock file`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
		if want := `tofu init`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
	})
	t.Run("version constraint changed in config before plan", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		err := os.WriteFile(filepath.Join(workDir, "provider-tampering-base.tf"), []byte(`
			terraform {
				required_providers {
					null = {
						source  = "hashicorp/null"
						version = "1.0.0"
					}
				}
			}
		`), 0600)
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("plan")
		if err == nil {
			t.Fatalf("unexpected plan success\nstdout:\n%s", stdout)
		}
		if want := `provider registry.opentofu.org/hashicorp/null: locked version selection 3.1.0 doesn't match the updated version constraints "1.0.0"`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
		if want := `tofu init -upgrade`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
	})
	t.Run("lock file modified before plan", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		// NOTE: We're just emptying out the lock file here because that's
		// good enough for what we're trying to assert. The leaf codepath
		// that generates this family of errors has some different variations
		// of this error message for other sorts of inconsistency, but those
		// are tested more thoroughly over in the "configs" package, which is
		// ultimately responsible for that logic.
		err := os.WriteFile(filepath.Join(workDir, ".terraform.lock.hcl"), []byte(``), 0600)
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("plan")
		if err == nil {
			t.Fatalf("unexpected plan success\nstdout:\n%s", stdout)
		}
		if want := `provider registry.opentofu.org/hashicorp/null: required by this configuration but no version is selected`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
		if want := `tofu init`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
	})
	t.Run("lock file modified after plan", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		_, stderr, err := tf.Run("plan", "-out", "tfplan")
		if err != nil {
			t.Fatalf("unexpected plan failure\nstderr:\n%s", stderr)
		}

		err = os.Remove(filepath.Join(workDir, ".terraform.lock.hcl"))
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("apply", "tfplan")
		if err == nil {
			t.Fatalf("unexpected apply success\nstdout:\n%s", stdout)
		}
		if want := `provider registry.opentofu.org/hashicorp/null: required by this configuration but no version is selected`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
		if want := `Create a new plan from the updated configuration.`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
	})
	t.Run("plugin cache dir entirely removed after plan", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		_, stderr, err := tf.Run("plan", "-out", "tfplan")
		if err != nil {
			t.Fatalf("unexpected plan failure\nstderr:\n%s", stderr)
		}

		err = os.RemoveAll(filepath.Join(workDir, ".terraform"))
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("apply", "tfplan")
		if err == nil {
			t.Fatalf("unexpected apply success\nstdout:\n%s", stdout)
		}
		if want := `registry.opentofu.org/hashicorp/null: there is no package for registry.opentofu.org/hashicorp/null 3.1.0 cached in ` + providerCacheDir; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
	})
	t.Run("null plugin package modified after plan", func(t *testing.T) {
		tf := e2e.NewBinary(t, tofuBin, seedDir)
		workDir := tf.WorkDir()

		_, stderr, err := tf.Run("plan", "-out", "tfplan")
		if err != nil {
			t.Fatalf("unexpected plan failure\nstderr:\n%s", stderr)
		}

		err = os.WriteFile(filepath.Join(workDir, pluginExe), []byte("tamper"), 0600)
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := tf.Run("apply", "tfplan")
		if err == nil {
			t.Fatalf("unexpected apply success\nstdout:\n%s", stdout)
		}
		if want := `registry.opentofu.org/hashicorp/null: the cached package for registry.opentofu.org/hashicorp/null 3.1.0 (in ` + providerCacheDir + `) does not match any of the checksums recorded in the dependency lock file`; !strings.Contains(stderr, want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
	})
}

// TestProviderLocksFromPredecessorProject is an end-to-end test of our
// special treatment of lock files that were originally created by the
// project that OpenTofu was forked from, and so refer to providers from
// that project's registry instead of OpenTofu's registry.
//
// In that case we attempt to adjust the lock file so that we'll select
// the same version of the equivalent provider in the OpenTofu registry,
// even though normally OpenTofu would see the providers in two different
// registries as completely distinct.
//
// This special behavior applies only to providers that match
// registry.terraform.io/hashicorp/*, since those are the ones that the
// OpenTofu project rebuilds and republishes with equivalent releases under
// registry.opentofu.org/hashicorp/*.
func TestProviderLocksFromPredecessorProject(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// null provider, so it can only run if network access is allowed.
	skipIfCannotAccessNetwork(t)

	fixturePath := filepath.Join("testdata", "predecessor-dependency-lock-file")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "Installing hashicorp/null v3.2.0") {
		t.Errorf("null provider download message is missing from init output:\n%s", stdout)
		t.Logf("(if the output specifies a version other than v3.2.0 then the fixup behavior did not work correctly)")
	}
	if !strings.Contains(stdout, "- registry.terraform.io/hashicorp/null => registry.opentofu.org/hashicorp/null") {
		t.Errorf("null provider dependency lock fixup message is missing from init output:\n%s", stdout)
	}

	// The lock file should have been updated to include the selection for
	// OpenTofu-flavored version of the provider along with the checksums
	// of OpenTofu's release, and the original entry should've been pruned
	// because as far as OpenTofu is concerned there's no dependency on
	// that provider in the current configuration.
	newLocks, err := tf.ReadFile(".terraform.lock.hcl")
	if err != nil {
		t.Fatalf("failed to load dependency lock file after init: %s", err)
	}
	locks, diags := depsfile.LoadLocksFromBytes(newLocks, ".terraform.lock.hcl")
	if diags.HasErrors() {
		t.Fatalf("failed to load dependency lock file after init: %s", diags.Err())
	}

	if lock := locks.Provider(addrs.MustParseProviderSourceString("registry.terraform.io/hashicorp/null")); lock != nil {
		t.Errorf("still have entry for %s v%s after init", lock.Provider(), lock.Version())
	}

	if lock := locks.Provider(addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/null")); lock != nil {
		if got, want := lock.Version(), getproviders.MustParseVersion("3.2.0"); got != want {
			t.Errorf("wrong version of %s was selected\ngot:  %s\nwant: %s", lock.Provider(), got, want)
		}
		// The full set of hashes we captured will vary depending on the
		// platform where this test is running, but the "zh:" ones in
		// particular come from the remote registry rather than from local
		// calculation and so we'll assume they should be consistent.
		allHashes := lock.AllHashes()
		wantHashes := []getproviders.Hash{
			// These are the official hashes for OpenTofu's build of
			// hashicorp/null v3.2.0, as recorded in the registry.
			getproviders.HashSchemeZip.New("11d576a7c9b9b5c3263fae11962216e8bce9e80ab9c5c7e2635a94f410d723f0"),
			getproviders.HashSchemeZip.New("11e53de20574d5e449c2d4e4f4249644244bad2a365e9793267796b9b96befab"),
			getproviders.HashSchemeZip.New("1eea180daf676f35e38aa0ca237334d86bdc7a4fd78da54c139d8c6e15ad0b7e"),
			getproviders.HashSchemeZip.New("47645b42501cb29acc270b99f93bf96bdae649159f2b3fdfafbc9543c36930e1"),
			getproviders.HashSchemeZip.New("639854d0182d91224e67b512bcc7d12705d7aca0095b2969c65680527402eef9"),
			getproviders.HashSchemeZip.New("894a3a5980bbe7e3d2948e0bcf56ae28b4ac16aa28c69f9a104c70af0f2f7ee1"),
			getproviders.HashSchemeZip.New("a4b4709333738c9e14cd285879f24792d8a2e277f071c9c641b11e5289c854f3"),
			getproviders.HashSchemeZip.New("c0fa29f9e93525f4672ea91b61ed866624ba3f3afd64d1c9eff8cc4c319ba69b"),
			getproviders.HashSchemeZip.New("f77678a6b62eb332d867cb7671982100f463d20a0f115c88a5d23f516ee872fa"),
			getproviders.HashSchemeZip.New("f7a8ab5f6b6c54667c240c8d8ed9c45a46bdbfa6bead009198a30def88e35376"),
		}
		var gotHashes []getproviders.Hash
		for _, hash := range allHashes {
			if hash.HasScheme(getproviders.HashSchemeZip) {
				gotHashes = append(gotHashes, hash)
			}
		}
		slices.Sort(gotHashes) // order is unimportant
		if diff := cmp.Diff(wantHashes, gotHashes); diff != "" {
			t.Error("wrong hashes in lock file after init\n" + diff)
		}
	} else {
		t.Errorf("missing entry for registry.opentofu.org/hashicorp/null after init")
	}

}

// TestProviderLocksFromPredecessorProjectWithAbsoluteSourceAddr is a variant
// of TestProviderLocksFromPredecessorProject for the not-typically-recommended
// situation where the configuration contains a source address that explicitly
// specifies our predecessor project's registry hostname.
//
// Using OpenTofu with such a configuration is problematic by default because
// that registry's terms of service prohibit using it with OpenTofu, but it
// can potentially be okay (with some caveats, and THIS IS NOT LEGAL ADVICE)
// if using a non-default installation method configuration that arranges for
// that hostname to be handled by a mirror source rather than by direct
// communication with the origin registry.
//
// This test ensures that our lock file migration behavior handles this unusual
// situation in a reasonable way, using a local filesystem mirror to avoid
// directly accessing the predecessor's registry.
func TestProviderLocksFromPredecessorProjectWithAbsoluteSourceAddr(t *testing.T) {
	t.Parallel()

	// We'll use an overridden CLI configuration file to force installing
	// from a filesystem mirror, since we're not allowed to access
	// registry.terraform.io directly from this test.
	tempDir := t.TempDir()
	cliConfigFile := filepath.Join(tempDir, "cliconfig.tfrc")
	err := os.WriteFile(
		cliConfigFile,
		fmt.Appendf(nil, `
			provider_installation {
				filesystem_mirror {
					path = %q
				}
			}

			# The following is just some additional insurance against
			# making real requests to registry.terraform.io.
			host "registry.terraform.io" {
				# Prevents service discovery, and instead behaves as if the
				# discovery document declares nothing at all.
				services = {}
			}
		`, tempDir),
		os.ModePerm,
	)
	if err != nil {
		t.Fatal(err)
	}
	platform := getproviders.CurrentPlatform
	providerPkgDir := filepath.Join(tempDir, "registry.terraform.io", "hashicorp", "null", "3.2.0", platform.OS+"_"+platform.Arch)
	err = os.MkdirAll(providerPkgDir, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(
		filepath.Join(providerPkgDir, "terraform-provider-null"),
		[]byte(`this is not a real provider plugin; it's just a placeholder`),
		os.ModePerm,
	)
	if err != nil {
		t.Fatal(err)
	}

	// We should now be able to use the temporary directory created above to
	// install our fake mirror of the provider.
	fixturePath := filepath.Join("testdata", "predecessor-dependency-lock-file-abs")
	tf := e2e.NewBinary(t, tofuBin, fixturePath)
	tf.AddEnv("TF_CLI_CONFIG_FILE=" + cliConfigFile)

	stdout, stderr, err := tf.Run("init")
	if err != nil {
		t.Fatalf("unexpected init error: %s\nstderr:\n%s", err, stderr)
	}
	// Note that this explicitly mentions registry.terraform.io. because that
	// hostname was chosen explicitly in the configuration.
	if !strings.Contains(stdout, "Installing registry.terraform.io/hashicorp/null v3.2.0") {
		t.Errorf("null provider download message is missing from init output:\n%s", stdout)
		t.Logf("(if the output specifies a version other than v3.2.0 then the fixup behavior did not work correctly)")
	}
	// We always produce the warning about amending the lock file, even in this
	// case where it doesn't technically apply, because we don't recommend
	// using registry.terraform.io directly and only make a best effort to keep
	// it working, so we don't want to over-complicate the migration logic for
	// a situation that should be very rare.
	if !strings.Contains(stdout, "- registry.terraform.io/hashicorp/null => registry.opentofu.org/hashicorp/null") {
		t.Errorf("null provider dependency lock fixup message is missing from init output:\n%s", stdout)
	}

	// The lock file should still contain the entry for
	// registry.terraform.io/hashicorp/null, and the synthetic extra entry
	// for registry.opentofu.org/hashicorp/null should have been pruned before
	// writing the file in this case because there's no mention of that provider
	// in the configuration.
	newLocks, err := tf.ReadFile(".terraform.lock.hcl")
	if err != nil {
		t.Fatalf("failed to load dependency lock file after init: %s", err)
	}
	locks, diags := depsfile.LoadLocksFromBytes(newLocks, ".terraform.lock.hcl")
	if diags.HasErrors() {
		t.Fatalf("failed to load dependency lock file after init: %s", diags.Err())
	}

	if lock := locks.Provider(addrs.MustParseProviderSourceString("registry.opentofu.org/hashicorp/null")); lock != nil {
		t.Errorf("unexpected entry for %s v%s after init", lock.Provider(), lock.Version())
	}
	if lock := locks.Provider(addrs.MustParseProviderSourceString("registry.terraform.io/hashicorp/null")); lock != nil {
		if got, want := lock.Version(), getproviders.MustParseVersion("3.2.0"); got != want {
			t.Errorf("wrong version of %s was selected\ngot:  %s\nwant: %s", lock.Provider(), got, want)
		}
		gotHashes := lock.AllHashes()
		wantHashes := []getproviders.Hash{
			// This is the hash of our placeholder provider package containing
			// a not-actually-executable plugin stub. We don't vary this by
			// platform so this hash should match regardless of where this
			// test is running.
			getproviders.HashScheme1.New("DvLRiv4Pbjq3Rh0yNWtq+9dwVXqHF+bQspfhckLyFWU="),
		}
		if diff := cmp.Diff(wantHashes, gotHashes); diff != "" {
			t.Error("wrong hashes in lock file after init\n" + diff)
		}
		for _, hash := range gotHashes {
			if hash.HasScheme(getproviders.HashSchemeZip) {
				// We should not get in here. If we do then we've likely just
				// installed the _real_ hashicorp/null, and so we need to fix
				// this test soon so that we're not depending on an external
				// network service for this supposedly-local-only test.
				t.Errorf("NOTE: unexpected hash %q suggests that this was installed from the origin registry, rather than the mirror!", hash)
			}
		}
	} else {
		t.Errorf("missing entry for registry.terraform.io/hashicorp/null after init")
		return
	}

	// The above work should've left us in a valid situation where we can now
	// run other workflow commands using the selected plugins. Since we've
	// "installed" a fake thing that can't actually be executed the following
	// will fail, but it should fail trying to install the fake executable
	// rather than failing in command.Meta.providerFactories due to there
	// being a plugin that isn't present at all.
	// (Historically we had a bug where other commands would re-shim the
	// dependency locks to refer to registry.opentofu.org/hashicorp/null and
	// would thus make this fail because no such plugin is available in the
	// cache directory: https://github.com/opentofu/opentofu/issues/2977 )
	_, stderr, err = tf.Run("validate")
	if err == nil {
		t.Fatalf("unexpected success from tofu validate; want plugin execution error")
	}
	gotErr := stderr
	wantErr := `failed to instantiate provider "registry.terraform.io/hashicorp/null" to obtain schema`
	if !strings.Contains(gotErr, wantErr) {
		t.Fatalf("unexpected validate error\ngot:\n%s\nwant substring: %s", gotErr, wantErr)
	}
}

const localBackendConfig = `
terraform {
  backend "local" {
    path = "terraform.tfstate"
  }
}
`
