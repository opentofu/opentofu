// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty/function"
)

// compileCoreFunctions prepares the table of core functions for inclusion in
// a module instance scope.
func compileCoreFunctions(_ context.Context, allowImpureFuncs bool, baseDir string, planTimestamp time.Time) map[string]function.Function {
	// For now we just borrow the functions table setup from the previous
	// system's concept of "scope".
	oldScope := lang.Scope{
		PureOnly:      !allowImpureFuncs,
		BaseDir:       baseDir,
		PlanTimestamp: planTimestamp,
	}
	return oldScope.Functions()
}

// compileProviderFunctions builds a lookup function that can supply a cty function
// given a provider function address. This is wired into the module instance's scope
// to provide custom functions.
func compileProviderFunctions(
	reqdProviders map[string]*configs.RequiredProvider,
	providers configgraph.CompileProviderConfigRef,
	evaluationGlue evalglue.Glue,
) func(ctx context.Context, pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
	return func(ctx context.Context, pf addrs.ProviderFunction, rng hcl.Range) (function.Function, tfdiags.Diagnostics) {
		var fn function.Function
		var diags tfdiags.Diagnostics

		localAddr := addrs.LocalProviderConfig{
			LocalName: pf.ProviderName,
			Alias:     pf.ProviderAlias,
		}

		reqdProvider, ok := reqdProviders[localAddr.LocalName]
		if !ok {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unknown function provider",
				Detail:   fmt.Sprintf("Provider %q does not exist within the required_providers of this module", pf.ProviderName),
				Subject:  &rng,
			})
			return fn, diags
		}

		pv, moreDiags := providers(ctx, localAddr).Value(ctx)
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return fn, diags
		}

		maybeProvider, _, err := configgraph.ProviderInstanceFromValue(pv, reqdProvider.Type)
		if err != nil {
			diags = diags.Append(err)
			return fn, diags
		}

		fn, moreDiags = evaluationGlue.ProviderFunction(ctx, reqdProvider.Type, maybeProvider, pf, rng)
		diags = diags.Append(moreDiags)
		if diags.HasErrors() {
			return fn, diags
		}

		// We have the option here to attatch the provider's resource marks to the value returned by the function.  Should we?
		// Does this have CBD implications?

		return fn, diags
	}
}
