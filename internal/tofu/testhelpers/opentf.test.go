// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testhelpers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/initwd"
	"github.com/opentofu/opentofu/internal/registry"
)

// This is the directory where our test fixtures are.
const fixtureDir = "./testdata"

func TestModule(t testing.TB, name string) *configs.Config {
	t.Helper()
	c, _ := TestModuleWithSnapshot(t, name)
	return c
}

func TestModuleWithSnapshot(t testing.TB, name string) (*configs.Config, *configload.Snapshot) {
	t.Helper()

	dir := filepath.Join(fixtureDir, name)
	loader := configload.NewLoaderForTests(t)

	// We need to be able to exercise experimental features in our integration tests.
	loader.AllowLanguageExperiments(true)

	// Test modules usually do not refer to remote sources, and for local
	// sources only this ultimately just records all of the module paths
	// in a JSON file so that we can load them below.
	inst := initwd.NewModuleInstaller(loader.ModulesDir(), loader, registry.NewClient(t.Context(), nil, nil), nil)
	_, instDiags := inst.InstallModules(context.Background(), dir, "tests", true, false, initwd.ModuleInstallHooksImpl{}, configs.RootModuleCallForTesting())
	if instDiags.HasErrors() {
		t.Fatal(instDiags.Err())
	}

	// Since module installer has modified the module manifest on disk, we need
	// to refresh the cache of it in the loader.
	if err := loader.RefreshModules(); err != nil {
		t.Fatalf("failed to refresh modules after installation: %s", err)
	}

	config, snap, diags := loader.LoadConfigWithSnapshot(t.Context(), dir, configs.RootModuleCallForTesting())
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	return config, snap
}

// testModuleInline takes a map of path -> config strings and yields a config
// structure with those files loaded from disk
func TestModuleInline(t testing.TB, sources map[string]string) *configs.Config {
	t.Helper()

	cfgPath := t.TempDir()

	for path, configStr := range sources {
		dir := filepath.Dir(path)
		if dir != "." {
			err := os.MkdirAll(filepath.Join(cfgPath, dir), os.FileMode(0777))
			if err != nil {
				t.Fatalf("Error creating subdir: %s", err)
			}
		}
		// Write the configuration
		cfgF, err := os.Create(filepath.Join(cfgPath, path))
		if err != nil {
			t.Fatalf("Error creating temporary file for config: %s", err)
		}

		_, err = io.Copy(cfgF, strings.NewReader(configStr))
		cfgF.Close()
		if err != nil {
			t.Fatalf("Error creating temporary file for config: %s", err)
		}
	}

	loader := configload.NewLoaderForTests(t)

	// We need to be able to exercise experimental features in our integration tests.
	loader.AllowLanguageExperiments(true)

	// Test modules usually do not refer to remote sources, and for local
	// sources only this ultimately just records all of the module paths
	// in a JSON file so that we can load them below.
	inst := initwd.NewModuleInstaller(loader.ModulesDir(), loader, registry.NewClient(t.Context(), nil, nil), nil)
	_, instDiags := inst.InstallModules(context.Background(), cfgPath, "tests", true, false, initwd.ModuleInstallHooksImpl{}, configs.RootModuleCallForTesting())
	if instDiags.HasErrors() {
		t.Fatal(instDiags.Err())
	}

	// Since module installer has modified the module manifest on disk, we need
	// to refresh the cache of it in the loader.
	if err := loader.RefreshModules(); err != nil {
		t.Fatalf("failed to refresh modules after installation: %s", err)
	}

	config, diags := loader.LoadConfigWithTests(t.Context(), cfgPath, "tests", configs.RootModuleCallForTesting())
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	return config
}

func MustResourceInstanceAddr(s string) addrs.AbsResourceInstance {
	addr, diags := addrs.ParseAbsResourceInstanceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr
}

func MustConfigResourceAddr(s string) addrs.ConfigResource {
	addr, diags := addrs.ParseAbsResourceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr.Config()
}

func MustAbsResourceAddr(s string) addrs.AbsResource {
	addr, diags := addrs.ParseAbsResourceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr
}

func MustProviderConfig(s string) addrs.AbsProviderConfig {
	p, diags := addrs.ParseAbsProviderConfigStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return p
}

func MustReference(s string) *addrs.Reference {
	p, diags := addrs.ParseRefStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return p
}
