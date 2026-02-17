// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/copy"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestImport(t *testing.T) {
	t.Chdir(testFixturePath("import-provider-implicit"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_providerConfig(t *testing.T) {
	t.Chdir(testFixturePath("import-provider"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
	}

	configured := false
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
		configured = true

		cfg := req.Config
		if !cfg.Type().HasAttribute("foo") {
			return providers.ConfigureProviderResponse{
				Diagnostics: tfdiags.Diagnostics{}.Append(fmt.Errorf("configuration has no foo argument")),
			}
		}
		if got, want := cfg.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			return providers.ConfigureProviderResponse{
				Diagnostics: tfdiags.Diagnostics{}.Append(fmt.Errorf("foo argument is %#v, but want %#v", got, want)),
			}
		}

		return providers.ConfigureProviderResponse{}
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

// "remote" state provided by the "local" backend
func TestImport_remoteState(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("import-provider-remote-state"), td)
	t.Chdir(td)

	statePath := "imported.tfstate"

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": []string{"1.2.3"},
	})
	defer close()

	// init our backend
	initView, initDone := testView(t)
	m := Meta{
		WorkingDir:       workdir.NewDir("."),
		testingOverrides: metaOverridesForProvider(testProvider()),
		View:             initView,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}

	// (Using log here rather than t.Log so that these messages interleave with other trace logs)
	log.Print("[TRACE] TestImport_remoteState running: tofu init")
	code := ic.Run([]string{})
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("init failed\n%s", initOutput.Stderr())
	}

	p := testProvider()
	importView, importDone := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             importView,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
	}

	configured := false
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureProviderResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"test_instance.foo",
		"bar",
	}
	log.Printf("[TRACE] TestImport_remoteState running: tofu import %s %s", args[0], args[1])
	code = c.Run(args)
	importOutput := importDone(t)
	if code != 0 {
		fmt.Println(importOutput.Stdout())
		t.Fatalf("bad: %d\n\n%s", code, importOutput.Stderr())
	}

	// verify that the local state was unlocked after import
	if _, err := os.Stat(filepath.Join(td, fmt.Sprintf(".%s.lock.info", statePath))); !os.IsNotExist(err) {
		t.Fatal("state left locked after import")
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

// early failure on import should not leave stale lock
func TestImport_initializationErrorShouldUnlock(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("import-provider-remote-state"), td)
	t.Chdir(td)

	statePath := "imported.tfstate"

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": []string{"1.2.3"},
	})
	defer close()

	// init our backend
	initView, initDone := testView(t)
	m := Meta{
		WorkingDir:       workdir.NewDir("."),
		testingOverrides: metaOverridesForProvider(testProvider()),
		View:             initView,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}

	// (Using log here rather than t.Log so that these messages interleave with other trace logs)
	log.Print("[TRACE] TestImport_initializationErrorShouldUnlock running: tofu init")
	code := ic.Run([]string{})
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("init failed\n%s", initOutput.Stderr())
	}

	// overwrite the config with one including a resource from an invalid provider
	if err := copy.CopyFile(filepath.Join(testFixturePath("import-provider-invalid"), "main.tf"), filepath.Join(td, "main.tf")); err != nil {
		t.Fatal(err)
	}

	p := testProvider()
	importView, importDone := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             importView,
		},
	}

	args := []string{
		"unknown_instance.baz",
		"bar",
	}
	log.Printf("[TRACE] TestImport_initializationErrorShouldUnlock running: tofu import %s %s", args[0], args[1])

	// this should fail
	code = c.Run(args)
	importOutput := importDone(t)
	if code != 1 {
		fmt.Println(importOutput.Stdout())
		t.Fatalf("bad: %d\n\n%s", code, importOutput.Stderr())
	}

	// specifically, it should fail due to a missing provider
	msg := strings.ReplaceAll(importOutput.Stderr(), "\n", " ")
	if want := `provider registry.opentofu.org/hashicorp/unknown: required by this configuration but no version is selected`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}

	// verify that the local state was unlocked after initialization error
	if _, err := os.Stat(filepath.Join(td, fmt.Sprintf(".%s.lock.info", statePath))); !os.IsNotExist(err) {
		t.Fatal("state left locked after import")
	}
}

func TestImport_providerConfigWithVar(t *testing.T) {
	t.Chdir(testFixturePath("import-provider-var"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
	}

	configured := false
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureProviderResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"-state", statePath,
		"-var", "foo=bar",
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_providerConfigWithDataSource(t *testing.T) {
	t.Chdir(testFixturePath("import-provider-datasource"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
		DataSources: map[string]providers.Schema{
			"test_data": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("bad, wanted error: %d\n\n%s", code, output.Stderr())
	}
}

func TestImport_providerConfigWithVarDefault(t *testing.T) {
	t.Chdir(testFixturePath("import-provider-var-default"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
	}

	configured := false
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureProviderResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_providerConfigWithVarFile(t *testing.T) {
	t.Chdir(testFixturePath("import-provider-var-file"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	p.ImportResourceStateFn = nil
	p.ImportResourceStateResponse = &providers.ImportResourceStateResponse{
		ImportedResources: []providers.ImportedResource{
			{
				TypeName: "test_instance",
				State: cty.ObjectVal(map[string]cty.Value{
					"id": cty.StringVal("yay"),
				}),
			},
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		Provider: providers.Schema{
			Block: &configschema.Block{
				Attributes: map[string]*configschema.Attribute{
					"foo": {Type: cty.String, Optional: true},
				},
			},
		},
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
	}

	configured := false
	p.ConfigureProviderFn = func(req providers.ConfigureProviderRequest) providers.ConfigureProviderResponse {
		var diags tfdiags.Diagnostics
		configured = true
		if got, want := req.Config.GetAttr("foo"), cty.StringVal("bar"); !want.RawEquals(got) {
			diags = diags.Append(fmt.Errorf("wrong \"foo\" value %#v; want %#v", got, want))
		}
		return providers.ConfigureProviderResponse{
			Diagnostics: diags,
		}
	}

	args := []string{
		"-state", statePath,
		"-var-file", "blah.tfvars",
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Verify that we were called
	if !configured {
		t.Fatal("Configure should be called")
	}

	if !p.ImportResourceStateCalled {
		t.Fatal("ImportResourceState should be called")
	}

	testStateOutput(t, statePath, testImportStr)
}

func TestImport_emptyConfig(t *testing.T) {
	t.Chdir(testFixturePath("empty"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := output.Stderr()
	if want := `No OpenTofu configuration files`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_missingResourceConfig(t *testing.T) {
	t.Chdir(testFixturePath("import-missing-resource-config"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := output.Stderr()
	if want := `resource address "test_instance.foo" does not exist`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_missingModuleConfig(t *testing.T) {
	t.Chdir(testFixturePath("import-missing-resource-config"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-state", statePath,
		"module.baz.test_instance.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := output.Stderr()
	if want := `module.baz is not defined in the configuration`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImportModuleVarFile(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("import-module-var-file"), td)
	t.Chdir(td)

	statePath := testTempFile(t)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": []string{"1.2.3"},
	})
	defer close()

	// init to install the module
	initView, initDone := testView(t)
	m := Meta{
		WorkingDir:       workdir.NewDir("."),
		testingOverrides: metaOverridesForProvider(testProvider()),
		View:             initView,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}
	code := ic.Run([]string{})
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("init failed\n%s", initOutput.Stderr())
	}

	// import
	importView, importDone := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             importView,
		},
	}
	args := []string{
		"-state", statePath,
		"module.child.test_instance.foo",
		"bar",
	}
	code = c.Run(args)
	importOutput := importDone(t)
	if code != 0 {
		t.Fatalf("import failed; expected success. %s", importOutput.All())
	}
}

// This test covers an edge case where a module with a complex input variable
// of nested objects has an invalid default which is overridden by the calling
// context, and is used in locals. If we don't evaluate module call variables
// for the import walk, this results in an error.
//
// The specific example has a variable "foo" which is a nested object:
//
//	foo = { bar = { baz = true } }
//
// This is used as foo = var.foo in the call to the child module, which then
// uses the traversal foo.bar.baz in a local. A default value in the child
// module of {} causes this local evaluation to error, breaking import.
func TestImportModuleInputVariableEvaluation(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("import-module-input-variable"), td)
	t.Chdir(td)

	statePath := testTempFile(t)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	providerSource, close := newMockProviderSource(t, map[string][]string{
		"test": {"1.2.3"},
	})
	defer close()

	// init to install the module
	initView, initDone := testView(t)
	m := Meta{
		WorkingDir:       workdir.NewDir("."),
		testingOverrides: metaOverridesForProvider(testProvider()),
		View:             initView,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}
	code := ic.Run([]string{})
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("init failed\n%s", initOutput.Stderr())
	}

	// import
	importView, importDone := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             importView,
		},
	}
	args := []string{
		"-state", statePath,
		"module.child.test_instance.foo",
		"bar",
	}
	code = c.Run(args)
	importOutput := importDone(t)
	if code != 0 {
		t.Fatalf("import failed; expected success: %s", importOutput.All())
	}
}

func TestImport_nonManagedResource(t *testing.T) {
	t.Chdir(testFixturePath("import-missing-resource-config"))

	statePath := testTempFile(t)

	p := testProvider()

	cases := []struct {
		resAddr        string
		expectedErrMsg string
	}{
		{
			resAddr:        "data.test_data_source.foo",
			expectedErrMsg: "A managed resource address is required. Importing into a data resource is not allowed.",
		},
		{
			resAddr:        "ephemeral.test_data_source.foo",
			expectedErrMsg: "A managed resource address is required. Importing into an ephemeral resource is not allowed.",
		},
	}
	for _, tt := range cases {
		t.Run(tt.resAddr, func(t *testing.T) {
			view, done := testView(t)
			c := &ImportCommand{
				Meta: Meta{
					WorkingDir:       workdir.NewDir("."),
					testingOverrides: metaOverridesForProvider(p),
					View:             view,
				},
			}

			args := []string{
				"-state", statePath,
				tt.resAddr,
				"bar",
			}
			code := c.Run(args)
			output := done(t)
			if code != 1 {
				t.Fatalf("import succeeded; expected failure: %s", output.All())
			}

			msg := output.Stderr()
			if want := tt.expectedErrMsg; !strings.Contains(msg, want) {
				t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
			}
		})
	}
}

func TestImport_invalidResourceAddr(t *testing.T) {
	t.Chdir(testFixturePath("import-missing-resource-config"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-no-color",
		"-state", statePath,
		"bananas",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := output.Stderr()
	if want := `Error: Invalid address`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_targetIsModule(t *testing.T) {
	t.Chdir(testFixturePath("import-missing-resource-config"))

	statePath := testTempFile(t)

	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-no-color",
		"-state", statePath,
		"module.foo",
		"bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("import succeeded; expected failure")
	}

	msg := output.Stderr()
	if want := `Error: Invalid address`; !strings.Contains(msg, want) {
		t.Errorf("incorrect message\nwant substring: %s\ngot:\n%s", want, msg)
	}
}

func TestImport_ForEachKeyInTargetResourceAddr(t *testing.T) {
	t.Chdir(testFixturePath("import-non-existent-key"))

	statePath := testTempFile(t)
	p := testProvider()
	view, done := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"id": {Type: cty.String, Optional: true, Computed: true},
					},
				},
			},
		},
	}
	// Existing key should success
	args := []string{
		"-state", statePath,
		"test_instance.this[\"a\"]",
		"aa",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("import failed; expected success for existing resource: %s", output.Stderr())
	}
	// Non-existent key should fail
	args = []string{
		"-state", statePath,
		"test_instance.this[\"f\"]",
		"ff",
	}
	view, done = testView(t)
	c.Meta.View = view
	code = c.Run(args)
	output = done(t)
	if code == 0 {
		t.Fatalf("import succeeded; expected failure when the resource instance doesn't exist with the given key: %s", output.All())
	}

}

func TestImport_ForEachKeyInModuleAddr(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("import-keyed-module"), td)
	t.Chdir(td)

	statePath := testTempFile(t)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	providerSource, closeCallback := newMockProviderSource(t, map[string][]string{
		"test": {"1.2.3"},
	})
	defer closeCallback()

	// init to install the module
	initView, initDone := testView(t)
	m := Meta{
		WorkingDir:       workdir.NewDir("."),
		testingOverrides: metaOverridesForProvider(testProvider()),
		View:             initView,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}
	code := ic.Run([]string{})
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("init failed\n%s", initOutput.Stderr())
	}

	// Import
	importView, importDone := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             importView,
		},
	}
	// Importing into an existing module should succeed
	args := []string{
		"-state", statePath,
		"module.child[\"a\"].test_instance.this",
		"aa",
	}
	code = c.Run(args)
	importOutput := importDone(t)
	if code != 0 {
		t.Fatalf("import failed for a valid scenario\n%s", importOutput.Stderr())
	}

	// Importing into a non-existent module should fail
	args = []string{
		"-state", statePath,
		"module.child[\"f\"].test_instance.this",
		"ff",
	}
	importView, importDone = testView(t)
	c.Meta.View = importView
	code = c.Run(args)
	importOutput = importDone(t)
	if code == 0 {
		t.Fatalf("import succeeded; expected failure for non-existant module\n%s", importOutput.Stdout())
	}
}

func TestImport_ForEachKeyInModuleAndResourceAddr(t *testing.T) {
	td := t.TempDir()
	// We have the "child" module with keys "a" and "b"
	// Resource "test_instance.this" under the child module with keys "a" and "b"
	testCopyDir(t, testFixturePath("import-keyed-module-keyed-resource"), td)
	t.Chdir(td)

	statePath := testTempFile(t)

	p := testProvider()
	p.GetProviderSchemaResponse = &providers.GetProviderSchemaResponse{
		ResourceTypes: map[string]providers.Schema{
			"test_instance": {
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"foo": {Type: cty.String, Optional: true},
					},
				},
			},
		},
	}

	providerSource, closeCallback := newMockProviderSource(t, map[string][]string{
		"test": {"1.2.3"},
	})
	defer closeCallback()

	// init to install the module
	initView, initDone := testView(t)
	m := Meta{
		WorkingDir:       workdir.NewDir("."),
		testingOverrides: metaOverridesForProvider(testProvider()),
		View:             initView,
		ProviderSource:   providerSource,
	}

	ic := &InitCommand{
		Meta: m,
	}
	code := ic.Run([]string{})
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("init failed\n%s", initOutput.Stderr())
	}

	// Import
	importView, importDone := testView(t)
	c := &ImportCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             importView,
		},
	}

	// Valid Module + Valid Resource success
	args := []string{
		"-state", statePath,
		"module.child[\"a\"].test_instance.this[\"a\"]",
		"aa",
	}
	code = c.Run(args)
	importOutput := importDone(t)
	if code != 0 {
		t.Fatalf("import failed for a valid scenario\n%s", importOutput.Stderr())
	}

	// All following combinations should fail
	// Valid Module + Invalid Resource
	args = []string{
		"-state", statePath,
		"module.child[\"a\"].test_instance.this[\"f\"]",
		"af",
	}
	importView, importDone = testView(t)
	c.Meta.View = importView
	code = c.Run(args)
	importOutput = importDone(t)
	if code == 0 {
		t.Fatalf("import succeeded; expected failure for non-existent resource instance\n%s", importOutput.Stdout())
	}

	// Invalid Module + Valid Resource
	args = []string{
		"-state", statePath,
		"module.child[\"f\"].test_instance.this[\"a\"]",
		"fa",
	}
	importView, importDone = testView(t)
	c.Meta.View = importView
	code = c.Run(args)
	importOutput = importDone(t)
	if code == 0 {
		t.Fatalf("import succeeded; expected failure for non-existent module\n%s", importOutput.Stdout())
	}

	// Invalid Module + Invalid Resource
	args = []string{
		"-state", statePath,
		"module.child[\"f\"].test_instance.this[\"f\"]",
		"ff",
	}
	importView, importDone = testView(t)
	c.Meta.View = importView
	code = c.Run(args)
	importOutput = importDone(t)
	if code == 0 {
		t.Fatalf("import succeeded; expected failure for non-existent module and resource instance\n%s", importOutput.Stdout())
	}
}

const testImportStr = `
test_instance.foo:
  ID = yay
  provider = provider["registry.opentofu.org/hashicorp/test"]
`
