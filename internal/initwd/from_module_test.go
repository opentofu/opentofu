// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package initwd

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	version "github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/copy"
	"github.com/opentofu/opentofu/internal/getmodules"
	"github.com/opentofu/opentofu/internal/registry"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestDirFromModule_registry(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("this test accesses registry.opentofu.org and github.com; set TF_ACC=1 to run it")
	}

	fixtureDir := filepath.Clean("testdata/empty")
	tmpDir := tempChdir(t, fixtureDir)

	// the module installer runs filepath.EvalSymlinks() on the destination
	// directory before copying files, and the resultant directory is what is
	// returned by the install hooks. Without this, tests could fail on machines
	// where the default temp dir was a symlink.
	dir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Error(err)
	}
	modsDir := filepath.Join(dir, ".terraform/modules")

	hooks := &testInstallHooks{}

	reg := registry.NewClient(nil, nil)
	loader := configload.NewLoaderForTests(t)
	diags := DirFromModule(context.Background(), loader, dir, modsDir, "hashicorp/module-installer-acctest/aws//examples/main", reg, nil, hooks)
	assertNoDiagnostics(t, diags)

	v := version.Must(version.NewVersion("0.0.2"))

	wantCalls := []testInstallHookCall{
		// The module specified to populate the root directory is not mentioned
		// here, because the hook mechanism is defined to talk about descendent
		// modules only and so a caller to InitDirFromModule is expected to
		// produce its own user-facing announcement about the root module being
		// installed.

		// Note that "root" in the following examples is, confusingly, the
		// label on the module block in the example we've installed here:
		//     module "root" {

		{
			Name:        "Download",
			ModuleAddr:  "root",
			PackageAddr: "registry.opentofu.org/hashicorp/module-installer-acctest/aws",
			Version:     v,
		},
		{
			Name:       "Install",
			ModuleAddr: "root",
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
			LocalPath: filepath.Join(dir, ".terraform/modules/root"),
		},
		{
			Name:       "Install",
			ModuleAddr: "root.child_a",
			LocalPath:  filepath.Join(dir, ".terraform/modules/root/modules/child_a"),
		},
		{
			Name:       "Install",
			ModuleAddr: "root.child_a.child_b",
			LocalPath:  filepath.Join(dir, ".terraform/modules/root/modules/child_b"),
		},
	}

	if diff := cmp.Diff(wantCalls, hooks.Calls); diff != "" {
		t.Fatalf("wrong installer calls\n%s", diff)
	}

	loader, err = configload.NewLoader(&configload.Config{
		ModulesDir: modsDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfig(".", configs.RootModuleCallForTesting())
	if assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags)) {
		return
	}

	wantTraces := map[string]string{
		"":                     "in example",
		"root":                 "in root module",
		"root.child_a":         "in child_a module",
		"root.child_a.child_b": "in child_b module",
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

func TestDirFromModule_submodules(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/empty")
	fromModuleDir, err := filepath.Abs("./testdata/local-modules")
	if err != nil {
		t.Fatal(err)
	}

	// DirFromModule will expand ("canonicalize") the pathnames, so we must do
	// the same for our "wantCalls" comparison values. Otherwise this test
	// will fail when building in a source tree with symlinks in $PWD.
	//
	// See also: https://github.com/hashicorp/terraform/issues/26014
	//
	fromModuleDirRealpath, err := filepath.EvalSymlinks(fromModuleDir)
	if err != nil {
		t.Error(err)
	}

	tmpDir := tempChdir(t, fixtureDir)

	hooks := &testInstallHooks{}
	dir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Error(err)
	}
	modInstallDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	diags := DirFromModule(
		context.Background(),
		loader,
		dir,
		modInstallDir,
		fromModuleDir,
		nil,
		// This test relies on the module installer's legacy support for
		// treating an absolute filesystem path as if it were a "remote"
		// source address, and so we need a real package fetcher but the
		// way we use it here does not cause it to make network requests.
		getmodules.NewPackageFetcher(nil),
		hooks,
	)
	assertNoDiagnostics(t, diags)
	wantCalls := []testInstallHookCall{
		{
			Name:       "Install",
			ModuleAddr: "child_a",
			LocalPath:  filepath.Join(fromModuleDirRealpath, "child_a"),
		},
		{
			Name:       "Install",
			ModuleAddr: "child_a.child_b",
			LocalPath:  filepath.Join(fromModuleDirRealpath, "child_a/child_b"),
		},
	}

	if assertResultDeepEqual(t, hooks.Calls, wantCalls) {
		return
	}

	loader, err = configload.NewLoader(&configload.Config{
		ModulesDir: modInstallDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfig(".", configs.RootModuleCallForTesting())
	if assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags)) {
		return
	}
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

// submodulesWithProvider is identical to above, except that the configuration
// would fail to load for some reason. We still want the module to be installed
// for use cases like testing or CDKTF, and will only emit warnings for config
// errors.
func TestDirFromModule_submodulesWithProvider(t *testing.T) {
	fixtureDir := filepath.Clean("testdata/empty")
	fromModuleDir, err := filepath.Abs("./testdata/local-module-missing-provider")
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := tempChdir(t, fixtureDir)
	hooks := &testInstallHooks{}
	dir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Error(err)
	}
	modInstallDir := filepath.Join(dir, ".terraform/modules")

	loader := configload.NewLoaderForTests(t)
	diags := DirFromModule(
		context.Background(),
		loader,
		dir,
		modInstallDir,
		fromModuleDir,
		nil,
		// This test relies on the module installer's legacy support for
		// treating an absolute filesystem path as if it were a "remote"
		// source address, and so we need a real package fetcher but the
		// way we use it here does not cause it to make network requests.
		getmodules.NewPackageFetcher(nil),
		hooks,
	)

	for _, d := range diags {
		if d.Severity() != tfdiags.Warning {
			t.Errorf("expected warning, got %v", diags.Err())
		}
	}
}

// TestDirFromModule_rel_submodules is similar to the test above, but the
// from-module is relative to the install dir ("../"):
// https://github.com/hashicorp/terraform/issues/23010
func TestDirFromModule_rel_submodules(t *testing.T) {
	// This test creates a tmpdir with the following directory structure:
	// - tmpdir/local-modules (with contents of testdata/local-modules)
	// - tmpdir/empty: the workDir we CD into for the test
	// - tmpdir/empty/target (target, the destination for init -from-module)
	tmpDir := t.TempDir()
	fromModuleDir := filepath.Join(tmpDir, "local-modules")
	workDir := filepath.Join(tmpDir, "empty")
	if err := os.Mkdir(fromModuleDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := copy.CopyDir(fromModuleDir, "testdata/local-modules"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(tmpDir, "target")
	if err := os.Mkdir(targetDir, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chdir(targetDir)
	if err != nil {
		t.Fatalf("failed to switch to temp dir %s: %s", tmpDir, err)
	}
	t.Cleanup(func() {
		os.Chdir(oldDir)
		// Trigger garbage collection to ensure that all open file handles are closed.
		// This prevents TempDir RemoveAll cleanup errors on Windows.
		if runtime.GOOS == "windows" {
			runtime.GC()
		}
	})

	hooks := &testInstallHooks{}

	modInstallDir := ".terraform/modules"
	sourceDir := "../local-modules"
	loader := configload.NewLoaderForTests(t)
	diags := DirFromModule(
		context.Background(),
		loader, ".",
		modInstallDir,
		sourceDir,
		nil,
		// This test relies on the module installer's legacy support for
		// treating an absolute filesystem path as if it were a "remote"
		// source address, and so we need a real package fetcher but the
		// way we use it here does not cause it to make network requests.
		getmodules.NewPackageFetcher(nil),
		hooks,
	)
	assertNoDiagnostics(t, diags)
	wantCalls := []testInstallHookCall{
		{
			Name:       "Install",
			ModuleAddr: "child_a",
			LocalPath:  filepath.Join(sourceDir, "child_a"),
		},
		{
			Name:       "Install",
			ModuleAddr: "child_a.child_b",
			LocalPath:  filepath.Join(sourceDir, "child_a/child_b"),
		},
	}

	if assertResultDeepEqual(t, hooks.Calls, wantCalls) {
		return
	}

	loader, err = configload.NewLoader(&configload.Config{
		ModulesDir: modInstallDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure the configuration is loadable now.
	// (This ensures that correct information is recorded in the manifest.)
	config, loadDiags := loader.LoadConfig(".", configs.RootModuleCallForTesting())
	if assertNoDiagnostics(t, tfdiags.Diagnostics{}.Append(loadDiags)) {
		return
	}
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
