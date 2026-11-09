// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestValidateDesiredResourceInstanceMeta(t *testing.T) {
	resourceAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test",
		Name: "example",
	}

	tests := map[string]struct {
		Addr      addrs.AbsResourceInstance
		Metas     []eval.ModuleConfiguredResourceInstanceObjectMeta
		WantDiags tfdiags.Diagnostics
	}{
		"no metadata at all": {
			Addr:  resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Metas: nil,
			// The evaluator's ResourceInstanceMeta API promises that a desired
			// resource instance always has at at least one meta for its current
			// object declaring that it's desired, so this would represent a bug
			// in its implementation.
			WantDiags: tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Inconsistent resource instance status",
				"Resource instance test.example was reported as desired but it has no desired instance metadata available. This is a bug in OpenTofu.",
			)),
		},
		"desired in root module": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
						PrimaryDeclRange: &tfdiags.SourceRange{
							Filename: "test.tofu",
						},
					},
				},
			},
			WantDiags: nil,
		},
		"desired in child module": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
						PrimaryDeclRange: &tfdiags.SourceRange{
							Filename: "test.tofu",
						},
					},
				},
			},
			WantDiags: nil,
		},
		"desired in child module with root meta": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectWantUnspecified,
					},
				},
				{
					ModuleInstance: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
						PrimaryDeclRange: &tfdiags.SourceRange{
							Filename: "test.tofu",
						},
					},
				},
			},
			WantDiags: nil,
		},
		"desired in child module with invalid desired in root": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
					},
				},
				{
					ModuleInstance: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
						PrimaryDeclRange: &tfdiags.SourceRange{
							Filename: "test.tofu",
						},
					},
				},
			},
			// The evaluator is expected to report a resource instance as
			// desired only in the module instance it belongs to.
			WantDiags: tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Inconsistent resource instance status",
				"Resource instance module.child.test.example is declared as desired in the root module, which is not the module instance it belongs to. This is a bug in OpenTofu.",
			)),
		},
		"desired in child module with invalid unwanted in root": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectUnwanted,
						PrimaryDeclRange: &tfdiags.SourceRange{
							Filename: "removed.tofu",
						},
					},
				},
				{
					ModuleInstance: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
						PrimaryDeclRange: &tfdiags.SourceRange{
							Filename: "test.tofu",
						},
					},
				},
			},
			// This one represents an authoring error: all of the modules in the
			// configuration must be consistent about whether a resource
			// instance is desired or not.
			WantDiags: tfdiags.New(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Removed resource instance still exists",
				Detail:   "The root module declares that module.child.test.example was removed, but it is still declared at test.tofu:0,0.",
				Subject: &hcl.Range{
					Filename: "removed.tofu",
				},
			}),
		},
		"desired in child module with invalid unwanted in root without DeclRanges": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectUnwanted,
					},
				},
				{
					ModuleInstance: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
					},
				},
			},
			// This is still an authoring error, but testing that we're robust
			// against these being reported without declaration location
			// information can still at least report which module instances are
			// involved in the problem.
			WantDiags: tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Removed resource instance still exists",
				"The root module declares that module.child.test.example was removed, but it is still declared in module.child.",
			)),
		},
		"no desired but importing": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want:            eval.ResourceInstanceObjectWantUnspecified,
						ImportStatement: &eval.ImportStatement{
							// Only the non-nilness of this currently matters
							// for the sake of the validation rules, though
							// it's okay to add more here if the validation
							// rules start relying on this more in future.
						},
					},
				},
			},
			WantDiags: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gotDiags := validateDesiredResourceInstanceMeta(test.Addr, test.Metas)
			// We use the "ForRPC" representation of diagnostics just to get
			// something that's easier to compare without getting caught up
			// on unimportant implementation details.
			gotDiags = gotDiags.ForRPC()
			wantDiags := test.WantDiags.ForRPC()
			if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
				t.Error("wrong diagnostics\n" + diff)
			}
		})
	}
}

func TestValidateUnwantedResourceInstanceObjectMeta(t *testing.T) {
	resourceAddr := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "test",
		Name: "example",
	}

	tests := map[string]struct {
		Addr        addrs.AbsResourceInstanceObject
		Metas       []eval.ModuleConfiguredResourceInstanceObjectMeta
		ForceDelete bool
		WantDiags   tfdiags.Diagnostics
	}{
		"no metadata at all": {
			Addr:  resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance).CurrentObject(),
			Metas: nil,
			// It's valid and common to have no configured metadata for an
			// unwanted object.
			WantDiags: nil,
		},
		"no opinion in the root module": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance).CurrentObject(),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectWantUnspecified,
					},
				},
			},
			WantDiags: nil,
		},
		"unwanted in the root module": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance).CurrentObject(),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectUnwanted,
					},
				},
			},
			WantDiags: nil,
		},
		"desired in root module": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance).CurrentObject(),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
					},
				},
			},
			WantDiags: tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Inconsistent resource instance status",
				"Resource instance test.example is declared as desired in the root module, but yet it wasn't reported as desired during planning. This is a bug in OpenTofu.",
			)),
		},
		"desired in child module": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)).CurrentObject(),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
					},
				},
			},
			WantDiags: tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Inconsistent resource instance status",
				"Resource instance module.child.test.example is declared as desired in module.child, but yet it wasn't reported as desired during planning. This is a bug in OpenTofu.",
			)),
		},
		"desired in child module with root meta": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)).CurrentObject(),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectWantUnspecified,
					},
				},
				{
					ModuleInstance: addrs.RootModuleInstance.Child("child", addrs.NoKey),
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
					},
				},
			},
			WantDiags: tfdiags.New(tfdiags.Sourceless(
				tfdiags.Error,
				"Inconsistent resource instance status",
				"Resource instance module.child.test.example is declared as desired in module.child, but yet it wasn't reported as desired during planning. This is a bug in OpenTofu.",
			)),
		},
		"desired with forceDelete": {
			Addr: resourceAddr.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance).CurrentObject(),
			Metas: []eval.ModuleConfiguredResourceInstanceObjectMeta{
				{
					ModuleInstance: addrs.RootModuleInstance,
					ConfiguredResourceInstanceObjectMeta: &eval.ConfiguredResourceInstanceObjectMeta{
						Want: eval.ResourceInstanceObjectDesired,
					},
				},
			},
			ForceDelete: true,
			WantDiags:   nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			gotDiags := validateUnwantedResourceInstanceObjectMeta(test.Addr, test.Metas, test.ForceDelete)
			// We use the "ForRPC" representation of diagnostics just to get
			// something that's easier to compare without getting caught up
			// on unimportant implementation details.
			gotDiags = gotDiags.ForRPC()
			wantDiags := test.WantDiags.ForRPC()
			if diff := cmp.Diff(wantDiags, gotDiags); diff != "" {
				t.Error("wrong diagnostics\n" + diff)
			}
		})
	}
}
