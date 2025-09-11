// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"testing"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestModuleCall_Value(t *testing.T) {
	testValuer(t, map[string]valuerTest[*ModuleCall]{
		"remote source address": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("./"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("https://example.com/foo"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(cty.NullVal(cty.String))),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.ObjectVal(map[string]cty.Value{
				"source": cty.StringVal("https://example.com/foo"),
			}),
			nil,
		},
		"registry source address with no version": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("./"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("example.com/foo/bar/baz"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(cty.NullVal(cty.String))),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.ObjectVal(map[string]cty.Value{
				"source": cty.StringVal("example.com/foo/bar/baz"),
			}),
			nil,
		},
		"registry source address with version": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("./"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("example.com/foo/bar/baz"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("<= 2.0.0"),
				)),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.ObjectVal(map[string]cty.Value{
				"source": cty.StringVal("example.com/foo/bar/baz"),
			}),
			nil,
		},
		"local source address relative to local": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("./modules/beep"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("../boop"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(cty.NullVal(cty.String))),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.ObjectVal(map[string]cty.Value{
				"source": cty.StringVal("./modules/boop"),
			}),
			nil,
		},
		"local source address relative to remote": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("https://example.com/foo.tar.gz//beep/beep"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("../boop"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(cty.NullVal(cty.String))),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.ObjectVal(map[string]cty.Value{
				"source": cty.StringVal("https://example.com/foo.tar.gz//beep/boop"),
			}),
			nil,
		},
		"local source address relative to remote escapes": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("https://example.com/foo.tar.gz//beep/beep"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("../../../outside"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(cty.NullVal(cty.String))),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.DynamicVal,
			diagsForTest(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid module source address",
				`Cannot use "../../../outside" as module source address: invalid relative path from https://example.com/foo.tar.gz//beep/beep: relative path ../../../outside has too many "../" segments.`,
			)),
		},
		"local source address relative to registry": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("example.com/foo/bar/baz//beep/beep"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("../boop"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(cty.NullVal(cty.String))),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.ObjectVal(map[string]cty.Value{
				"source": cty.StringVal("example.com/foo/bar/baz//beep/boop"),
			}),
			nil,
		},
		"local source address relative to registry escapes": {
			&ModuleCall{
				ParentSourceAddr: addrs.MustParseModuleSource("example.com/foo/bar/baz//beep/beep"),
				InstanceSelector: singleInstanceSelectorForTesting(),
				SourceAddrValuer: ValuerOnce(exprs.ConstantValuer(
					cty.StringVal("../../../outside"),
				)),
				VersionConstraintValuer: ValuerOnce(exprs.ConstantValuer(cty.NullVal(cty.String))),
				ValidateSourceArguments: func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics {
					return nil
				},
				CompileCallInstance: compileModuleCallInstanceSimpleForTesting,
			},
			cty.DynamicVal,
			diagsForTest(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid module source address",
				`Cannot use "../../../outside" as module source address: invalid relative path from example.com/foo/bar/baz//beep/beep: relative path ../../../outside has too many "../" segments.`,
			)),
		},
	})
}

// compileModuleCallInstanceSimpleForTesting is a simple implementation of
// [ModuleCall.CompileCallInstance] that just returns a stub
// [ModuleCallInstance] with its glue set to an instance of
// [moduleInstanceGlueForTesting], so that the instance's result value
// will be an object with "source" and "config" attributes just echoing
// back the source that was passed in with an empty config.
func compileModuleCallInstanceSimpleForTesting(ctx context.Context, sourceArgs ModuleSourceArguments, key addrs.InstanceKey, repData instances.RepetitionData) *ModuleCallInstance {
	return &ModuleCallInstance{
		Glue: &moduleInstanceGlueForTesting{
			sourceAddr: sourceArgs.Source.String(),
		},
		InputsValuer: ValuerOnce(exprs.ConstantValuer(cty.EmptyObjectVal)),
	}
}
