// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package initwd

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-test/deep"
	"github.com/google/go-cmp/cmp"
	version "github.com/hashicorp/go-version"
	svchost "github.com/hashicorp/terraform-svchost"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/copy"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/registry"
	"github.com/opentofu/opentofu/internal/tfdiags"

	_ "github.com/opentofu/opentofu/internal/logging"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestModuleInstaller(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/local-modules")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")
	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	wantCalls := []testInstallHookCall{
		{
			Name:        "Install",
			ModuleAddr:  "child_a",
			PackageAddr: "",
			LocalPath:   "child_a",
		},
		{
			Name:        "Install",
			ModuleAddr:  "child_a.child_b",
			PackageAddr: "",
			LocalPath:   filepath.Join("child_a", "child_b"),
		},
	}

	if assertResultDeepEqual(t, hooks.Calls, wantCalls) {
		return
	}

	loader, err := configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfig(t.Context(), ".", configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags))

	wantTraces := map[string]string{
		"":                "in root module",
		"child_a":         "in child_a module",
		"child_a.child_b": "in child_b module",
	}
	gotTraces := map[string]string{}
	config.DeepEach(func(c *configs.Config) {
		path := strings.Join(c.Path, ".")
		if c.Module.Variables["v"] == nil {
			gotTraces[path] = "<missing>"
			return
		}
		varDesc := c.Module.Variables["v"].Description
		gotTraces[path] = varDesc
	})
	assertResultDeepEqual(t, gotTraces, wantTraces)
}

func TestModuleInstaller_error(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/local-module-error")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

	if !diags.HasErrors() {
		t.Fatal("expected error")
	} else {
		assertDiagnosticSummary(t, diags, "Invalid module source address")
	}
}

func TestModuleInstaller_emptyModuleName(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/empty-module-name")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

	if !diags.HasErrors() {
		t.Fatal("expected error")
	} else {
		assertDiagnosticSummary(t, diags, "Invalid module instance name")
	}
}

func TestModuleInstaller_invalidModuleName(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/invalid-module-name")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, registry.NewClient(t.Context(), nil, nil), nil)
	_, diags := inst.InstallModules(context.Background(), dir, "tests", false, false, hooks, configs.RootModuleCallForTesting())
	if !diags.HasErrors() {
		t.Fatal("expected error")
	} else {
		assertDiagnosticSummary(t, diags, "Invalid module instance name")
	}
}

func TestModuleInstaller_packageEscapeError(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/load-module-package-escape")
	dir := tempChdir(t, fixtureDir)

	// For this particular test we need an absolute path in the root module
	// that must actually resolve to our temporary directory in "dir", so
	// we need to do a little rewriting. We replace the arbitrary placeholder
	// %%BASE%% with the temporary directory path.
	{
		rootFilename := filepath.Join(dir, "package-escape.tf")
		template, err := os.ReadFile(rootFilename)
		if err != nil {
			t.Fatal(err)
		}
		final := bytes.ReplaceAll(template, []byte("%%BASE%%"), []byte(filepath.ToSlash(dir)))
		err = os.WriteFile(rootFilename, final, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	// This test needs a real getmodules.PackageFetcher because it makes use of
	// the esoteric legacy support for treating an absolute filesystem path
	// as if it were a "remote package". This should not use any of the
	// truly-"remote" module sources, even though it technically has access to.
	inst := NewModuleInstaller(modulesDir, loader, nil, getmodules.NewPackageFetcher(t.Context(), nil))
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

	if !diags.HasErrors() {
		t.Fatal("expected error")
	} else {
		assertDiagnosticSummary(t, diags, "Local module path escapes module package")
	}
}

func TestModuleInstaller_explicitPackageBoundary(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/load-module-package-prefix")
	dir := tempChdir(t, fixtureDir)

	// For this particular test we need an absolute path in the root module
	// that must actually resolve to our temporary directory in "dir", so
	// we need to do a little rewriting. We replace the arbitrary placeholder
	// %%BASE%% with the temporary directory path.
	{
		rootFilename := filepath.Join(dir, "package-prefix.tf")
		template, err := os.ReadFile(rootFilename)
		if err != nil {
			t.Fatal(err)
		}
		final := bytes.ReplaceAll(template, []byte("%%BASE%%"), []byte(filepath.ToSlash(dir)))
		err = os.WriteFile(rootFilename, final, 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	// This test needs a real getmodules.PackageFetcher because it makes use of
	// the esoteric legacy support for treating an absolute filesystem path
	// as if it were a "remote package". This should not use any of the
	// truly-"remote" module sources, even though it technically has access to.
	inst := NewModuleInstaller(modulesDir, loader, nil, getmodules.NewPackageFetcher(t.Context(), nil))
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

	if diags.HasErrors() {
		t.Fatalf("unexpected errors\n%s", diags.Err().Error())
	}
}

func TestModuleInstaller_Prerelease(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("this test accesses registry.opentofu.org and github.com; set TF_ACC=1 to run it")
	}

	testCases := []struct {
		name            string
		modulePath      string
		expectedVersion string
		shouldError     bool
	}{
		{
			name:            "exact match",
			modulePath:      "testdata/prerelease-constraint-match",
			expectedVersion: "v0.0.3-alpha.1",
		},
		{
			name:            "exact match v prefix",
			modulePath:      "testdata/prerelease-constraint-v-match",
			expectedVersion: "v0.0.3-alpha.1",
		},
		{
			name:            "exact match eq selector",
			modulePath:      "testdata/prerelease-constraint-eq-match",
			expectedVersion: "v0.0.3-alpha.1",
		},
		{
			name:            "exact match v prefix eq selector",
			modulePath:      "testdata/prerelease-constraint-v-eq-match",
			expectedVersion: "v0.0.3-alpha.1",
		},
		{
			name:            "partial match",
			modulePath:      "testdata/prerelease-constraint",
			expectedVersion: "v0.0.2",
		},
		{
			name:            "partial match v prefix",
			modulePath:      "testdata/prerelease-constraint-v",
			expectedVersion: "v0.0.2",
		},
		{
			name:        "multiple constraints",
			modulePath:  "testdata/prerelease-constraint-multiple",
			shouldError: true,
			// NOTE: This one fails because we don't support mixing a prerelease version
			// selection with other constraints in a single constraint string. This is
			// unfortunate but accepted for now as a concession to backward compatibility
			// until we have a more complete plan on how to deal with the various legacy
			// quirks of our version constraint matching.
			//
			// For more information:
			//     https://github.com/opentofu/opentofu/issues/2117
		},
		{
			name:        "err",
			modulePath:  "testdata/prerelease-constraint-err",
			shouldError: true,
		},
		{
			name:        "err v prefix",
			modulePath:  "testdata/prerelease-constraint-v-err",
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixtureDir := filepath.Clean(tc.modulePath)
			dir := tempChdir(t, fixtureDir)

			hooks := &testInstallHooks{}

			modulesDir := filepath.Join(dir, ".terraform/modules")

			loader := configload.NewLoaderForTests(t)
			inst := NewModuleInstaller(modulesDir, loader, registry.NewClient(t.Context(), nil, nil), nil)
			cfg, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

			if tc.shouldError {
				if !diags.HasErrors() {
					t.Fatalf("an error was expected, but none was found")
				}
				return
			}

			if diags.HasErrors() {
				t.Fatalf("found unexpected errors: %s", diags.Err())
			}

			if !cfg.Children["acctest"].Version.Equal(version.Must(version.NewVersion(tc.expectedVersion))) {
				t.Fatalf("expected version %s but found version %s", tc.expectedVersion, cfg.Version.String())
			}
		})
	}
}

func TestModuleInstaller_invalid_version_constraint_error(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/invalid-version-constraint")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

	if !diags.HasErrors() {
		t.Fatal("expected error")
	} else {
		// We use the presence of the "version" argument as a heuristic for
		// user intent to use a registry module, and so we intentionally catch
		// this as an invalid registry module address rather than an invalid
		// version constraint, so we can surface the specific address parsing
		// error instead of a generic version constraint error.
		assertDiagnosticSummary(t, diags, "Invalid registry module source address")
	}
}

func TestModuleInstaller_invalidVersionConstraintGetter(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/invalid-version-constraint")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

	if !diags.HasErrors() {
		t.Fatal("expected error")
	} else {
		// We use the presence of the "version" argument as a heuristic for
		// user intent to use a registry module, and so we intentionally catch
		// this as an invalid registry module address rather than an invalid
		// version constraint, so we can surface the specific address parsing
		// error instead of a generic version constraint error.
		assertDiagnosticSummary(t, diags, "Invalid registry module source address")
	}
}

func TestModuleInstaller_invalidVersionConstraintLocal(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/invalid-version-constraint-local")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())

	if !diags.HasErrors() {
		t.Fatal("expected error")
	} else {
		// We use the presence of the "version" argument as a heuristic for
		// user intent to use a registry module, and so we intentionally catch
		// this as an invalid registry module address rather than an invalid
		// version constraint, so we can surface the specific address parsing
		// error instead of a generic version constraint error.
		assertDiagnosticSummary(t, diags, "Invalid registry module source address")
	}
}

func TestModuleInstaller_symlink(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/local-module-symlink")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	wantCalls := []testInstallHookCall{
		{
			Name:        "Install",
			ModuleAddr:  "child_a",
			PackageAddr: "",
			LocalPath:   "child_a",
		},
		{
			Name:        "Install",
			ModuleAddr:  "child_a.child_b",
			PackageAddr: "",
			LocalPath:   filepath.Join("child_a", "child_b"),
		},
	}

	if assertResultDeepEqual(t, hooks.Calls, wantCalls) {
		return
	}

	loader, err := configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfig(t.Context(), ".", configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags))

	wantTraces := map[string]string{
		"":                "in root module",
		"child_a":         "in child_a module",
		"child_a.child_b": "in child_b module",
	}
	gotTraces := map[string]string{}
	config.DeepEach(func(c *configs.Config) {
		path := strings.Join(c.Path, ".")
		if c.Module.Variables["v"] == nil {
			gotTraces[path] = "<missing>"
			return
		}
		varDesc := c.Module.Variables["v"].Description
		gotTraces[path] = varDesc
	})
	assertResultDeepEqual(t, gotTraces, wantTraces)
}

func TestLoaderInstallModules_registry(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("this test accesses registry.opentofu.org and github.com; set TF_ACC=1 to run it")
	}

	fixtureDir := filepath.Clean("testdata/registry-modules")
	tmpDir := tempChdir(t, fixtureDir)
	// the module installer runs filepath.EvalSymlinks() on the destination
	// directory before copying files, and the resultant directory is what is
	// returned by the install hooks. Without this, tests could fail on machines
	// where the default temp dir was a symlink.
	dir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Error(err)
	}

	hooks := &testInstallHooks{}
	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, registry.NewClient(t.Context(), nil, nil), nil)
	_, diags := inst.InstallModules(context.Background(), dir, "tests", false, false, hooks, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	v := version.Must(version.NewVersion("0.0.1"))

	wantCalls := []testInstallHookCall{
		// the configuration builder visits each level of calls in lexicographical
		// order by name, so the following list is kept in the same order.

		// acctest_child_a accesses //modules/child_a directly
		{
			Name:        "Download",
			ModuleAddr:  "acctest_child_a",
			PackageAddr: "registry.opentofu.org/hashicorp/module-installer-acctest/aws", // intentionally excludes the subdir because we're downloading the whole package here
			Version:     v,
		},
		{
			Name:       "Install",
			ModuleAddr: "acctest_child_a",
			Version:    v,
			// NOTE: This local path and the other paths derived from it below
			// can vary depending on how the registry is implemented. At the
			// time of writing this test, registry.opentofu.org returns
			// git repository source addresses and so this path refers to the
			// root of the git clone, but historically the registry referred
			// to GitHub-provided tar archives which meant that there was an
			// extra level of subdirectory here for the typical directory
			// nesting in tar archives, which would've been reflected as
			// an extra segment on this path. If this test fails due to an
			// additional path segment in future, then a change to the upstream
			// registry might be the root cause.
			LocalPath: filepath.Join(dir, ".terraform/modules/acctest_child_a/modules/child_a"),
		},

		// acctest_child_a.child_b
		// (no download because it's a relative path inside acctest_child_a)
		{
			Name:       "Install",
			ModuleAddr: "acctest_child_a.child_b",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_child_a/modules/child_b"),
		},

		// acctest_child_b accesses //modules/child_b directly
		{
			Name:        "Download",
			ModuleAddr:  "acctest_child_b",
			PackageAddr: "registry.opentofu.org/hashicorp/module-installer-acctest/aws", // intentionally excludes the subdir because we're downloading the whole package here
			Version:     v,
		},
		{
			Name:       "Install",
			ModuleAddr: "acctest_child_b",
			Version:    v,
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_child_b/modules/child_b"),
		},

		// acctest_root
		{
			Name:        "Download",
			ModuleAddr:  "acctest_root",
			PackageAddr: "registry.opentofu.org/hashicorp/module-installer-acctest/aws",
			Version:     v,
		},
		{
			Name:       "Install",
			ModuleAddr: "acctest_root",
			Version:    v,
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_root"),
		},

		// acctest_root.child_a
		// (no download because it's a relative path inside acctest_root)
		{
			Name:       "Install",
			ModuleAddr: "acctest_root.child_a",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_root/modules/child_a"),
		},

		// acctest_root.child_a.child_b
		// (no download because it's a relative path inside acctest_root, via acctest_root.child_a)
		{
			Name:       "Install",
			ModuleAddr: "acctest_root.child_a.child_b",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_root/modules/child_b"),
		},
	}

	if diff := cmp.Diff(wantCalls, hooks.Calls); diff != "" {
		t.Fatalf("wrong installer calls\n%s", diff)
	}

	// check that the registry responses were cached
	packageAddr := addrs.ModuleRegistryPackage{
		Host:         svchost.Hostname("registry.opentofu.org"),
		Namespace:    "hashicorp",
		Name:         "module-installer-acctest",
		TargetSystem: "aws",
	}
	if _, ok := inst.registryPackageVersions[packageAddr]; !ok {
		t.Errorf("module versions cache was not populated\ngot: %s\nwant: key hashicorp/module-installer-acctest/aws", spew.Sdump(inst.registryPackageVersions))
	}
	if _, ok := inst.registryPackageSources[moduleVersion{module: packageAddr, version: "0.0.1"}]; !ok {
		t.Errorf("module download url cache was not populated\ngot: %s", spew.Sdump(inst.registryPackageSources))
	}

	loader, err = configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfig(t.Context(), ".", configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags))

	wantTraces := map[string]string{
		"":                             "in local caller for registry-modules",
		"acctest_root":                 "in root module",
		"acctest_root.child_a":         "in child_a module",
		"acctest_root.child_a.child_b": "in child_b module",
		"acctest_child_a":              "in child_a module",
		"acctest_child_a.child_b":      "in child_b module",
		"acctest_child_b":              "in child_b module",
	}
	gotTraces := map[string]string{}
	config.DeepEach(func(c *configs.Config) {
		path := strings.Join(c.Path, ".")
		if c.Module.Variables["v"] == nil {
			gotTraces[path] = "<missing>"
			return
		}
		varDesc := c.Module.Variables["v"].Description
		gotTraces[path] = varDesc
	})
	assertResultDeepEqual(t, gotTraces, wantTraces)

}

func TestLoaderInstallModules_goGetter(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("this test accesses github.com; set TF_ACC=1 to run it")
	}

	fixtureDir := filepath.Clean("testdata/go-getter-modules")
	tmpDir := tempChdir(t, fixtureDir)
	// the module installer runs filepath.EvalSymlinks() on the destination
	// directory before copying files, and the resultant directory is what is
	// returned by the install hooks. Without this, tests could fail on machines
	// where the default temp dir was a symlink.
	dir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Error(err)
	}

	hooks := &testInstallHooks{}
	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, registry.NewClient(t.Context(), nil, nil), nil)
	_, diags := inst.InstallModules(context.Background(), dir, "tests", false, false, hooks, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	wantCalls := []testInstallHookCall{
		// the configuration builder visits each level of calls in lexicographical
		// order by name, so the following list is kept in the same order.

		// acctest_child_a accesses //modules/child_a directly
		{
			Name:        "Download",
			ModuleAddr:  "acctest_child_a",
			PackageAddr: "git::https://github.com/hashicorp/terraform-aws-module-installer-acctest.git?ref=v0.0.1", // intentionally excludes the subdir because we're downloading the whole repo here
		},
		{
			Name:       "Install",
			ModuleAddr: "acctest_child_a",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_child_a/modules/child_a"),
		},

		// acctest_child_a.child_b
		// (no download because it's a relative path inside acctest_child_a)
		{
			Name:       "Install",
			ModuleAddr: "acctest_child_a.child_b",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_child_a/modules/child_b"),
		},

		// acctest_child_b accesses //modules/child_b directly
		{
			Name:        "Download",
			ModuleAddr:  "acctest_child_b",
			PackageAddr: "git::https://github.com/hashicorp/terraform-aws-module-installer-acctest.git?ref=v0.0.1", // intentionally excludes the subdir because we're downloading the whole package here
		},
		{
			Name:       "Install",
			ModuleAddr: "acctest_child_b",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_child_b/modules/child_b"),
		},

		// acctest_root
		{
			Name:        "Download",
			ModuleAddr:  "acctest_root",
			PackageAddr: "git::https://github.com/hashicorp/terraform-aws-module-installer-acctest.git?ref=v0.0.1",
		},
		{
			Name:       "Install",
			ModuleAddr: "acctest_root",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_root"),
		},

		// acctest_root.child_a
		// (no download because it's a relative path inside acctest_root)
		{
			Name:       "Install",
			ModuleAddr: "acctest_root.child_a",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_root/modules/child_a"),
		},

		// acctest_root.child_a.child_b
		// (no download because it's a relative path inside acctest_root, via acctest_root.child_a)
		{
			Name:       "Install",
			ModuleAddr: "acctest_root.child_a.child_b",
			LocalPath:  filepath.Join(dir, ".terraform/modules/acctest_root/modules/child_b"),
		},
	}

	if diff := cmp.Diff(wantCalls, hooks.Calls); diff != "" {
		t.Fatalf("wrong installer calls\n%s", diff)
	}

	loader, err = configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfig(t.Context(), ".", configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags))

	wantTraces := map[string]string{
		"":                             "in local caller for go-getter-modules",
		"acctest_root":                 "in root module",
		"acctest_root.child_a":         "in child_a module",
		"acctest_root.child_a.child_b": "in child_b module",
		"acctest_child_a":              "in child_a module",
		"acctest_child_a.child_b":      "in child_b module",
		"acctest_child_b":              "in child_b module",
	}
	gotTraces := map[string]string{}
	config.DeepEach(func(c *configs.Config) {
		path := strings.Join(c.Path, ".")
		if c.Module.Variables["v"] == nil {
			gotTraces[path] = "<missing>"
			return
		}
		varDesc := c.Module.Variables["v"].Description
		gotTraces[path] = varDesc
	})
	assertResultDeepEqual(t, gotTraces, wantTraces)

}

func TestModuleInstaller_fromTests(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/local-module-from-test")
	dir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}

	modulesDir := filepath.Join(dir, ".terraform/modules")
	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, nil, nil)
	_, diags := inst.InstallModules(context.Background(), ".", "tests", false, false, hooks, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	wantCalls := []testInstallHookCall{
		{
			Name:        "Install",
			ModuleAddr:  "test.tests.main.setup",
			PackageAddr: "",
			LocalPath:   "setup",
		},
	}

	if assertResultDeepEqual(t, hooks.Calls, wantCalls) {
		return
	}

	loader, err := configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfigWithTests(t.Context(), ".", "tests", configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags))

	if config.Module.Tests[filepath.Join("tests", "main.tftest.hcl")].Runs[0].ConfigUnderTest == nil {
		t.Fatalf("should have loaded config into the relevant run block but did not")
	}
}

func TestLoadInstallModules_registryFromTest(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("this test accesses registry.opentofu.org and github.com; set TF_ACC=1 to run it")
	}

	fixtureDir := filepath.Clean("testdata/registry-module-from-test")
	tmpDir := tempChdir(t, fixtureDir)
	// the module installer runs filepath.EvalSymlinks() on the destination
	// directory before copying files, and the resultant directory is what is
	// returned by the install hooks. Without this, tests could fail on machines
	// where the default temp dir was a symlink.
	dir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Error(err)
	}

	hooks := &testInstallHooks{}
	modulesDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(modulesDir, loader, registry.NewClient(t.Context(), nil, nil), nil)
	_, diags := inst.InstallModules(context.Background(), dir, "tests", false, false, hooks, configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, diags)

	v := version.Must(version.NewVersion("0.0.1"))
	wantCalls := []testInstallHookCall{
		// the configuration builder visits each level of calls in lexicographical
		// order by name, so the following list is kept in the same order.

		// setup access acctest directly.
		{
			Name:        "Download",
			ModuleAddr:  "test.main.setup",
			PackageAddr: "registry.opentofu.org/hashicorp/module-installer-acctest/aws", // intentionally excludes the subdir because we're downloading the whole package here
			Version:     v,
		},
		{
			Name:       "Install",
			ModuleAddr: "test.main.setup",
			Version:    v,
			// NOTE: This local path and the other paths derived from it below
			// can vary depending on how the registry is implemented. At the
			// time of writing this test, registry.opentofu.org returns
			// git repository source addresses and so this path refers to the
			// root of the git clone, but historically the registry referred
			// to GitHub-provided tar archives which meant that there was an
			// extra level of subdirectory here for the typical directory
			// nesting in tar archives, which would've been reflected as
			// an extra segment on this path. If this test fails due to an
			// additional path segment in future, then a change to the upstream
			// registry might be the root cause.
			LocalPath: filepath.Join(dir, ".terraform/modules/test.main.setup"),
		},

		// main.tftest.hcl.setup.child_a
		// (no download because it's a relative path inside acctest_child_a)
		{
			Name:       "Install",
			ModuleAddr: "test.main.setup.child_a",
			LocalPath:  filepath.Join(dir, ".terraform/modules/test.main.setup/modules/child_a"),
		},

		// main.tftest.hcl.setup.child_a.child_b
		// (no download because it's a relative path inside main.tftest.hcl.setup.child_a)
		{
			Name:       "Install",
			ModuleAddr: "test.main.setup.child_a.child_b",
			LocalPath:  filepath.Join(dir, ".terraform/modules/test.main.setup/modules/child_b"),
		},
	}

	if diff := cmp.Diff(wantCalls, hooks.Calls); diff != "" {
		t.Fatalf("wrong installer calls\n%s", diff)
	}

	// check that the registry responses were cached
	packageAddr := addrs.ModuleRegistryPackage{
		Host:         svchost.Hostname("registry.opentofu.org"),
		Namespace:    "hashicorp",
		Name:         "module-installer-acctest",
		TargetSystem: "aws",
	}
	if _, ok := inst.registryPackageVersions[packageAddr]; !ok {
		t.Errorf("module versions cache was not populated\ngot: %s\nwant: key hashicorp/module-installer-acctest/aws", spew.Sdump(inst.registryPackageVersions))
	}
	if _, ok := inst.registryPackageSources[moduleVersion{module: packageAddr, version: "0.0.1"}]; !ok {
		t.Errorf("module download url cache was not populated\ngot: %s", spew.Sdump(inst.registryPackageSources))
	}

	loader, err = configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfigWithTests(t.Context(), ".", "tests", configs.RootModuleCallForTesting())
	assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags))

	if config.Module.Tests["main.tftest.hcl"].Runs[0].ConfigUnderTest == nil {
		t.Fatalf("should have loaded config into the relevant run block but did not")
	}
}

type testInstallHooks struct {
	Calls []testInstallHookCall
}

type testInstallHookCall struct {
	Name        string
	ModuleAddr  string
	PackageAddr string
	Version     *version.Version
	LocalPath   string
}

func (h *testInstallHooks) Download(moduleAddr, packageAddr string, version *version.Version) {
	h.Calls = append(h.Calls, testInstallHookCall{
		Name:        "Download",
		ModuleAddr:  moduleAddr,
		PackageAddr: packageAddr,
		Version:     version,
	})
}

func (h *testInstallHooks) Install(moduleAddr string, version *version.Version, localPath string) {
	h.Calls = append(h.Calls, testInstallHookCall{
		Name:       "Install",
		ModuleAddr: moduleAddr,
		Version:    version,
		LocalPath:  localPath,
	})
}

// tempChdir copies the contents of the given directory to a temporary
// directory and changes the test process's current working directory to
// point to that directory. The temporary directory is deleted and the
// working directory restored after the calling test is complete.
//
// Tests using this helper cannot safely be run in parallel with other tests.
func tempChdir(t testing.TB, sourceDir string) string {
	t.Helper()

	tmpDir := t.TempDir()
	if err := copy.CopyDir(tmpDir, sourceDir); err != nil {
		t.Fatalf("failed to copy fixture to temporary directory: %s", err)
		return ""
	}
	t.Chdir(tmpDir)

	// Most of the tests need this, so we'll make it just in case.
	if err := os.MkdirAll(filepath.Join(tmpDir, ".terraform/modules"), os.ModePerm); err != nil {
		t.Fatalf("failed to make module cache directory: %s", err)
		return ""
	}

	t.Logf("tempChdir switched to %s after copying from %s", tmpDir, sourceDir)
	return tmpDir
}

func assertNoDiagnostics(t *testing.T, diags tfdiags.Diagnostics) bool {
	t.Helper()
	return assertDiagnosticCount(t, diags, 0)
}

func assertDiagnosticCount(t *testing.T, diags tfdiags.Diagnostics, want int) bool {
	t.Helper()
	if len(diags) != want {
		t.Errorf("wrong number of diagnostics %d; want %d", len(diags), want)
		for _, diag := range diags {
			desc := diag.Description()
			t.Logf("- %s: %s", desc.Summary, desc.Detail)
		}
		return true
	}
	return false
}

func assertDiagnosticSummary(t *testing.T, diags tfdiags.Diagnostics, want string) bool {
	t.Helper()

	for _, diag := range diags {
		if diag.Description().Summary == want {
			return false
		}
	}

	t.Errorf("missing diagnostic summary %q", want)
	for _, diag := range diags {
		desc := diag.Description()
		t.Logf("- %s: %s", desc.Summary, desc.Detail)
	}
	return true
}

func assertResultDeepEqual(t *testing.T, got, want interface{}) bool {
	t.Helper()
	if diff := deep.Equal(got, want); diff != nil {
		for _, problem := range diff {
			t.Errorf("%s", problem)
		}
		return true
	}
	return false
}
