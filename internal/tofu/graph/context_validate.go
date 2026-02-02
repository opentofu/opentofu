package graph

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/importing"
	"github.com/opentofu/opentofu/internal/tofu/variables"
)

func (c *Context) Validate(ctx context.Context, config *configs.Config, varValues variables.InputValues, importTargets []*importing.ImportTarget) tfdiags.Diagnostics {
	defer c.acquireRun("validate")()

	var diags tfdiags.Diagnostics

	providerFunctionTracker := make(ProviderFunctionMapping)

	graph, moreDiags := (&PlanGraphBuilder{
		Config:                  config,
		Plugins:                 c.plugins,
		State:                   states.NewState(),
		RootVariableValues:      varValues,
		Operation:               walkValidate,
		ProviderFunctionTracker: providerFunctionTracker,
		ImportTargets:           importTargets,
	}).Build(ctx, addrs.RootModuleInstance)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return diags
	}

	walker, walkDiags := c.walk(ctx, graph, walkValidate, &graphWalkOpts{
		Config:                  config,
		ProviderFunctionTracker: providerFunctionTracker,
	})
	diags = diags.Append(walker.NonFatalDiagnostics)
	diags = diags.Append(walkDiags)
	if walkDiags.HasErrors() {
		return diags
	}

	return diags
}
