// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/tfdiags"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/opentofu/svchost"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
)

func TestConfigProviderTypes(t *testing.T) {
	// nil cfg should return an empty map
	got := NewEmptyConfig().ProviderTypes()
	if len(got) != 0 {
		t.Fatal("expected empty result from empty config")
	}

	cfg, diags := testModuleConfigFromFile(t.Context(), "testdata/valid-files/providers-explicit-implied.tf")
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	got = cfg.ProviderTypes()
	want := []addrs.Provider{
		addrs.NewDefaultProvider("aws"),
		addrs.NewDefaultProvider("local"),
		addrs.NewDefaultProvider("null"),
		addrs.NewDefaultProvider("template"),
		addrs.NewDefaultProvider("test"),
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result:\n" + diff)
	}
}

func TestConfigProviderTypes_nested(t *testing.T) {
	// basic test with a nil config
	c := NewEmptyConfig()
	got := c.ProviderTypes()
	if len(got) != 0 {
		t.Fatalf("wrong result!\ngot: %#v\nwant: nil\n", got)
	}

	// config with two provider sources, and one implicit (default) provider
	cfg, diags := testNestedModuleConfigFromDir(t, "testdata/valid-modules/nested-providers-fqns")
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	got = cfg.ProviderTypes()
	want := []addrs.Provider{
		addrs.NewProvider(addrs.DefaultProviderRegistryHost, "bar", "test"),
		addrs.NewProvider(addrs.DefaultProviderRegistryHost, "foo", "test"),
		addrs.NewDefaultProvider("test"),
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result:\n" + diff)
	}
}

func TestConfigResolveAbsProviderAddr(t *testing.T) {
	cfg, diags := testModuleConfigFromDir(t.Context(), "testdata/providers-explicit-fqn")
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}

	t.Run("already absolute", func(t *testing.T) {
		addr := addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("test"),
			Alias:    "boop",
		}
		got := cfg.ResolveAbsProviderAddr(addr, addrs.RootModule)
		if got, want := got.String(), addr.String(); got != want {
			t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("local, implied mapping", func(t *testing.T) {
		addr := addrs.LocalProviderConfig{
			LocalName: "implied",
			Alias:     "boop",
		}
		got := cfg.ResolveAbsProviderAddr(addr, addrs.RootModule)
		want := addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewDefaultProvider("implied"),
			Alias:    "boop",
		}
		if got, want := got.String(), want.String(); got != want {
			t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
		}
	})
	t.Run("local, explicit mapping", func(t *testing.T) {
		addr := addrs.LocalProviderConfig{
			LocalName: "foo-test", // this is explicitly set in the config
			Alias:     "boop",
		}
		got := cfg.ResolveAbsProviderAddr(addr, addrs.RootModule)
		want := addrs.AbsProviderConfig{
			Module:   addrs.RootModule,
			Provider: addrs.NewProvider(addrs.DefaultProviderRegistryHost, "foo", "test"),
			Alias:    "boop",
		}
		if got, want := got.String(), want.String(); got != want {
			t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
		}
	})
}

func TestConfigProviderRequirements(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDir(t, "testdata/provider-reqs")
	// TODO: Version Constraint Deprecation.
	// Once we've removed the version argument from provider configuration
	// blocks, this can go back to expected 0 diagnostics.
	// assertNoDiagnostics(t, diags)
	assertDiagnosticCount(t, diags, 1)
	assertDiagnosticSummary(t, diags, "Version constraints inside provider configuration blocks are deprecated")

	tlsProvider := addrs.NewProvider(
		addrs.DefaultProviderRegistryHost,
		"hashicorp", "tls",
	)
	happycloudProvider := addrs.NewProvider(
		svchost.Hostname("tf.example.com"),
		"awesomecorp", "happycloud",
	)
	nullProvider := addrs.NewDefaultProvider("null")
	randomProvider := addrs.NewDefaultProvider("random")
	impliedProvider := addrs.NewDefaultProvider("implied")
	importimpliedProvider := addrs.NewDefaultProvider("importimplied")
	importexplicitProvider := addrs.NewDefaultProvider("importexplicit")
	terraformProvider := addrs.NewBuiltInProvider("terraform")
	configuredProvider := addrs.NewDefaultProvider("configured")
	grandchildProvider := addrs.NewDefaultProvider("grandchild")

	got, qualifs, diags := cfg.ProviderRequirements()
	assertNoDiagnostics(t, diags)
	want := getproviders.Requirements{
		// the nullProvider constraints from the two modules are merged
		nullProvider:           getproviders.MustParseVersionConstraints("~> 2.0.0, 2.0.1"),
		randomProvider:         getproviders.MustParseVersionConstraints("~> 1.2.0"),
		tlsProvider:            getproviders.MustParseVersionConstraints("~> 3.0"),
		configuredProvider:     getproviders.MustParseVersionConstraints("~> 1.4"),
		impliedProvider:        nil,
		importimpliedProvider:  nil,
		importexplicitProvider: nil,
		happycloudProvider:     nil,
		terraformProvider:      nil,
		grandchildProvider:     nil,
	}
	wantQualifs := &getproviders.ProvidersQualification{
		Implicit: map[addrs.Provider][]getproviders.ResourceRef{
			grandchildProvider: {
				{
					CfgRes: addrs.ConfigResource{Module: []string{"kinder", "nested"}, Resource: addrs.Resource{Mode: addrs.ManagedResourceMode, Type: "grandchild_foo", Name: "bar"}},
					Ref:    tfdiags.SourceRange{Filename: filepath.FromSlash("testdata/provider-reqs/child/grandchild/provider-reqs-grandchild.tf"), Start: tfdiags.SourcePos{Line: 3, Column: 1, Byte: 136}, End: tfdiags.SourcePos{Line: 3, Column: 32, Byte: 167}},
				},
			},
			impliedProvider: {
				{
					CfgRes: addrs.ConfigResource{Resource: addrs.Resource{Mode: addrs.ManagedResourceMode, Type: "implied_foo", Name: "bar"}},
					Ref:    tfdiags.SourceRange{Filename: filepath.FromSlash("testdata/provider-reqs/provider-reqs-root.tf"), Start: tfdiags.SourcePos{Line: 16, Column: 1, Byte: 317}, End: tfdiags.SourcePos{Line: 16, Column: 29, Byte: 345}},
				},
			},
			importexplicitProvider: {
				{
					CfgRes: addrs.ConfigResource{Resource: addrs.Resource{Mode: addrs.ManagedResourceMode, Type: "importimplied", Name: "targetB"}},
					Ref:    tfdiags.SourceRange{Filename: filepath.FromSlash("testdata/provider-reqs/provider-reqs-root.tf"), Start: tfdiags.SourcePos{Line: 42, Column: 1, Byte: 939}, End: tfdiags.SourcePos{Line: 42, Column: 7, Byte: 945}},
				},
			},
			importimpliedProvider: {
				{
					CfgRes: addrs.ConfigResource{Resource: addrs.Resource{Mode: addrs.ManagedResourceMode, Type: "importimplied", Name: "targetA"}},
					Ref:    tfdiags.SourceRange{Filename: filepath.FromSlash("testdata/provider-reqs/provider-reqs-root.tf"), Start: tfdiags.SourcePos{Line: 37, Column: 1, Byte: 886}, End: tfdiags.SourcePos{Line: 37, Column: 7, Byte: 892}},
				},
			},
			terraformProvider: {
				{
					CfgRes: addrs.ConfigResource{Resource: addrs.Resource{Mode: addrs.DataResourceMode, Type: "terraform_remote_state", Name: "bar"}},
					Ref:    tfdiags.SourceRange{Filename: filepath.FromSlash("testdata/provider-reqs/provider-reqs-root.tf"), Start: tfdiags.SourcePos{Line: 27, Column: 1, Byte: 628}, End: tfdiags.SourcePos{Line: 27, Column: 36, Byte: 663}},
				},
			},
		},
		Explicit: map[addrs.Provider]struct{}{
			happycloudProvider: {},
			nullProvider:       {},
			randomProvider:     {},
			tlsProvider:        {},
		},
	}
	// These 2 assertions are strictly to ensure that later the "provider" blocks are not registered into the qualifications.
	// Technically speaking, provider blocks are indeed implicit references, but the current warning message
	// on implicitly referenced providers could be misleading for the "provider" blocks.
	if _, okExpl := qualifs.Explicit[configuredProvider]; okExpl {
		t.Errorf("provider blocks shouldn't be added into the explicit qualifications")
	}
	if _, okImpl := qualifs.Implicit[configuredProvider]; okImpl {
		t.Errorf("provider blocks shouldn't be added into the implicit qualifications")
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong reqs result\n%s", diff)
	}

	if diff := cmp.Diff(wantQualifs, qualifs); diff != "" {
		t.Errorf("wrong qualifs result\n%s", diff)
	}
}

func TestConfigProviderRequirementsInclTests(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDirWithTests(t, "testdata/provider-reqs-with-tests")
	// TODO: Version Constraint Deprecation.
	// Once we've removed the version argument from provider configuration
	// blocks, this can go back to expected 0 diagnostics.
	// assertNoDiagnostics(t, diags)
	assertDiagnosticCount(t, diags, 1)
	assertDiagnosticSummary(t, diags, "Version constraints inside provider configuration blocks are deprecated")

	tlsProvider := addrs.NewProvider(
		addrs.DefaultProviderRegistryHost,
		"hashicorp", "tls",
	)
	nullProvider := addrs.NewDefaultProvider("null")
	randomProvider := addrs.NewDefaultProvider("random")
	impliedProvider := addrs.NewDefaultProvider("implied")
	terraformProvider := addrs.NewBuiltInProvider("terraform")
	configuredProvider := addrs.NewDefaultProvider("configured")

	got, qualifs, diags := cfg.ProviderRequirements()
	assertNoDiagnostics(t, diags)
	want := getproviders.Requirements{
		// the nullProvider constraints from the two modules are merged
		nullProvider:       getproviders.MustParseVersionConstraints("~> 2.0.0"),
		randomProvider:     getproviders.MustParseVersionConstraints("~> 1.2.0"),
		tlsProvider:        getproviders.MustParseVersionConstraints("~> 3.0"),
		configuredProvider: getproviders.MustParseVersionConstraints("~> 1.4"),
		impliedProvider:    nil,
		terraformProvider:  nil,
	}

	wantQualifs := &getproviders.ProvidersQualification{
		Implicit: map[addrs.Provider][]getproviders.ResourceRef{
			impliedProvider: {
				{
					CfgRes: addrs.ConfigResource{Resource: addrs.Resource{Mode: addrs.ManagedResourceMode, Type: "implied_foo", Name: "bar"}},
					Ref:    tfdiags.SourceRange{Filename: filepath.FromSlash("testdata/provider-reqs-with-tests/provider-reqs-root.tf"), Start: tfdiags.SourcePos{Line: 12, Column: 1, Byte: 247}, End: tfdiags.SourcePos{Line: 12, Column: 29, Byte: 275}},
				},
			},
			terraformProvider: {
				{
					CfgRes: addrs.ConfigResource{Resource: addrs.Resource{Mode: addrs.DataResourceMode, Type: "terraform_remote_state", Name: "bar"}},
					Ref:    tfdiags.SourceRange{Filename: filepath.FromSlash("testdata/provider-reqs-with-tests/provider-reqs-root.tf"), Start: tfdiags.SourcePos{Line: 19, Column: 1, Byte: 516}, End: tfdiags.SourcePos{Line: 19, Column: 36, Byte: 551}},
				},
			},
		},
		Explicit: map[addrs.Provider]struct{}{
			nullProvider:   {},
			randomProvider: {},
			tlsProvider:    {},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result\n%s", diff)
	}

	if diff := cmp.Diff(wantQualifs, qualifs); diff != "" {
		t.Errorf("wrong qualifs result\n%s", diff)
	}
}

func TestConfigProviderRequirementsDuplicate(t *testing.T) {
	_, diags := testNestedModuleConfigFromDir(t, "testdata/duplicate-local-name")
	assertDiagnosticCount(t, diags, 3)
	assertDiagnosticSummary(t, diags, "Duplicate required provider")
}

func TestConfigProviderForEach(t *testing.T) {
	_, diags := testNestedModuleConfigFromDir(t, "testdata/provider_for_each")
	assertDiagnosticCount(t, diags, 4)

	want := hcl.Diagnostics{
		{
			Summary: "Provider configuration for_each matches module",
			Detail:  "This provider configuration uses the same for_each expression as a module, which means that subsequent removal of elements from this collection would cause a planning error.",
		}, {
			Summary: "Provider configuration for_each matches resource",
			Detail:  "This provider configuration uses the same for_each expression as a resource, which means that subsequent removal of elements from this collection would cause a planning error.",
		}, {
			Summary: "Invalid module provider configuration",
			Detail:  `This module doesn't declare a provider "dumme" block with alias = "key", which is required for use with for_each`,
		}, {
			Summary: "Invalid resource provider configuration",
			Detail:  `This module doesn't declare a provider "dumme" block with alias = "key", which is required for use with for_each`,
		},
	}

	for _, wd := range want {
		found := false
		for _, gd := range diags {
			if gd.Summary == wd.Summary && strings.HasPrefix(gd.Detail, wd.Detail) {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected Diagnostic %s", wd)
		}
	}
}

func TestConfigProviderFromJSON(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDir(t, "testdata/provider_from_json")
	assertNoDiagnostics(t, diags)

	got, diags := cfg.ProviderRequirementsShallow()
	assertNoDiagnostics(t, diags)

	nullProvider := addrs.NewDefaultProvider("null")
	want := getproviders.Requirements{
		nullProvider: nil,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result\n%s", diff)
	}
}

func TestConfigProviderRequirementsShallow(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDir(t, "testdata/provider-reqs")
	// TODO: Version Constraint Deprecation.
	// Once we've removed the version argument from provider configuration
	// blocks, this can go back to expected 0 diagnostics.
	// assertNoDiagnostics(t, diags)
	assertDiagnosticCount(t, diags, 1)
	assertDiagnosticSummary(t, diags, "Version constraints inside provider configuration blocks are deprecated")

	tlsProvider := addrs.NewProvider(
		addrs.DefaultProviderRegistryHost,
		"hashicorp", "tls",
	)
	nullProvider := addrs.NewDefaultProvider("null")
	randomProvider := addrs.NewDefaultProvider("random")
	impliedProvider := addrs.NewDefaultProvider("implied")
	importimpliedProvider := addrs.NewDefaultProvider("importimplied")
	importexplicitProvider := addrs.NewDefaultProvider("importexplicit")
	terraformProvider := addrs.NewBuiltInProvider("terraform")
	configuredProvider := addrs.NewDefaultProvider("configured")

	got, diags := cfg.ProviderRequirementsShallow()
	assertNoDiagnostics(t, diags)
	want := getproviders.Requirements{
		// the nullProvider constraint is only from the root module
		nullProvider:           getproviders.MustParseVersionConstraints("~> 2.0.0"),
		randomProvider:         getproviders.MustParseVersionConstraints("~> 1.2.0"),
		tlsProvider:            getproviders.MustParseVersionConstraints("~> 3.0"),
		configuredProvider:     getproviders.MustParseVersionConstraints("~> 1.4"),
		impliedProvider:        nil,
		importimpliedProvider:  nil,
		importexplicitProvider: nil,
		terraformProvider:      nil,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result\n%s", diff)
	}
}

func TestConfigProviderRequirementsShallowInclTests(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDirWithTests(t, "testdata/provider-reqs-with-tests")
	// TODO: Version Constraint Deprecation.
	// Once we've removed the version argument from provider configuration
	// blocks, this can go back to expected 0 diagnostics.
	// assertNoDiagnostics(t, diags)
	assertDiagnosticCount(t, diags, 1)
	assertDiagnosticSummary(t, diags, "Version constraints inside provider configuration blocks are deprecated")

	tlsProvider := addrs.NewProvider(
		addrs.DefaultProviderRegistryHost,
		"hashicorp", "tls",
	)
	impliedProvider := addrs.NewDefaultProvider("implied")
	terraformProvider := addrs.NewBuiltInProvider("terraform")
	configuredProvider := addrs.NewDefaultProvider("configured")

	got, diags := cfg.ProviderRequirementsShallow()
	assertNoDiagnostics(t, diags)
	want := getproviders.Requirements{
		tlsProvider:        getproviders.MustParseVersionConstraints("~> 3.0"),
		configuredProvider: getproviders.MustParseVersionConstraints("~> 1.4"),
		impliedProvider:    nil,
		terraformProvider:  nil,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result\n%s", diff)
	}
}

func TestConfigProviderRequirementsByModule(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDir(t, "testdata/provider-reqs")
	// TODO: Version Constraint Deprecation.
	// Once we've removed the version argument from provider configuration
	// blocks, this can go back to expected 0 diagnostics.
	// assertNoDiagnostics(t, diags)
	assertDiagnosticCount(t, diags, 1)
	assertDiagnosticSummary(t, diags, "Version constraints inside provider configuration blocks are deprecated")

	tlsProvider := addrs.NewProvider(
		addrs.DefaultProviderRegistryHost,
		"hashicorp", "tls",
	)
	happycloudProvider := addrs.NewProvider(
		svchost.Hostname("tf.example.com"),
		"awesomecorp", "happycloud",
	)
	nullProvider := addrs.NewDefaultProvider("null")
	randomProvider := addrs.NewDefaultProvider("random")
	impliedProvider := addrs.NewDefaultProvider("implied")
	importimpliedProvider := addrs.NewDefaultProvider("importimplied")
	importexplicitProvider := addrs.NewDefaultProvider("importexplicit")
	terraformProvider := addrs.NewBuiltInProvider("terraform")
	configuredProvider := addrs.NewDefaultProvider("configured")
	grandchildProvider := addrs.NewDefaultProvider("grandchild")

	got, diags := cfg.ProviderRequirementsByModule()
	assertNoDiagnostics(t, diags)
	want := &ModuleRequirements{
		Name:       "",
		SourceAddr: nil,
		SourceDir:  "testdata/provider-reqs",
		Requirements: getproviders.Requirements{
			// Only the root module's version is present here
			nullProvider:           getproviders.MustParseVersionConstraints("~> 2.0.0"),
			randomProvider:         getproviders.MustParseVersionConstraints("~> 1.2.0"),
			tlsProvider:            getproviders.MustParseVersionConstraints("~> 3.0"),
			configuredProvider:     getproviders.MustParseVersionConstraints("~> 1.4"),
			impliedProvider:        nil,
			importimpliedProvider:  nil,
			importexplicitProvider: nil,
			terraformProvider:      nil,
		},
		Children: map[string]*ModuleRequirements{
			"kinder": {
				Name:       "kinder",
				SourceAddr: addrs.ModuleSourceLocal("./child"),
				SourceDir:  filepath.FromSlash("testdata/provider-reqs/child"),
				Requirements: getproviders.Requirements{
					nullProvider:       getproviders.MustParseVersionConstraints("= 2.0.1"),
					happycloudProvider: nil,
				},
				Children: map[string]*ModuleRequirements{
					"nested": {
						Name:       "nested",
						SourceAddr: addrs.ModuleSourceLocal("./grandchild"),
						SourceDir:  filepath.FromSlash("testdata/provider-reqs/child/grandchild"),
						Requirements: getproviders.Requirements{
							grandchildProvider: nil,
						},
						Children: map[string]*ModuleRequirements{},
						Tests:    make(map[string]*TestFileModuleRequirements),
					},
				},
				Tests: make(map[string]*TestFileModuleRequirements),
			},
		},
		Tests: make(map[string]*TestFileModuleRequirements),
	}

	ignore := cmpopts.IgnoreUnexported(version.Constraint{}, cty.Value{}, hclsyntax.Body{})
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("wrong result\n%s", diff)
	}
}

func TestConfigProviderRequirementsByModuleInclTests(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDirWithTests(t, "testdata/provider-reqs-with-tests")
	// TODO: Version Constraint Deprecation.
	// Once we've removed the version argument from provider configuration
	// blocks, this can go back to expected 0 diagnostics.
	// assertNoDiagnostics(t, diags)
	assertDiagnosticCount(t, diags, 1)
	assertDiagnosticSummary(t, diags, "Version constraints inside provider configuration blocks are deprecated")

	tlsProvider := addrs.NewProvider(
		addrs.DefaultProviderRegistryHost,
		"hashicorp", "tls",
	)
	nullProvider := addrs.NewDefaultProvider("null")
	randomProvider := addrs.NewDefaultProvider("random")
	impliedProvider := addrs.NewDefaultProvider("implied")
	terraformProvider := addrs.NewBuiltInProvider("terraform")
	configuredProvider := addrs.NewDefaultProvider("configured")

	got, diags := cfg.ProviderRequirementsByModule()
	assertNoDiagnostics(t, diags)
	want := &ModuleRequirements{
		Name:       "",
		SourceAddr: nil,
		SourceDir:  "testdata/provider-reqs-with-tests",
		Requirements: getproviders.Requirements{
			// Only the root module's version is present here
			tlsProvider:       getproviders.MustParseVersionConstraints("~> 3.0"),
			impliedProvider:   nil,
			terraformProvider: nil,
		},
		Children: make(map[string]*ModuleRequirements),
		Tests: map[string]*TestFileModuleRequirements{
			"provider-reqs-root.tftest.hcl": {
				Requirements: getproviders.Requirements{
					configuredProvider: getproviders.MustParseVersionConstraints("~> 1.4"),
				},
				Runs: map[string]*ModuleRequirements{
					"setup": {
						Name:       "setup",
						SourceAddr: addrs.ModuleSourceLocal("./setup"),
						SourceDir:  filepath.FromSlash("testdata/provider-reqs-with-tests/setup"),
						Requirements: getproviders.Requirements{
							nullProvider:   getproviders.MustParseVersionConstraints("~> 2.0.0"),
							randomProvider: getproviders.MustParseVersionConstraints("~> 1.2.0"),
						},
						Children: make(map[string]*ModuleRequirements),
						Tests:    make(map[string]*TestFileModuleRequirements),
					},
				},
			},
		},
	}

	ignore := cmpopts.IgnoreUnexported(version.Constraint{}, cty.Value{}, hclsyntax.Body{})
	if diff := cmp.Diff(want, got, ignore); diff != "" {
		t.Errorf("wrong result\n%s", diff)
	}
}

func TestVerifyDependencySelections(t *testing.T) {
	cfg, diags := testNestedModuleConfigFromDir(t, "testdata/provider-reqs")
	// TODO: Version Constraint Deprecation.
	// Once we've removed the version argument from provider configuration
	// blocks, this can go back to expected 0 diagnostics.
	// assertNoDiagnostics(t, diags)
	assertDiagnosticCount(t, diags, 1)
	assertDiagnosticSummary(t, diags, "Version constraints inside provider configuration blocks are deprecated")

	tlsProvider := addrs.NewProvider(
		addrs.DefaultProviderRegistryHost,
		"hashicorp", "tls",
	)
	happycloudProvider := addrs.NewProvider(
		svchost.Hostname("tf.example.com"),
		"awesomecorp", "happycloud",
	)
	nullProvider := addrs.NewDefaultProvider("null")
	randomProvider := addrs.NewDefaultProvider("random")
	impliedProvider := addrs.NewDefaultProvider("implied")
	importimpliedProvider := addrs.NewDefaultProvider("importimplied")
	importexplicitProvider := addrs.NewDefaultProvider("importexplicit")
	configuredProvider := addrs.NewDefaultProvider("configured")
	grandchildProvider := addrs.NewDefaultProvider("grandchild")

	tests := map[string]struct {
		PrepareLocks func(*depsfile.Locks)
		WantErrs     []string
	}{
		"empty locks": {
			func(*depsfile.Locks) {
				// Intentionally blank
			},
			[]string{
				`provider registry.opentofu.org/hashicorp/configured: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/grandchild: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/implied: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/importexplicit: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/importimplied: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/null: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/random: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/tls: required by this configuration but no version is selected`,
				`provider tf.example.com/awesomecorp/happycloud: required by this configuration but no version is selected`,
			},
		},
		"suitable locks": {
			func(locks *depsfile.Locks) {
				locks.SetProvider(configuredProvider, getproviders.MustParseVersion("1.4.0"), nil, nil)
				locks.SetProvider(grandchildProvider, getproviders.MustParseVersion("0.1.0"), nil, nil)
				locks.SetProvider(impliedProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(importimpliedProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(importexplicitProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(nullProvider, getproviders.MustParseVersion("2.0.1"), nil, nil)
				locks.SetProvider(randomProvider, getproviders.MustParseVersion("1.2.2"), nil, nil)
				locks.SetProvider(tlsProvider, getproviders.MustParseVersion("3.0.1"), nil, nil)
				locks.SetProvider(happycloudProvider, getproviders.MustParseVersion("0.0.1"), nil, nil)
			},
			nil,
		},
		"null provider constraints changed": {
			func(locks *depsfile.Locks) {
				locks.SetProvider(configuredProvider, getproviders.MustParseVersion("1.4.0"), nil, nil)
				locks.SetProvider(grandchildProvider, getproviders.MustParseVersion("0.1.0"), nil, nil)
				locks.SetProvider(impliedProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(importimpliedProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(importexplicitProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(nullProvider, getproviders.MustParseVersion("3.0.0"), nil, nil)
				locks.SetProvider(randomProvider, getproviders.MustParseVersion("1.2.2"), nil, nil)
				locks.SetProvider(tlsProvider, getproviders.MustParseVersion("3.0.1"), nil, nil)
				locks.SetProvider(happycloudProvider, getproviders.MustParseVersion("0.0.1"), nil, nil)
			},
			[]string{
				`provider registry.opentofu.org/hashicorp/null: locked version selection 3.0.0 doesn't match the updated version constraints "~> 2.0.0, 2.0.1"`,
			},
		},
		"null provider lock changed": {
			func(locks *depsfile.Locks) {
				// In this case, we set the lock file version constraints to
				// match the configuration, and so our error message changes
				// to not assume the configuration changed anymore.
				locks.SetProvider(nullProvider, getproviders.MustParseVersion("3.0.0"), getproviders.MustParseVersionConstraints("~> 2.0.0, 2.0.1"), nil)

				locks.SetProvider(configuredProvider, getproviders.MustParseVersion("1.4.0"), nil, nil)
				locks.SetProvider(grandchildProvider, getproviders.MustParseVersion("0.1.0"), nil, nil)
				locks.SetProvider(impliedProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(importimpliedProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(importexplicitProvider, getproviders.MustParseVersion("0.2.0"), nil, nil)
				locks.SetProvider(randomProvider, getproviders.MustParseVersion("1.2.2"), nil, nil)
				locks.SetProvider(tlsProvider, getproviders.MustParseVersion("3.0.1"), nil, nil)
				locks.SetProvider(happycloudProvider, getproviders.MustParseVersion("0.0.1"), nil, nil)
			},
			[]string{
				`provider registry.opentofu.org/hashicorp/null: version constraints "~> 2.0.0, 2.0.1" don't match the locked version selection 3.0.0`,
			},
		},
		"overridden provider": {
			func(locks *depsfile.Locks) {
				locks.SetProviderOverridden(happycloudProvider)
			},
			[]string{
				// We still catch all of the other ones, because only happycloud was overridden
				`provider registry.opentofu.org/hashicorp/configured: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/grandchild: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/implied: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/importexplicit: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/importimplied: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/null: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/random: required by this configuration but no version is selected`,
				`provider registry.opentofu.org/hashicorp/tls: required by this configuration but no version is selected`,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			depLocks := depsfile.NewLocks()
			test.PrepareLocks(depLocks)
			gotErrs := cfg.VerifyDependencySelections(depLocks)

			var gotErrsStr []string
			if gotErrs != nil {
				gotErrsStr = make([]string, len(gotErrs))
				for i, err := range gotErrs {
					gotErrsStr[i] = err.Error()
				}
			}

			if diff := cmp.Diff(test.WantErrs, gotErrsStr); diff != "" {
				t.Errorf("wrong errors\n%s", diff)
			}
		})
	}
}

func TestConfigProviderForConfigAddr(t *testing.T) {
	cfg, diags := testModuleConfigFromDir(t.Context(), "testdata/valid-modules/providers-fqns")
	assertNoDiagnostics(t, diags)

	got := cfg.ProviderForConfigAddr(addrs.NewDefaultLocalProviderConfig("foo-test"))
	want := addrs.NewProvider(addrs.DefaultProviderRegistryHost, "foo", "test")
	if !got.Equals(want) {
		t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
	}

	// now check a provider that isn't in the configuration. It should return a DefaultProvider.
	got = cfg.ProviderForConfigAddr(addrs.NewDefaultLocalProviderConfig("bar-test"))
	want = addrs.NewDefaultProvider("bar-test")
	if !got.Equals(want) {
		t.Errorf("wrong result\ngot:  %s\nwant: %s", got, want)
	}
}

func TestConfigAddProviderRequirements(t *testing.T) {
	cfg, diags := testModuleConfigFromFile(t.Context(), "testdata/valid-files/providers-explicit-implied.tf")
	assertNoDiagnostics(t, diags)

	reqs := getproviders.Requirements{
		addrs.NewDefaultProvider("null"): nil,
	}
	qualifs := new(getproviders.ProvidersQualification)
	diags = cfg.addProviderRequirements(reqs, qualifs, true, false)
	assertNoDiagnostics(t, diags)
	if got, want := len(qualifs.Explicit), 1; got != want {
		t.Fatalf("expected to have %d explicit provider requirement but got %d", want, got)
	}
	if got, want := len(qualifs.Implicit), 4; got != want {
		t.Fatalf("expected to have %d explicit provider requirement but got %d", want, got)
	}

	checks := []struct {
		key  addrs.Provider
		want []addrs.Resource
	}{
		{
			// check registry.opentofu.org/hashicorp/aws
			key: addrs.NewProvider("registry.opentofu.org", "hashicorp", "aws"),
			want: []addrs.Resource{
				cfg.Path.Resource(addrs.ManagedResourceMode, "aws_instance", "foo").Resource,
				cfg.Path.Resource(addrs.DataResourceMode, "aws_s3_object", "baz").Resource,
				cfg.Path.Resource(addrs.EphemeralResourceMode, "aws_secret", "bar").Resource,
			},
		},
		{
			// check registry.opentofu.org/hashicorp/null
			key: addrs.NewProvider("registry.opentofu.org", "hashicorp", "null"),
			want: []addrs.Resource{
				cfg.Path.Resource(addrs.ManagedResourceMode, "null_resource", "foo").Resource,
			},
		},
		{
			// check registry.opentofu.org/hashicorp/local
			key: addrs.NewProvider("registry.opentofu.org", "hashicorp", "local"),
			want: []addrs.Resource{
				cfg.Path.Resource(addrs.ManagedResourceMode, "local_file", "foo").Resource,
			},
		},
		{
			// check registry.opentofu.org/hashicorp/template
			key: addrs.NewProvider("registry.opentofu.org", "hashicorp", "template"),
			want: []addrs.Resource{
				cfg.Path.Resource(addrs.ManagedResourceMode, "local_file", "bar").Resource,
			},
		},
	}
	for _, c := range checks {
		t.Run(c.key.String(), func(t *testing.T) {
			refs := qualifs.Implicit[c.key]
			if got, want := len(refs), len(c.want); got != want {
				t.Fatalf("expected to find %d implicit references for provider %q but got %d", want, c.key, got)
			}

			var refsAddrs []addrs.Resource
			for _, ref := range refs {
				refsAddrs = append(refsAddrs, ref.CfgRes.Resource)
			}
			sort.Slice(refsAddrs, func(i, j int) bool {
				return refsAddrs[i].Less(refsAddrs[j])
			})
			sort.Slice(c.want, func(i, j int) bool {
				return c.want[i].Less(c.want[j])
			})
			if diff := cmp.Diff(refsAddrs, c.want); diff != "" {
				t.Fatalf("expected to find specific resources to implicitly reference the provider %s. diff:\n%s", c.key, diff)
			}
		})
	}

	wantReqs := getproviders.Requirements{
		addrs.NewProvider("registry.opentofu.org", "hashicorp", "template"): nil,
		addrs.NewProvider("registry.opentofu.org", "hashicorp", "local"):    nil,
		addrs.NewProvider("registry.opentofu.org", "hashicorp", "null"):     nil,
		addrs.NewProvider("registry.opentofu.org", "hashicorp", "aws"):      nil,
		addrs.NewProvider("registry.opentofu.org", "hashicorp", "test"):     nil,
	}
	if diff := cmp.Diff(wantReqs, reqs); diff != "" {
		t.Fatalf("unexected returned providers qualifications: %s", diff)
	}
}

func TestConfigImportProviderClashesWithModules(t *testing.T) {
	src, err := os.ReadFile("testdata/invalid-import-files/import-and-module-clash.tf")
	if err != nil {
		t.Fatal(err)
	}

	parser := testParser(map[string]string{
		"main.tf": string(src),
	})

	_, diags := parser.LoadConfigFile("main.tf")
	assertExactDiagnostics(t, diags, []string{
		`main.tf:9,3-19: Invalid import provider argument; The provider argument can only be specified in import blocks that will generate configuration.

Use the providers argument within the module block to configure providers for all resources within a module, including imported resources.`,
	})
}

func TestConfigImportProviderClashesWithResources(t *testing.T) {
	cfg, diags := testModuleConfigFromFile(t.Context(), "testdata/invalid-import-files/import-and-resource-clash.tf")
	assertNoDiagnostics(t, diags)
	qualifs := new(getproviders.ProvidersQualification)

	diags = cfg.addProviderRequirements(getproviders.Requirements{}, qualifs, true, false)
	assertExactDiagnostics(t, diags, []string{
		`testdata/invalid-import-files/import-and-resource-clash.tf:9,3-19: Invalid import provider argument; The provider argument in the target resource block must match the import block.`,
	})
}

func TestConfigImportProviderWithNoResourceProvider(t *testing.T) {
	cfg, diags := testModuleConfigFromFile(t.Context(), "testdata/invalid-import-files/import-and-no-resource.tf")
	assertNoDiagnostics(t, diags)

	qualifs := new(getproviders.ProvidersQualification)
	diags = cfg.addProviderRequirements(getproviders.Requirements{}, qualifs, true, false)
	assertExactDiagnostics(t, diags, []string{
		`testdata/invalid-import-files/import-and-no-resource.tf:5,3-19: Invalid import provider argument; The provider argument in the target resource block must be specified and match the import block.`,
	})
}

func TestConfigWithDeprecatedVariables(t *testing.T) {
	src, err := os.ReadFile("testdata/variable-empty-deprecated/main.tf")
	if err != nil {
		t.Fatal(err)
	}

	parser := testParser(map[string]string{
		"main.tf": string(src),
	})

	_, diags := parser.LoadConfigFile("main.tf")
	// The lack of a diagnostic for the "without_deprecated" variable validates also that a variable without any "deprecated" field specified
	// is parsed correctly
	assertExactDiagnostics(t, diags, []string{
		"main.tf:1,10-33: Invalid `deprecated` value; The \"deprecated\" argument must not be empty, and should provide instructions on how to migrate away from usage of this deprecated variable.",
		"main.tf:7,10-39: Invalid `deprecated` value; The \"deprecated\" argument must not be empty, and should provide instructions on how to migrate away from usage of this deprecated variable.",
	})
}

func TestTransformForTest(t *testing.T) {

	str := func(providers map[string]string) string {
		var buffer bytes.Buffer
		for key, config := range providers {
			buffer.WriteString(fmt.Sprintf("%s: %s\n", key, config))
		}
		return buffer.String()
	}

	convertToProviders := func(t *testing.T, contents map[string]string) map[string]*Provider {
		t.Helper()

		providers := make(map[string]*Provider)
		for key, content := range contents {
			parser := hclparse.NewParser()
			file, diags := parser.ParseHCL([]byte(content), fmt.Sprintf("%s.hcl", key))
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}

			provider := &Provider{
				Config: file.Body,
			}

			parts := strings.Split(key, ".")
			provider.Name = parts[0]
			if len(parts) > 1 {
				provider.Alias = parts[1]
			}

			providers[key] = provider
		}
		return providers
	}

	validate := func(t *testing.T, msg string, expected map[string]string, actual map[string]*Provider) {
		t.Helper()

		converted := make(map[string]string)
		for key, provider := range actual {
			content, err := provider.Config.Content(&hcl.BodySchema{
				Attributes: []hcl.AttributeSchema{
					{Name: "source", Required: true},
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			source, diags := content.Attributes["source"].Expr.Value(nil)
			if diags.HasErrors() {
				t.Fatal(diags.Error())
			}
			converted[key] = fmt.Sprintf("source = %q", source.AsString())
		}

		if diff := cmp.Diff(expected, converted); len(diff) > 0 {
			t.Errorf("%s\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", msg, str(expected), str(converted), diff)
		}
	}

	tcs := map[string]struct {
		configProviders   map[string]string
		fileProviders     map[string]string
		runProviders      []PassedProviderConfig
		expectedProviders map[string]string
		expectedErrors    []string
	}{
		"empty": {
			configProviders:   make(map[string]string),
			expectedProviders: make(map[string]string),
		},
		"only providers in config": {
			configProviders: map[string]string{
				"foo": "source = \"config\"",
				"bar": "source = \"config\"",
			},
			expectedProviders: map[string]string{
				"foo": "source = \"config\"",
				"bar": "source = \"config\"",
			},
		},
		"only providers in test file": {
			configProviders: make(map[string]string),
			fileProviders: map[string]string{
				"foo": "source = \"testfile\"",
				"bar": "source = \"testfile\"",
			},
			expectedProviders: map[string]string{
				"foo": "source = \"testfile\"",
				"bar": "source = \"testfile\"",
			},
		},
		"only providers in run block": {
			configProviders: make(map[string]string),
			runProviders: []PassedProviderConfig{
				{
					InChild: &ProviderConfigRef{
						Name: "foo",
					},
					InParent: &ProviderConfigRef{
						Name: "bar",
					},
				},
			},
			expectedProviders: make(map[string]string),
			expectedErrors: []string{
				":0,0-0: Missing provider definition for bar; This provider block references a provider definition that does not exist.",
			},
		},
		"subset of providers in test file": {
			configProviders: make(map[string]string),
			fileProviders: map[string]string{
				"bar": "source = \"testfile\"",
			},
			runProviders: []PassedProviderConfig{
				{
					InChild: &ProviderConfigRef{
						Name: "foo",
					},
					InParent: &ProviderConfigRef{
						Name: "bar",
					},
				},
			},
			expectedProviders: map[string]string{
				"foo": "source = \"testfile\"",
			},
		},
		"overrides providers in config": {
			configProviders: map[string]string{
				"foo": "source = \"config\"",
				"bar": "source = \"config\"",
			},
			fileProviders: map[string]string{
				"bar": "source = \"testfile\"",
			},
			expectedProviders: map[string]string{
				"foo": "source = \"config\"",
				"bar": "source = \"testfile\"",
			},
		},
		"overrides subset of providers in config": {
			configProviders: map[string]string{
				"foo": "source = \"config\"",
				"bar": "source = \"config\"",
			},
			fileProviders: map[string]string{
				"foo": "source = \"testfile\"",
				"bar": "source = \"testfile\"",
			},
			runProviders: []PassedProviderConfig{
				{
					InChild: &ProviderConfigRef{
						Name: "bar",
					},
					InParent: &ProviderConfigRef{
						Name: "bar",
					},
				},
			},
			expectedProviders: map[string]string{
				"foo": "source = \"config\"",
				"bar": "source = \"testfile\"",
			},
		},
		"handles aliases": {
			configProviders: map[string]string{
				"foo.primary":   "source = \"config\"",
				"foo.secondary": "source = \"config\"",
			},
			fileProviders: map[string]string{
				"foo": "source = \"testfile\"",
			},
			runProviders: []PassedProviderConfig{
				{
					InChild: &ProviderConfigRef{
						Name: "foo.secondary",
					},
					InParent: &ProviderConfigRef{
						Name: "foo",
					},
				},
			},
			expectedProviders: map[string]string{
				"foo.primary":   "source = \"config\"",
				"foo.secondary": "source = \"testfile\"",
			},
		},
	}
	for name, tc := range tcs {
		t.Run(name, func(t *testing.T) {
			config := &Config{
				Module: &Module{
					ProviderConfigs: convertToProviders(t, tc.configProviders),
				},
			}

			file := &TestFile{
				Providers: convertToProviders(t, tc.fileProviders),
			}

			run := &TestRun{
				Providers: tc.runProviders,
			}

			evalCtx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"run": cty.ObjectVal(map[string]cty.Value{}),
					"var": cty.ObjectVal(map[string]cty.Value{}),
				},
			}
			reset, diags := config.TransformForTest(run, file, evalCtx)

			var actualErrs []string
			for _, err := range diags.Errs() {
				actualErrs = append(actualErrs, err.Error())
			}
			if diff := cmp.Diff(actualErrs, tc.expectedErrors, cmpopts.IgnoreUnexported()); len(diff) > 0 {
				t.Errorf("unmatched errors\nexpected:\n%s\nactual:\n%s\ndiff:\n%s", strings.Join(tc.expectedErrors, "\n"), strings.Join(actualErrs, "\n"), diff)
			}

			validate(t, "after transform mismatch", tc.expectedProviders, config.Module.ProviderConfigs)
			reset()
			validate(t, "after reset mismatch", tc.configProviders, config.Module.ProviderConfigs)

		})
	}
}

// This test is checking that by giving the outermost called module, the method called is
// returning correctly that is a remote module relatively to the root module.
// This is because root module is calling the child module from a remote source
// but all the other calls are done from local modules.
// Eg: Root module is calling a module from a git repo in a particular directory,
// but that module is calling other modules from the same repo by referencing those
// with a relative path.
func TestIsCallFromRemote(t *testing.T) {
	childName := "call-to-child"
	gchildName := "call-to-gchild"
	ggchildName := "call-to-ggchild"
	gggchildName := "call-to-gggchild"
	parseModuleSource := func(t *testing.T, source string) addrs.ModuleSource {
		s, err := addrs.ParseModuleSource(source)
		if err != nil {
			t.Fatalf("failed to parse module source %q: %s", source, err)
		}
		return s
	}
	tests := map[string]struct {
		childModulePath string
		expectedRes     bool
	}{
		"from git repo": {
			childModulePath: "git::https://github.com/user/repo//child",
			expectedRes:     true,
		},
		"from registry": {
			childModulePath: "registry.example.com/foo/bar/baz",
			expectedRes:     true,
		},
		"from local": {
			childModulePath: "../mod",
			expectedRes:     false,
		},
	}
	for ttn, tt := range tests {
		t.Run(ttn, func(t *testing.T) {
			root := &Config{
				Module: &Module{
					ModuleCalls: map[string]*ModuleCall{
						childName: {SourceAddr: parseModuleSource(t, tt.childModulePath)},
					},
				},
			}
			child := &Config{
				Parent: root,
				Path:   []string{childName},
				Module: &Module{
					ModuleCalls: map[string]*ModuleCall{
						gchildName: {SourceAddr: parseModuleSource(t, "../gchild-module")},
					},
				},
			}
			gchild := &Config{
				Parent: child,
				Path:   []string{gchildName},
				Module: &Module{
					ModuleCalls: map[string]*ModuleCall{
						ggchildName: {SourceAddr: parseModuleSource(t, "../ggchild-module")},
					},
				},
			}
			ggchild := &Config{
				Parent: gchild,
				Path:   []string{ggchildName},
				Module: &Module{
					ModuleCalls: map[string]*ModuleCall{
						gggchildName: {SourceAddr: parseModuleSource(t, "../gggchild-module")},
					},
				},
			}

			if want, got := tt.expectedRes, ggchild.IsModuleCallFromRemoteModule(ggchildName); want != got {
				t.Fatalf("expected IsModuleCallFromRemoteModule to return %t but got %t", want, got)
			}
		})
	}
}

func TestParseEphemeralBlocks(t *testing.T) {
	p := NewParser(nil)
	f, diags := p.LoadConfigFile("testdata/ephemeral-blocks/main.tf")
	// check diags
	{
		if len(diags) != 6 { // 4 lifecycle unallowed attributes, unallowed connection block and unallowed provisioner block
			t.Fatalf("expected 6 diagnostics but got only: %d", len(diags))
		}
		containsExpectedKeywords := func(diagContent string) bool {
			for _, k := range []string{"ignore_changes", "prevent_destroy", "create_before_destroy", "replace_triggered_by", "connection", "provisioner"} {
				if strings.Contains(diagContent, k) {
					return true
				}
			}
			return false
		}
		var connDiag *hcl.Diagnostic
		for _, diag := range diags {
			content := diag.Error()
			if !containsExpectedKeywords(content) {
				t.Fatalf("expected diagnostic to contain at least one of the keywords: %s", content)
			}
			if strings.Contains(content, "connection") {
				connDiag = diag
			}
		}
		// specific assertions for ensuring that the definition block from diags are configured properly
		if connDiag == nil {
			t.Fatalf("diagnostic for the 'connection' block not found")
		}
		expectedRange := hcl.Range{
			Filename: "testdata/ephemeral-blocks/main.tf",
			Start: hcl.Pos{
				Line:   18,
				Column: 3,
				Byte:   274,
			},
			End: hcl.Pos{
				Line:   18,
				Column: 13,
				Byte:   284,
			},
		}
		if !expectedRange.Overlaps(*connDiag.Subject) {
			t.Fatalf("unexpected connection block definition range.\nwant: %s\ngot: %s", expectedRange, *connDiag.Subject)
		}
	}
	{
		if len(f.EphemeralResources) != 2 {
			t.Fatalf("expected 2 ephemeral resources but got only: %d", len(f.EphemeralResources))
		}
		for _, er := range f.EphemeralResources {
			switch er.Name {
			case "foo":
				if er.ForEach == nil {
					t.Errorf("expected to have a for_each expression but got nothing")
				}
			case "bar":
				attrs, _ := er.Config.JustAttributes()
				if _, ok := attrs["attribute"]; !ok {
					t.Errorf("expected to have \"attribute\" but could not find it")
				}
				if _, ok := attrs["attribute2"]; !ok {
					t.Errorf("expected to have \"attribute\" but could not find it")
				}
				if er.Count == nil {
					t.Errorf("expected to have a count expression but got nothing")
				}
				if er.ProviderConfigRef == nil || er.ProviderConfigRef.Addr().String() != "provider.test.name" {
					t.Errorf("expected to have \"provider.test.name\" provider alias configured but instead it was: %+v", er.ProviderConfigRef)
				}
				if len(er.Preconditions) != 1 {
					t.Errorf("expected to have one precondition but got %d", len(er.Preconditions))
				}
				if len(er.Postconditions) != 1 {
					t.Errorf("expected to have one postcondition but got %d", len(er.Postconditions))
				}
				if len(er.DependsOn) != 1 {
					t.Errorf("expected to have a depends_on traversal but got %d", len(er.Postconditions))
				}
				if er.Managed != nil {
					t.Errorf("error in the parsing code. Ephemeral resources are not meant to have a managed object")
				}
			}
		}
	}

}
