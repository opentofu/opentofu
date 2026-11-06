// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"testing"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This file contains test helpers for use in other packages. These functions
// must not be used in non-test code.

// ModulesForTesting returns an [evalglue.ExternalModules] implementation that just
// returns module objects directly from the provided map, without any additional
// logic.
//
// This is intended for unit testing only, and only supports local module
// source addresses because it has no means to resolve remote sources or
// selected versions for registry-based modules.
//
// [configs.ModulesFromStringsForTesting] is a convenient way to build a
// suitable map to pass to this function when the required configuration is
// relatively small.
func ModulesForTesting(modules map[addrs.ModuleSourceLocal]*configs.Module) evalglue.ExternalModules {
	return externalModulesStatic{modules}
}

// ModuleFromSourceForTesting parses the given source code into a
// [*configs.Module] for use with [ModulesForTesting], or directly with
// [CompileModuleInstance].
//
// If the given source code cannot be loaded successfully, this function
// immediately halts testing using the given [testing.TB].
func ModuleFromSourceForTesting(t testing.TB, src string) *configs.Module {
	t.Helper()
	snap := &configload.Snapshot{
		Modules: map[string]*configload.SnapshotModule{
			"": {
				Dir: "",
				Files: map[string][]byte{
					"test.tofu": []byte(src),
				},
			},
		},
	}
	loader := configload.NewLoaderFromSnapshot(snap)
	mod, diags := loader.LoadConfigDir(".")
	if diags.HasErrors() {
		t.Fatalf("failed to load module: %s", diags.Error())
	}
	return mod
}

type externalModulesStatic struct {
	modules map[addrs.ModuleSourceLocal]*configs.Module
}

// ModuleConfig implements ExternalModules.
func (ms externalModulesStatic) ModuleConfig(_ context.Context, source addrs.ModuleSource, _ versions.Set, _ *addrs.AbsModuleCall) (evalglue.UncompiledModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	localSource, ok := source.(addrs.ModuleSourceLocal)
	if !ok {
		diags = diags.Append(fmt.Errorf("only local module source addresses are supported for this test"))
		return nil, diags
	}
	mod, ok := ms.modules[localSource]
	if !ok {
		diags = diags.Append(fmt.Errorf("module path %q is not available to this test", localSource))
		return nil, diags
	}
	return NewUncompiledModule(source, mod), diags
}

type evalGlueForTesting struct {
	prepareResourceInstanceValue func(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics)
}

var _ evalglue.Glue = (*evalGlueForTesting)(nil)

// ResourceInstanceValue implements [evalglue.Glue].
func (e *evalGlueForTesting) ResourceInstanceValue(ctx context.Context, ri *configgraph.ResourceInstance, configVal cty.Value, providerInst exprs.FromValue[*configgraph.ProviderInstance], riDeps addrs.Set[addrs.AbsResourceInstance]) (cty.Value, tfdiags.Diagnostics) {
	if e.prepareResourceInstanceValue == nil {
		// Default behavior is to just echo back the configuration value verbatim.
		return configVal, nil
	}
	return e.prepareResourceInstanceValue(ctx, ri, configVal, riDeps)
}

// ProviderFunction implements [evalglue.Glue].
func (e *evalGlueForTesting) ProviderFunction(ctx context.Context, provider addrs.Provider, providerInst exprs.FromValue[*configgraph.ProviderInstance], pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	return function.Function{}, tfdiags.New(fmt.Errorf("no provider functions available in this test"))
}
