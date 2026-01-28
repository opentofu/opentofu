// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval_test

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/providers"
)

func TestConfigInstanceWriteGraphvizGraphForDebugging(t *testing.T) {
	providers := eval.ProvidersForTesting(map[addrs.Provider]*providers.GetProviderSchemaResponse{
		addrs.MustParseProviderSourceString("test/foo"): {
			Provider: providers.Schema{
				Block: &configschema.Block{
					Attributes: map[string]*configschema.Attribute{
						"greeting": {
							Type:     cty.String,
							Required: true,
						},
					},
				},
			},
			ResourceTypes: map[string]providers.Schema{
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
		},
	})
	configInst, diags := eval.NewConfigInstance(t.Context(), &eval.ConfigCall{
		EvalContext: evalglue.EvalContextForTesting(t, &eval.EvalContext{
			Modules: eval.ModulesForTesting(map[addrs.ModuleSourceLocal]*configs.Module{
				addrs.ModuleSourceLocal("."): configs.ModuleFromStringForTesting(t, `
					terraform {
						required_providers {
							foo = {
								source = "test/foo"
							}
						}
					}
					provider "foo" {
						greeting = "Hello"
					}
					resource "foo" "bar" {
						name = "Thingy"
					}
					resource "foo" "baz" {
					    count = 2

						name = "${foo.bar.name} ${count.index}"
					}
					resource "foo" "beep" {
					    count = length(foo.baz)

						name = "${foo.baz[count.index].name} beep"
					}
					resource "foo" "deferred" {
					    # This just forces the count to be unknown.
						count = length(timestamp())

						name = "deferred ${count.index}"
					}
				`),
			}),
			Providers: providers,
		}),
		RootModuleSource: addrs.ModuleSourceLocal("."),
		InputValues:      eval.InputValuesForTesting(nil),
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	var buf strings.Builder
	diags = configInst.WriteGraphvizGraphForDebugging(t.Context(), &buf)
	if diags.HasErrors() {
		t.Fatalf("unexpected errors: %s", diags.Err())
	}

	got := strings.TrimSpace(buf.String())
	want := strings.TrimSpace(`
digraph {
  rankdir=TB;
  node [align=left,bgcolor="#ffffff",color="#000000",fontname=Helvetica,shape=rect];
  "foo.bar" [label="foo.bar",style=solid];
  "foo.baz[0]" [label="foo.baz[0]",style=solid];
  "foo.baz[1]" [label="foo.baz[1]",style=solid];
  "foo.beep[0]" [label="foo.beep[0]",style=solid];
  "foo.beep[1]" [label="foo.beep[1]",style=solid];
  "foo.deferred[*]" [label="foo.deferred[*]",style=dashed];
  "provider[\"registry.opentofu.org/test/foo\"]" [label="provider[\"registry.opentofu.org/test/foo\"]",style=solid];
  "foo.bar":s -> "foo.baz[0]":n;
  "foo.bar":s -> "foo.baz[1]":n;
  "foo.baz[0]":s -> "foo.beep[0]":n;
  "foo.baz[1]":s -> "foo.beep[1]":n;
  "provider[\"registry.opentofu.org/test/foo\"]":s -> "foo.bar":n;
  "provider[\"registry.opentofu.org/test/foo\"]":s -> "foo.deferred[*]":n;
}
`)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error("wrong result:\n" + diff)
	}

}
