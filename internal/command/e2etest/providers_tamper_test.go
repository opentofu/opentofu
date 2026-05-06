// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package e2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/e2e"
	"github.com/opentofu/opentofu/internal/getproviders"
)

// providerTamperingFixtureNullProviderVersion must contain a string
// representation of the same version number used for "hashicorp/null" in the
// "provider-tampering-base" fixture.
const providerTamperingFixtureNullProviderVersion = "3.1.0"

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
	pluginDir := filepath.Join(
		".terraform",
		"providers",
		"registry.opentofu.org",
		"hashicorp",
		"null",
		providerTamperingFixtureNullProviderVersion,
		getproviders.CurrentPlatform.String(),
	)
	pluginExe := filepath.Join(pluginDir, "terraform-provider-null_v"+providerTamperingFixtureNullProviderVersion+"_x5")
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
		if want := `there is no package for registry.opentofu.org/hashicorp/null 3.1.0 cached in ` + providerCacheDir; !strings.Contains(SanitizeStderr(stderr), want) {
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
		if want := `the cached package for registry.opentofu.org/hashicorp/null 3.1.0 (in ` + providerCacheDir + `) does not match any of the checksums recorded in the dependency lock file`; !strings.Contains(SanitizeStderr(stderr), want) {
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
		if want := `there is no package for registry.opentofu.org/hashicorp/null 3.1.0 cached in ` + providerCacheDir; !strings.Contains(SanitizeStderr(stderr), want) {
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
		if want := `the cached package for registry.opentofu.org/hashicorp/null 3.1.0 (in ` + providerCacheDir + `) does not match any of the checksums recorded in the dependency lock file`; !strings.Contains(SanitizeStderr(stderr), want) {
			t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, stderr)
		}
	})
}

// TestProviderCacheJunkDirectory verifies our behavior for if there's already
// a directory present where we'd want to create a provider cache entry, for
// whatever reason: in that case, we want to fail with a clear error so that
// the operator can decide how to fix the problem, rather than either totally
// clobbering whatever was already there or merging the new package contents
// into an preexisting directory.
//
// One way we could get into this case is if someone has intentionally modified
// something in the provider cache directory to quickly test something and then
// ran "tofu init" again. We prefer to fail in that case to avoid silently
// clobbering whatever modification was made; operator should either delete
// their modified directory or restore it to match the official package before
// continuing.
func TestProviderCacheJunkDirectory(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// null provider, so it can only run if network access is allowed.
	skipIfCannotAccessNetwork(t)

	// We have a special case to allow installing into a precreated empty
	// directory, since someone reading our documentation about directory
	// layout might possibly try to create the empty directory manually first
	// and there's no real harm in accepting that case because an empty
	// directory is easy enough to recreate if that's what the operator really
	// wanted, for some reason. (An empty provider package is never actually
	// valid though, so that would be strange.)
	for _, emptyDir := range []bool{true, false} {
		t.Run(fmt.Sprintf("emptyDir=%#v", emptyDir), func(t *testing.T) {
			t.Parallel()

			// We reuse the "tampering" test fixture here even though this is a slightly
			// different situation where the potential problem exists before init,
			// rather than being introduced after init already ran.
			fixturePath := filepath.Join("testdata", "provider-tampering-base")
			tofu := e2e.NewBinary(t, tofuBin, fixturePath)

			// This test starts with a strange mismatching directory at the location
			// where the null provider package needs to be extracted, meaning that we
			// won't be able to install the provider without clobbering it.
			unexpectedDir := tofu.Path(
				".terraform",
				"providers",
				addrs.DefaultProviderRegistryHost.String(),
				"hashicorp",
				"null",
				providerTamperingFixtureNullProviderVersion,
				getproviders.CurrentPlatform.String(),
			)
			err := os.MkdirAll(unexpectedDir, os.ModePerm)
			if err != nil {
				t.Fatalf("failed to make 'unexpected' directory: %s", err)
			}
			if !emptyDir {
				// We'll make a file in the directory just to make it clear that this
				// directory can't possibly match the expected content of the provider
				// package, and so the provider installer definitely can't just try to
				// use this directory as-is without clobbering it.
				err = os.WriteFile(
					filepath.Join(unexpectedDir, "extra.txt"),
					[]byte("this file is not part of the hashicorp/null package"),
					os.ModePerm,
				)
				if err != nil {
					t.Fatalf("failed to make file in the 'unexpected' directory: %s", err)
				}
			}

			// Now we run "tofu init" with the expectation that it should try to
			// install "hashicorp/null" into the same location where we created
			// the directory above. There's no dependency lock file to tell us
			// that the existing directory might be enough to skip installation
			// completely, so this should always attempt installation.
			stdout, stderr, err := tofu.Run("init")
			if emptyDir {
				if err != nil {
					t.Fatalf("unexpected failure: %s\n(installing into a preexisting empty directory should be allowed)\n%s\n%s", err, stderr, stdout)
				}
			} else {
				if err == nil {
					t.Fatalf("unexpected success; want error about conflicting cache entry\n%s\n%s", stderr, stdout)
				}
				if want := "does not match the content of the downloaded package"; !strings.Contains(stderr, want) {
					t.Fatalf("stderr missing expected substring %q\n%s", want, stderr)
				}
			}
		})
	}
}

// TestProviderCacheJunkSymlink verifies our behavior for if there's already
// a symlink present where we'd want to create a provider cache entry, for
// whatever reason: in that case, we want to fail with a clear error so that
// the operator can decide how to fix the problem, rather than either totally
// clobbering their symlink or merging the package contents into whereever
// the symlink points.
//
// One way we could get into this case is if someone has intentionally created
// a symlink in their cache directory for provider development or testing
// purposes, and then later ran "tofu init" again. We prefer to fail in that
// case to avoid silently clobbering whatever modification was made; operator
// should either delete their symlink or make sure it refers to a directory
// that matches the expected package contents before continuing.
func TestProviderCacheJunkSymlink(t *testing.T) {
	t.Parallel()

	// This test reaches out to registry.opentofu.org to download the
	// null provider, so it can only run if network access is allowed.
	skipIfCannotAccessNetwork(t)

	// There is a special case to allow installing into a precreated empty
	// directory which we test in [TestProviderCacheJunkDirectory] above,
	// but that exception does not apply when what we find is a symlink _to_
	// an empty directory: a symlink is only acceptable if it refers to a
	// directory that was already correctly populated, because otherwise we
	// might be writing into an existing empty directory somewhere else in
	// the filesystem that is shared by other processes that expect it to stay
	// empty.
	for _, emptyDir := range []bool{true, false} {
		t.Run(fmt.Sprintf("emptyDir=%#v", emptyDir), func(t *testing.T) {
			t.Parallel()

			// We reuse the "tampering" test fixture here even though this is a slightly
			// different situation where the potential problem exists before init,
			// rather than being introduced after init already ran.
			fixturePath := filepath.Join("testdata", "provider-tampering-base")
			tofu := e2e.NewBinary(t, tofuBin, fixturePath)

			// This test starts with an unexpected symlink at the location where the
			// null provider package needs to be extracted, meaning that we won't
			// be able to install the provider without clobbering it.
			targetDir := tofu.Path("symlink-target")
			err := os.Mkdir(targetDir, os.ModePerm)
			if err != nil {
				t.Fatalf("failed to make symlink target directory: %s", err)
			}
			if !emptyDir {
				// We'll make a file in the directory just to make it clear that this
				// directory can't possibly match the expected content of the provider
				// package, and so the provider installer definitely can't just try to
				// use this directory as-is without clobbering the symlink.
				err = os.WriteFile(
					filepath.Join(targetDir, "extra.txt"),
					[]byte("this file is not part of the hashicorp/null package"),
					os.ModePerm,
				)
				if err != nil {
					t.Fatalf("failed to make file in the symlink target directory: %s", err)
				}
			}

			unexpectedSymlink := tofu.Path(
				".terraform",
				"providers",
				addrs.DefaultProviderRegistryHost.String(),
				"hashicorp",
				"null",
				providerTamperingFixtureNullProviderVersion,
				getproviders.CurrentPlatform.String(),
			)
			symlinkParent := filepath.Dir(unexpectedSymlink)
			err = os.MkdirAll(symlinkParent, os.ModePerm)
			if err != nil {
				t.Fatalf("failed to create parent directory of symlink: %s", err)
			}
			err = os.Symlink(targetDir, unexpectedSymlink)
			if err != nil {
				if runtime.GOOS == "windows" {
					// By default Windows does not allow creation of symlinks, so
					// we'll skip this test to avoid creating false-negative noise
					// for anyone developing OpenTofu on Windows without their
					// administrator having allowed symlink creation.
					t.Skipf("can't create symlink on this Windows system: %s", err)
				}
				t.Fatalf("failed to make 'unexpected' symlink: %s", err)
			}

			// Now we run "tofu init" with the expectation that it should try to
			// install "hashicorp/null" into the same location where we created
			// the directory above. There's no dependency lock file to tell us
			// that the existing directory might be enough to skip installation
			// completely, so this should always attempt installation.
			stdout, stderr, err := tofu.Run("init")
			// Note that unlike [TestProviderCacheJunkDirectory] we expect this
			// one to fail regardless of whether emptyDir is set.
			if err == nil {
				t.Fatalf("unexpected success; want error about conflicting cache entry\n%s\n%s", stderr, stdout)
			}
			if want := "does not match the content of the downloaded package"; !strings.Contains(stderr, want) {
				t.Errorf("stderr missing expected substring %q\n%s", want, stderr)
			}
		})
	}
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
