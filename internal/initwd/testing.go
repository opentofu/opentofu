// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package initwd

import (
	"context"
	"testing"

	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/registry"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// LoadConfigForTests is a convenience wrapper around configload.NewLoaderForTests,
// ModuleInstaller.InstallModules and configload.Loader.LoadConfig that allows
// a test configuration to be loaded in a single step.
//
// If module installation fails, t.Fatal (or similar) is called to halt
// execution of the test, under the assumption that installation failures are
// not expected. If installation failures _are_ expected then use
// NewLoaderForTests and work with the loader object directly. If module
// installation succeeds but generates warnings, these warnings are discarded.
//
// If installation succeeds but errors are detected during loading then a
// possibly-incomplete config is returned along with error diagnostics. The
// test run is not aborted in this case, so that the caller can make assertions
// against the returned diagnostics.
func LoadConfigForTests(t testing.TB, rootDir string, testsDir string) (*configs.Config, *configload.Loader, tfdiags.Diagnostics) {
	t.Helper()

	var diags tfdiags.Diagnostics

	loader := configload.NewLoaderForTests(t)
	inst := NewModuleInstaller(loader.ModulesDir(), loader, registry.NewClient(nil, nil), nil)

	call := configs.RootModuleCallForTesting()
	_, moreDiags := inst.InstallModules(context.Background(), rootDir, testsDir, true, false, ModuleInstallHooksImpl{}, call)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
		return nil, nil, diags
	}

	// Since module installer has modified the module manifest on disk, we need
	// to refresh the cache of it in the loader.
	if err := loader.RefreshModules(); err != nil {
		t.Fatalf("failed to refresh modules after installation: %s", err)
	}

	config, hclDiags := loader.LoadConfig(rootDir, call)
	diags = diags.Append(hclDiags)
	return config, loader, diags
}

// MustLoadConfigForTests is a variant of LoadConfigForTests which calls
// t.Fatal (or similar) if there are any errors during loading, and thus
// does not return diagnostics at all.
//
// This is useful for concisely writing tests that don't expect errors at
// all. For tests that expect errors and need to assert against them, use
// LoadConfigForTests instead.
func MustLoadConfigForTests(t testing.TB, rootDir, testsDir string) (*configs.Config, *configload.Loader) {
	t.Helper()

	config, loader, diags := LoadConfigForTests(t, rootDir, testsDir)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}
	return config, loader
}
