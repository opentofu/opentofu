// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/modsdir"
)

func TestLazyLoader_Initialize(t *testing.T) {
	t.Run("successful w/o manifest", func(t *testing.T) {
		d := t.TempDir()
		writeConfigFile(t, filepath.Join(d, "main.tf"), []byte(""))

		ll := NewLazy(&Config{ModulesDir: d})
		il, err := Initialise(ll)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		// do a sanity call to ensure that the returned loader works as expected
		if ok := il.IsConfigDir(d); !ok {
			t.Errorf("expected for the current directory to be a config directory but got %t", ok)
		}
	})
	t.Run("successful with manifest", func(t *testing.T) {
		d := t.TempDir()
		writeConfigFile(t, filepath.Join(d, "main.tf"), []byte(""))
		writeManifestFile(t, d, []byte(`{"Modules":[]}`))

		ll := NewLazy(&Config{ModulesDir: d})
		il, err := Initialise(ll)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		// do a sanity call to ensure that the returned loader works as expected
		if ok := il.IsConfigDir(d); !ok {
			t.Errorf("expected for the current directory to be a config directory but got %t", ok)
		}
	})
	t.Run("fails when manifest is malformed", func(t *testing.T) {
		d := t.TempDir()
		writeConfigFile(t, filepath.Join(d, "main.tf"), []byte(""))
		writeManifestFile(t, d, []byte("invalid content"))

		ll := NewLazy(&Config{ModulesDir: d})
		il, err := Initialise(ll)
		if err == nil {
			t.Fatalf("expected but got nil")
		}
		expectedErr := "failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
		if err.Error() != expectedErr {
			t.Errorf("the returned error is different from the expected one\nwanted: %s\ngot: %s\n", expectedErr, err.Error())
		}
		if il != nil {
			t.Errorf("expected the returned loader to be nil because of the error in the initialisation")
		}
	})
}

func TestLoaderInitializationDuringMethodCalls(t *testing.T) {
	cases := map[string]struct {
		setup func(t *testing.T, wd string)
		act   func(t *testing.T, wd string, l Loader)
	}{
		"loader ok - import sources": {
			setup: func(t *testing.T, wd string) {},
			act: func(t *testing.T, wd string, l Loader) {
				l.ImportSources(map[string][]byte{"f": []byte("content")})
				c, ok := l.Sources()["f"]
				if !ok {
					t.Fatalf("expected to have a file in the sources but got nothing")
				}
				if diff := cmp.Diff([]byte("content"), c.Bytes); diff != "" {
					t.Errorf("invalid content returned from the loader sources (-want,+got):\n%s", diff)
				}
			},
		},
		"loader uninitializable - import sources": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				l.ImportSources(map[string][]byte{"f": []byte("content")})
				if len(l.Sources()) > 0 {
					t.Fatalf("unexpected content in the loader sources. Importin sources in a broken loader should not work")
				}
			},
		},
		"loader ok - ImportSourcesFromSnapshot": {
			setup: func(t *testing.T, wd string) {},
			act: func(t *testing.T, wd string, l Loader) {
				l.ImportSourcesFromSnapshot(&Snapshot{
					Modules: map[string]*SnapshotModule{
						"m1": {
							Dir:   "m1",
							Files: map[string][]byte{"f": []byte("content")},
						},
					},
				})
				c, ok := l.Sources()[filepath.Join("m1", "f")]
				if !ok {
					t.Fatalf("expected to have a file in the sources but got nothing. sources: %v", l.Sources())
				}
				if diff := cmp.Diff([]byte("content"), c.Bytes); diff != "" {
					t.Errorf("invalid content returned from the loader sources (-want,+got):\n%s", diff)
				}
			},
		},
		"loader uninitializable - ImportSourcesFromSnapshot": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				l.ImportSourcesFromSnapshot(&Snapshot{
					Modules: map[string]*SnapshotModule{
						"m1": {
							Dir:   "m1",
							Files: map[string][]byte{"f": []byte("content")},
						},
					},
				})
				if len(l.Sources()) > 0 {
					t.Fatalf("unexpected content in the loader sources. Importin sources in a broken loader should not work")
				}
			},
		},
		"loader ok - IsRemoteModuleSource": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`module "test" { source = "test1/test2/test3"}`))
				writeConfigFile(t, filepath.Join(wd, ".terraform", "modules", "test1", "main.tf"), []byte(`variable "in" {}`))
				writeManifestFile(t, wd, []byte(`{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"test","Source":"registry.opentofu.org/test1/test2/test3","Dir":".terraform/modules/test1"}]}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfig(t.Context(), wd, configs.StaticModuleCall{})
				if diags.HasErrors() {
					t.Errorf("unexpected error: %s", diags)
				}
				if c == nil {
					t.Errorf("expected non-nil config but got nil back")
				}
				if got := l.IsRemoteModuleSource(addrs.Module{"test"}); !got {
					t.Errorf("expected the module to be reported as remote but it was not")
				}
			},
		},
		"loader uninitializable - IsRemoteModuleSource": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte(`invalid content`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				if got := l.IsRemoteModuleSource(addrs.Module{"test"}); got {
					t.Errorf("expected the module to be reported as not being remote but it was true")
				}
			},
		},
		"loader ok - ModuleSourceAddrs": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`module "test" { source = "test1/test2/test3"}`))
				writeConfigFile(t, filepath.Join(wd, ".terraform", "modules", "test1", "main.tf"), []byte(`variable "in" {}`))
				writeManifestFile(t, wd, []byte(`{"Modules":[{"Key":"","Source":"","Dir":"."},{"Key":"test","Source":"registry.opentofu.org/test1/test2/test3","Dir":".terraform/modules/test1"}]}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfig(t.Context(), wd, configs.StaticModuleCall{})
				if diags.HasErrors() {
					t.Errorf("unexpected error: %s", diags)
				}
				if c == nil {
					t.Errorf("expected non-nil config but got nil back")
				}
				want := addrs.MustParseModuleSource("test1/test2/test3")
				got := l.ModuleSourceAddrs(addrs.Module{"test"})
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("invalid module source returned (-want,+got):\n%s", diff)
				}
			},
		},
		"loader uninitializable - ModuleSourceAddrs": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte(`invalid content`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				got := l.ModuleSourceAddrs(addrs.Module{"test"})
				want := addrs.ModuleSource(nil)
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("invalid module source returned (-want,+got):\n%s", diff)
				}
			},
		},
		"loader ok - ForceFileSource": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				l.ForceFileSource("f", []byte("content"))
				c, ok := l.Sources()["f"]
				if !ok {
					t.Fatalf("expected to have a file in the sources but got nothing")
				}
				if diff := cmp.Diff([]byte("content"), c.Bytes); diff != "" {
					t.Errorf("invalid content returned from the loader sources (-want,+got):\n%s", diff)
				}
			},
		},
		"loader uninitializable - ForceFileSource": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				l.ForceFileSource("f", []byte("content"))
				if len(l.Sources()) > 0 {
					t.Fatalf("unexpected content in the loader sources. Forcing file sources in a broken loader should not work")
				}
			},
		},
		"loader ok - IsConfigDir - false": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				if ok := l.IsConfigDir("unexisting"); ok {
					t.Errorf("expected to be false but was true")
				}
			},
		},
		"loader ok - IsConfigDir - true": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				if ok := l.IsConfigDir(wd); !ok {
					t.Errorf("expected to be true but was false")
				}
			},
		},
		"loader uninitializable - IsConfigDir": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				if ok := l.IsConfigDir(wd); !ok {
					t.Errorf("expected to be true (as default because initialization of the loader should have failed) but was false")
				}
			},
		},
		"loader ok - ModulesDir": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				if dir := l.ModulesDir(); dir != wd {
					t.Errorf("expected %q dir but got %q", wd, dir)
				}
			},
		},
		"loader uninitializable - ModulesDir": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				if dir := l.ModulesDir(); dir != "" {
					t.Errorf("expected %q dir but got %q", "", dir)
				}
			},
		},
		"loader ok - RefreshModules": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				if err := l.RefreshModules(); err != nil {
					t.Errorf("expected no error but got %s", err)
				}
			},
		},
		"loader uninitializable - RefreshModules": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				err := l.RefreshModules()
				if err == nil {
					t.Fatalf("expected error but got nothing")
				}
				expectedErrMsg := "failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(err.Error(), expectedErrMsg) {
					t.Errorf("expected error to contain %q but didn't: %s", expectedErrMsg, err)
				}
			},
		},
		"loader ok - LoadConfig": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfig(t.Context(), wd, configs.StaticModuleCall{})
				if diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				if _, ok := c.Module.Variables["in"]; !ok {
					t.Errorf("expected to have a variable named 'in' but got nothing. variables: %v", c.Module.Variables)
				}
			},
		},
		"loader uninitializable - LoadConfig": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfig(t.Context(), wd, configs.StaticModuleCall{})
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if c != nil {
					t.Errorf("expected no config but got a non-nil one")
				}
			},
		},
		"loader ok - LoadConfigWithTests": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
				writeConfigFile(t, filepath.Join(wd, "main.tftest.hcl"), []byte(``))
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfigWithTests(t.Context(), wd, ".", configs.StaticModuleCall{})
				if diags.HasErrors() {
					t.Fatalf("expected to have no diagnostics but got: %s", diags)
				}
				if _, ok := c.Module.Variables["in"]; !ok {
					t.Errorf("expected to have a variable named 'in' but got nothing. variables: %v", c.Module.Variables)
				}
			},
		},
		"loader uninitializable - LoadConfigWithTests": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfigWithTests(t.Context(), wd, wd, configs.StaticModuleCall{})
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if c != nil {
					t.Errorf("expected no config but got a non-nil one")
				}
			},
		},
		"loader ok - LoadConfigWithSnapshot": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, snap, diags := l.LoadConfigWithSnapshot(t.Context(), wd, configs.StaticModuleCall{})
				if diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				if _, ok := c.Module.Variables["in"]; !ok {
					t.Errorf("expected to have a variable named 'in' but got nothing. variables: %v", c.Module.Variables)
				}
				if snap == nil {
					t.Fatalf("unexpected nil snapshot")
				}
				mod, ok := snap.Modules[addrs.RootModule.String()]
				if !ok {
					t.Fatalf("expected for the snapshot to have the root module but got nothing")
				}
				content, ok := mod.Files["main.tf"]
				if !ok {
					t.Fatalf("expected the snapshot to contain a root module with one file but the file is missing")
				}
				if diff := cmp.Diff([]byte(`variable "in" {}`), content); diff != "" {
					t.Errorf("invalid content returned from the loader sources (-want,+got):\n%s", diff)
				}
			},
		},
		"loader uninitializable - LoadConfigWithSnapshot": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, snap, diags := l.LoadConfigWithSnapshot(t.Context(), wd, configs.StaticModuleCall{})
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if c != nil {
					t.Errorf("expected no config but got a non-nil one")
				}
				if snap != nil {
					t.Errorf("expected no snapshot but got a non-nil one")
				}
			},
		},
		"loader ok - LoadConfigDirUneval": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfigDirUneval(wd, configs.SelectiveLoadAll)
				if diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				if _, ok := c.Variables["in"]; !ok {
					t.Errorf("expected to have a variable named 'in' but got nothing. variables: %v", c.Variables)
				}
			},
		},
		"loader uninitializable - LoadConfigDirUneval": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfigDirUneval(wd, configs.SelectiveLoadAll)
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if c != nil {
					t.Errorf("expected no config but got a non-nil one")
				}
			},
		},
		"loader ok - LoadConfigDir": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfigDir(wd, configs.StaticModuleCall{})
				if diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				if _, ok := c.Variables["in"]; !ok {
					t.Errorf("expected to have a variable named 'in' but got nothing. variables: %v", c.Variables)
				}
			},
		},
		"loader uninitializable - LoadConfigDir": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadConfigDir(wd, configs.StaticModuleCall{})
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if c != nil {
					t.Errorf("expected no config but got a non-nil one")
				}
			},
		},
		"loader ok - LoadHCLFile": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				body, diags := l.LoadHCLFile(filepath.Join(wd, "main.tf"))
				if diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				if body == nil {
					t.Fatalf("expected for the hcl file to have an actual body but got nothing")
				}
			},
		},
		"loader uninitializable - LoadHCLFile": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				c, diags := l.LoadHCLFile(filepath.Join(wd, "main.tf"))
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if c != nil {
					t.Errorf("expected no config but got a non-nil one")
				}
			},
		},
		"loader ok - LoadConfigDirSelective": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				m, diags := l.LoadConfigDirSelective(wd, configs.StaticModuleCall{}, configs.SelectiveLoadAll)
				if diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				if m == nil {
					t.Fatalf("expected for the hcl file to have an actual body but got nothing")
				}
				if _, ok := m.Variables["in"]; !ok {
					t.Errorf("expected to have a variable named 'in' but got nothing. variables: %v", m.Variables)
				}
			},
		},
		"loader uninitializable - LoadConfigDirSelective": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				m, diags := l.LoadConfigDirSelective(wd, configs.StaticModuleCall{}, configs.SelectiveLoadAll)
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if m != nil {
					t.Errorf("expected no module but got a non-nil one")
				}
			},
		},
		"loader ok - LoadConfigDirWithTests": {
			setup: func(t *testing.T, wd string) {
				writeConfigFile(t, filepath.Join(wd, "main.tf"), []byte(`variable "in" {}`))
			},
			act: func(t *testing.T, wd string, l Loader) {
				m, diags := l.LoadConfigDirWithTests(wd, ".", configs.StaticModuleCall{})
				if diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				if m == nil {
					t.Fatalf("expected for the hcl file to have an actual body but got nothing")
				}
				if _, ok := m.Variables["in"]; !ok {
					t.Errorf("expected to have a variable named 'in' but got nothing. variables: %v", m.Variables)
				}
			},
		},
		"loader uninitializable - LoadConfigDirWithTests": {
			setup: func(t *testing.T, wd string) {
				writeManifestFile(t, wd, []byte("invalid content")) // to break the loader initialization
			},
			act: func(t *testing.T, wd string, l Loader) {
				m, diags := l.LoadConfigDirWithTests(wd, ".", configs.StaticModuleCall{})
				if !diags.HasErrors() {
					t.Errorf("expected to have no diagnostics but got: %s", diags)
				}
				expectedErrMsg := "Failed to initialise a config loader: failed to read module manifest: error unmarshalling snapshot: invalid character 'i' looking for beginning of value"
				if !strings.Contains(diags.Error(), expectedErrMsg) {
					t.Errorf("expected the diags to contain %q but it didn't: %s", expectedErrMsg, diags)
				}
				if m != nil {
					t.Errorf("expected no module but got a non-nil one")
				}
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			t.Chdir(d)
			tc.setup(t, d)
			ll := NewLazy(&Config{ModulesDir: d})
			tc.act(t, d, ll)
		})
	}
}

func writeManifestFile(t *testing.T, toDir string, content []byte) {
	if err := os.WriteFile(filepath.Join(toDir, modsdir.ManifestSnapshotFilename), content, 0666); err != nil {
		t.Fatalf("unexpected error writing the modules manifest file: %s", err)
	}
}

func writeConfigFile(t *testing.T, path string, content []byte) {
	finalDir := filepath.Dir(path)
	if err := os.MkdirAll(finalDir, 0766); err != nil {
		t.Fatalf("failed to create the final target directory %q: %s", finalDir, err)
	}
	if err := os.WriteFile(path, content, 0666); err != nil {
		t.Fatalf("unexpected error writing configuration file: %s", err)
	}
}
