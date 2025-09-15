// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/providers"
)

// NOTE: Unlike many of the _test.go files in this package, this one is in
// "package eval" itself rather than in "package eval_test", because it's
// testing the "prepareToPlan" implementation detail that isn't part of the
// public API.
//
// If you bring test code from other files into here then you'll probably
// need to remove some "eval." prefixes from references to avoid making this
// package import itself.

func TestPrepare_ephemeralResourceUsers(t *testing.T) {
	configInst, diags := NewConfigInstance(t.Context(), &ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &EvalContext{
			Modules: ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					terraform {
						required_providers {
							foo = {
								source = "test/foo"
							}
						}
					}
					ephemeral "foo" "a" {
						count = 2

						name = "a ${count.index}"
					}
					ephemeral "foo" "b" {
						count = 2

						name = ephemeral.foo.a[count.index].id
					}
					locals {
					    # This is intentionally a more complex expression
						# to analyze, to prove that we can still chase the
						# instance-specific references through it.
						# This produces a tuple of two-element tuples with the
						# corresponding ids of ephemeral.foo.a and
						# ephemeral.foo.b respectively.
						together = [
							for i, a in ephemeral.foo.a :
							[a.id, ephemeral.foo.b[i].id]
						]
					}
					resource "foo" "c" {
						count = 2

						# Even indirectly through this projection of values
						# from the two ephemeral resources we should correctly
						# detect that foo.c instances are correlated with
						# ephemeral.foo.a and ephemeral.foo.b instances of
						# the same index.
						something = local.together[count.index]

						# The above is really just an overly-complicated way of
						# writing this:
						#
						# something = [
						#   ephemeral.foo.a[count.index],
						#   ephemeral.foo.b[count.index],
						# ]
					}
					provider "foo" {
						alias = "other"

						name = ephemeral.foo.a[0].name
					}
				`),
			}),
			Providers: ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
				addrs.MustParseProviderSourceString("test/foo"): {
					Provider: providers.Schema{
						Block: &configschema.Block{
							Attributes: map[string]*configschema.Attribute{
								"name": {
									Type:     cty.String,
									Optional: true,
								},
							},
						},
					},
					EphemeralResources: map[string]providers.Schema{
						"foo": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"name": {
										Type:     cty.String,
										Required: true,
									},
									"id": {
										Type:     cty.String,
										Computed: true,
									},
								},
							},
						},
					},
					ResourceTypes: map[string]providers.Schema{
						"foo": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"something": {
										Type:      cty.List(cty.String),
										Optional:  true,
										WriteOnly: true,
									},
								},
							},
						},
					},
				},
			}),
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues:      InputValuesForTesting(map[string]cty.Value{}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	got, diags := configInst.prepareToPlan(t.Context())
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	fooA := addrs.Resource{
		Mode: addrs.EphemeralResourceMode,
		Type: "foo",
		Name: "a",
	}.Absolute(addrs.RootModuleInstance)
	fooB := addrs.Resource{
		Mode: addrs.EphemeralResourceMode,
		Type: "foo",
		Name: "b",
	}.Absolute(addrs.RootModuleInstance)
	fooC := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "c",
	}.Absolute(addrs.RootModuleInstance)
	inst0 := addrs.IntKey(0)
	inst1 := addrs.IntKey(1)
	providerInstAddr := addrs.AbsProviderConfigCorrect{
		Module: addrs.RootModuleInstance,
		Config: addrs.ProviderConfigCorrect{
			Provider: addrs.MustParseProviderSourceString("test/foo"),
		},
	}.Instance(addrs.NoKey)
	providerOtherInstAddr := addrs.AbsProviderConfigCorrect{
		Module: addrs.RootModuleInstance,
		Config: addrs.ProviderConfigCorrect{
			Provider: addrs.MustParseProviderSourceString("test/foo"),
			Alias:    "other",
		},
	}.Instance(addrs.NoKey)

	// The analysis should detect that:
	// - ephemeral.foo.a[0] is used by ephemeral.foo.b[0] and foo.c[0], and by the foo.other provider instance
	// - ephemeral.foo.a[1] is used by ephemeral.foo.b[1] and foo.c[1]
	// - ephemeral.foo.b[0] is used by only foo.c[0]
	// - ephemeral.foo.b[1] is used by only foo.c[1]
	// In particular, the evaluator should be able to notice that
	// only the correlated instance keys have any relationship between
	// them, and so e.g. ephemeral.foo.a[0] is NOT used by ephemeral.foo.b[1].
	//
	// This level of precision was not possible in the traditional
	// "package tofu" language runtime, because it calculated dependencies
	// based only on static analysis, but this new evaluator uses dynamic
	// analysis. Refer to [configgraph.ContributingResourceInstances]
	// to learn more about how that's meant to work, if you're trying to
	// debug a regression here that made the analysis less precise.
	want := &ResourceRelationships{
		// Note that this field captures _inverse_ dependencies: the values
		// are instances that depend on the keys.
		//
		// The best way to understand this is that the ephemeral resource
		// instance identified in an element's key must remain "open" until all
		// of the instances identified in the element's value have finished
		// planning.
		EphemeralResourceUsers: addrs.MakeMap(
			addrs.MakeMapElem(fooA.Instance(inst0), EphemeralResourceInstanceUsers{
				ResourceInstances: addrs.MakeSet(
					fooB.Instance(inst0),
					fooC.Instance(inst0),
				),
				ProviderInstances: addrs.MakeSet(
					providerOtherInstAddr,
				),
			}),
			addrs.MakeMapElem(fooA.Instance(inst1), EphemeralResourceInstanceUsers{
				ResourceInstances: addrs.MakeSet(
					fooB.Instance(inst1),
					fooC.Instance(inst1),
				),
				ProviderInstances: addrs.MakeSet[addrs.AbsProviderInstanceCorrect](),
			}),
			addrs.MakeMapElem(fooB.Instance(inst0), EphemeralResourceInstanceUsers{
				ResourceInstances: addrs.MakeSet(
					fooC.Instance(inst0),
				),
				ProviderInstances: addrs.MakeSet[addrs.AbsProviderInstanceCorrect](),
			}),
			addrs.MakeMapElem(fooB.Instance(inst1), EphemeralResourceInstanceUsers{
				ResourceInstances: addrs.MakeSet(
					fooC.Instance(inst1),
				),
				ProviderInstances: addrs.MakeSet[addrs.AbsProviderInstanceCorrect](),
			}),
		),

		// PrepareToPlan also finds the resources that belong to each
		// provider instance, which is not the focus of this test but
		// are part of the result nonetheless.
		ProviderInstanceUsers: addrs.MakeMap(
			addrs.MakeMapElem(providerInstAddr, ProviderInstanceUsers{
				ResourceInstances: addrs.MakeSet(
					fooA.Instance(inst0),
					fooA.Instance(inst1),
					fooB.Instance(inst0),
					fooB.Instance(inst1),
					fooC.Instance(inst0),
					fooC.Instance(inst1),
				),
			}),
		),
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result\n" + diff)
	}
}

func TestPrepare_crossModuleReferences(t *testing.T) {
	configInst, diags := NewConfigInstance(t.Context(), &ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &EvalContext{
			Modules: ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					module "a" {
						source = "./a"
					}
					module "b" {
						source = "./b"

						name = module.a.name
					}
				`),
				addrs.ModuleSourceLocal("./a"): configs.ModuleFromStringForTesting(t, `
					terraform {
						required_providers {
							foo = {
								source = "test/foo"
							}
						}
					}
					provider "foo" {}
					ephemeral "foo" "a" {
						name = "a"
					}
					output "name" {
						value = ephemeral.foo.a.name
					}
				`),
				addrs.ModuleSourceLocal("./b"): configs.ModuleFromStringForTesting(t, `
					terraform {
						required_providers {
							foo = {
								source = "test/foo"
							}
						}
					}
					provider "foo" {}
					variable "name" {
						type      = string
						ephemeral = true
					}
					resource "foo" "b" {
						name = var.name
					}
				`),
			}),
			Providers: ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
				addrs.MustParseProviderSourceString("test/foo"): {
					Provider: providers.Schema{
						Block: &configschema.Block{},
					},
					EphemeralResources: map[string]providers.Schema{
						"foo": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"name": {
										Type:     cty.String,
										Required: true,
									},
								},
							},
						},
					},
					ResourceTypes: map[string]providers.Schema{
						"foo": {
							Block: &configschema.Block{
								Attributes: map[string]*configschema.Attribute{
									"name": {
										Type:      cty.String,
										Optional:  true,
										WriteOnly: true,
									},
								},
							},
						},
					},
				},
			}),
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues:      InputValuesForTesting(map[string]cty.Value{}),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	got, diags := configInst.prepareToPlan(t.Context())
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	fooA := addrs.Resource{
		Mode: addrs.EphemeralResourceMode,
		Type: "foo",
		Name: "a",
	}.Absolute(addrs.RootModuleInstance.Child("a", addrs.NoKey))
	fooB := addrs.Resource{
		Mode: addrs.ManagedResourceMode,
		Type: "foo",
		Name: "b",
	}.Absolute(addrs.RootModuleInstance.Child("b", addrs.NoKey))
	providerInstAddr := addrs.AbsProviderConfigCorrect{
		Module: addrs.RootModuleInstance,
		Config: addrs.ProviderConfigCorrect{
			Provider: addrs.MustParseProviderSourceString("test/foo"),
		},
	}.Instance(addrs.NoKey)

	// The analyzer should detect that foo.b in module.b depends on
	// ephemeral.foo.a in module.a even though they are declared in
	// different modules.
	want := &ResourceRelationships{
		EphemeralResourceUsers: addrs.MakeMap(
			addrs.MakeMapElem(fooA.Instance(addrs.NoKey), EphemeralResourceInstanceUsers{
				ResourceInstances: addrs.MakeSet(
					fooB.Instance(addrs.NoKey),
				),
				ProviderInstances: addrs.MakeSet[addrs.AbsProviderInstanceCorrect](),
			}),
		),

		// PrepareToPlan also finds the resources that belong to each
		// provider instance, which is not the focus of this test but
		// are part of the result nonetheless.
		ProviderInstanceUsers: addrs.MakeMap(
			addrs.MakeMapElem(providerInstAddr, ProviderInstanceUsers{
				ResourceInstances: addrs.MakeSet(
					fooA.Instance(addrs.NoKey),
					fooB.Instance(addrs.NoKey),
				),
			}),
		),
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result\n" + diff)
	}
}
