// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/engine/applying"
	"github.com/opentofu/opentofu/internal/engine/planning"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

/////////////////////////
// The definitions in this file are intended as temporary shims to help support
// the development of the new runtime engine, by allowing experiments-enabled
// builds to be opted in to the new implementation by setting the environment
// variable TOFU_X_EXPERIMENTAL_RUNTIME to any non-empty value.
//
// These shims should remain here only as long as the new runtime engine is
// under active development and is not yet adopted as the primary engine. It's
// also acceptable for work being done for other separate projects to ignore
// these shims and let this code become broken, as long as the code continues
// to compile: only those working on the implementation of the new engine are
// responsible for updating this if the rest of the system evolves to the point
// of that being necessary.
/////////////////////////

// SetExperimentalRuntimeAllowed must be called with the argument set to true
// at some point before calling [New] or [NewWithBackend] in order for the
// experimental opt-in to be effective.
//
// In practice this is called by code in the "command" package early in the
// backend initialization codepath and enables the experimental runtime only
// in an experiments-enabled OpenTofu build, to make sure that it's not
// possible to accidentally enable this experimental functionality in normal
// release builds.
//
// Refer to "cmd/tofu/experiments.go" for information on how to produce an
// experiments-enabled build.
func SetExperimentalRuntimeAllowed(allowed bool) {
	experimentalRuntimeAllowed.Store(allowed)
}

var experimentalRuntimeAllowed atomic.Bool

func experimentalRuntimeEnabled() bool {
	if !experimentalRuntimeAllowed.Load() {
		// The experimental runtime is never enabled when it hasn't been
		// explicitly allowed.
		return false
	}

	optIn := os.Getenv("TOFU_X_EXPERIMENTAL_RUNTIME")
	return optIn != ""
}

func (c *Context) newEngineShim(ctx context.Context, config *configs.Config, inputValuesRaw InputValues) (*eval.ConfigInstance, plugins.Plugins, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	rawInput := map[string]cty.Value{}
	for key, value := range inputValuesRaw {
		if !value.Value.IsNull() {
			rawInput[key] = value.Value
		}
	}

	inputValues := exprs.ConstantValuer(cty.ObjectVal(rawInput))

	tempLoader, _ := configload.NewLoader(&configload.Config{})

	plugins := plugins.NewRuntimePlugins(c.plugins.providerFactories, c.plugins.provisionerFactories)
	evalCtx := &eval.EvalContext{
		RootModuleDir:      config.Module.SourceDir,
		OriginalWorkingDir: c.meta.OriginalWorkingDir,
		Modules: &newRuntimeModules{
			loader: tempLoader,
		},
		Providers:    plugins,
		Provisioners: plugins,
	}
	done := func() {
		// We'll call close with a cancel-free context because we do still
		// want to shut the providers down even if we're dealing with
		// graceful shutdown after cancellation.
		err := plugins.Close(context.WithoutCancel(ctx))
		// If a provider fails to close there isn't really much we can do
		// about that... this shouldn't really be possible unless the
		// plugin process already exited for some other reason anyway.
		log.Printf("[ERROR] plugin shutdown failed: %s", err)
	}

	// The new config-loading system wants to work in terms of module source
	// addresses rather than raw local filenames, so we'll ask the
	// addrs package to parse the path we were given. We need to adjust
	// a little though, because this function was designed for parsing
	// the "source" argument in a module block, not a plain filepath.
	// We should add a function in package addrs that's actually intended for
	// turning arbitrary filesystem paths in to addrs.LocalSource in the long
	// run, but this will do for now.
	configDir := config.Module.SourceDir
	if !filepath.IsAbs(configDir) {
		configDir = "." + string(filepath.Separator) + configDir
	}
	rootModuleSource, err := addrs.ParseModuleSource(configDir)
	if err != nil {
		diags = diags.Append(fmt.Errorf("invalid root module source address: %w", err))
		return nil, nil, done, diags
	}

	configCall := &eval.ConfigCall{
		RootModuleSource:     rootModuleSource,
		InputValues:          inputValues,
		AllowImpureFunctions: false,
		EvalContext:          evalCtx,
	}
	configInst, moreDiags := eval.NewConfigInstance(ctx, configCall)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return nil, nil, done, diags
	}
	return configInst, plugins, done, diags
}

func (c *Context) newEngineValidate(ctx context.Context, config *configs.Config, inputValues InputValues) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	log.Println("[WARN] Using validate implementation from the experimental language runtime")

	configInst, _, done, moreDiags := c.newEngineShim(ctx, config, inputValues)
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		return diags
	}

	defer done()

	moreDiags = configInst.Validate(ctx)
	diags = diags.Append(moreDiags)
	return diags
}

func (c *Context) newEnginePlan(ctx context.Context, config *configs.Config, prevRoundState *states.State, opts *PlanOpts) (*plans.Plan, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	log.Println("[WARN] Using plan implementation from the experimental language runtime")

	configInst, plugins, done, moreDiags := c.newEngineShim(ctx, config, opts.SetVariables)
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		return nil, diags
	}

	defer done()

	plan, moreDiags := planning.PlanChanges(ctx, prevRoundState, configInst, plugins)
	diags = diags.Append(moreDiags)
	return plan, diags
}

func (c *Context) newEngineApply(ctx context.Context, config *configs.Config, plan *plans.Plan, variables InputValues) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	log.Println("[WARN] Using apply implementation from the experimental language runtime")

	if len(plan.ExecutionGraph) == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Saved plan contains no execution graph",
			"The experimental new apply engine can only apply plans created by the experimental new planning engine.",
		))
		return nil, diags
	}

	configInst, plugins, done, moreDiags := c.newEngineShim(ctx, config, variables)
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		return nil, diags
	}

	defer done()

	newState, moreDiags := applying.ApplyPlannedChanges(ctx, plan, configInst, plugins)
	diags = diags.Append(moreDiags)
	return newState, diags
}

// newRuntimeModules is an implementation of [eval.ExternalModules] that makes
// a best effort to shim to OpenTofu's current module loader, even though
// it works in some slightly-different terms than this new API expects.
type newRuntimeModules struct {
	loader *configload.Loader
}

var _ eval.ExternalModules = (*newRuntimeModules)(nil)

// ModuleConfig implements evalglue.ExternalModules.
func (n *newRuntimeModules) ModuleConfig(ctx context.Context, source addrs.ModuleSource, allowedVersions versions.Set, forCall *addrs.AbsModuleCall) (eval.UncompiledModule, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	var sourceDir string
	switch source := source.(type) {
	case addrs.ModuleSourceLocal:
		sourceDir = filepath.Clean(filepath.FromSlash(string(source)))
	default:
		// For this early stub implementation we only support local source
		// addresses. We'll expand this later but that'll require this codepath
		// to have access to the information about what's in the module cache
		// directory at ".terraform/modules", which we've not arranged for yet.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"New runtime codepath only supports local module sources",
			fmt.Sprintf("Cannot load %q, because our temporary codepath for the new language runtime only supports local module sources for now.", source),
		))
		return nil, diags
	}
	log.Printf("[TRACE] backend/local: Loading module from %q from local path %q", source, sourceDir)

	mod, hclDiags := n.loader.Parser().LoadConfigDirUneval(sourceDir, configs.SelectiveLoadAll)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return nil, diags
	}

	return eval.PrepareTofu2024Module(source, mod), diags
}
