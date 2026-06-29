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
	"time"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
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

	return experimentalRuntimeWanted()
}

func experimentalRuntimeWanted() bool {
	optIn := os.Getenv("TOFU_X_EXPERIMENTAL_RUNTIME")
	return optIn != ""
}

func (c *Context) newEngineShim(ctx context.Context, config *configs.Config, inputValuesRaw InputValues, planTimestamp time.Time, allowImpureFunctions bool) (*eval.ConfigInstance, plugins.Plugins, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	rawInput := map[string]cty.Value{}
	for key, value := range inputValuesRaw {
		if !value.Value.IsNull() {
			rawInput[key] = value.Value
		}
	}

	inputValues := exprs.ConstantValuer(cty.ObjectVal(rawInput))

	owd := "."
	if c.meta != nil && c.meta.OriginalWorkingDir != "" {
		owd = c.meta.OriginalWorkingDir
	}

	modules := c.modules
	if modules == nil {
		// testing fallback
		modules = newRuntimeModulesForTesting{config: config}
	}

	plugins := plugins.NewRuntimePluginsTemp(c.plugins.providers, c.plugins.provisioners)
	evalCtx := &eval.EvalContext{
		RootModuleDir:      config.Module.SourceDir,
		OriginalWorkingDir: owd,
		Modules:            modules,
		Providers:          plugins,
		Provisioners:       plugins,
		PlanTimestamp:      planTimestamp,
	}
	done := func() {
		// We'll call close with a cancel-free context because we do still
		// want to shut the providers down even if we're dealing with
		// graceful shutdown after cancellation.
		err := plugins.Close(context.WithoutCancel(ctx))
		// If a provider fails to close there isn't really much we can do
		// about that... this shouldn't really be possible unless the
		// plugin process already exited for some other reason anyway.
		if err != nil {
			log.Printf("[ERROR] plugin shutdown failed: %s", err.Error())
		}
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
		AllowImpureFunctions: allowImpureFunctions,
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

	configInst, _, done, moreDiags := c.newEngineShim(ctx, config, inputValues, time.Time{}, false)
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

	timestamp := time.Now().UTC()

	tracer := c.newEnginePlanTracer()
	ctx = planning.ContextWithTracer(ctx, tracer)

	configInst, plugins, done, moreDiags := c.newEngineShim(ctx, config, opts.SetVariables, timestamp, false)
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		return nil, diags
	}

	defer done()

	newOpts := &planning.PlanOpts{
		Mode: opts.Mode,
		// TODO: Most other things that are in this package's [PlanOpts]
		// package, though notably not "SetVariables" because the new runtime
		// deals with input variables during the module compilation step, rather
		// than directly during planning.
	}

	plan, moreDiags := planning.PlanChanges(ctx, newOpts, prevRoundState, configInst, plugins)
	if plan != nil {
		plan.Timestamp = timestamp
	}
	diags = diags.Append(moreDiags)
	return plan, diags
}

func (c *Context) newEnginePlanTracer() *planning.Tracer {
	// TODO: For now this just shims to our old Hook API as best we can. Once we
	// start using the new runtime directly instead of shimming it through
	// the old runtime's API we should let the CLI layer be responsible for
	// providing its own planning.PlanTracer directly, which it can then
	// use both to drive its own UI and to centralize our OpenTelemetry tracing
	// logic instead of having it spread all over the codebase.
	eachHook := func(fn func(Hook) (HookAction, error)) {
		for _, h := range c.hooks {
			action, err := fn(h)
			if err != nil {
				// The new runtime intentionally doesn't allow tracers to
				// force failure: this API is purely for passive tracing and
				// UI reporting. Therefore we'll just log the error and return.
				log.Printf("[ERROR] %T: %s", h, err)
				return
			}
			switch action {
			case HookActionContinue:
				continue
			case HookActionHalt:
				return
			}
		}
	}

	return &planning.Tracer{
		StartManagedResourceInstanceObjectRefresh: func(ctx context.Context, addr addrs.AbsResourceInstanceObject) context.Context {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			eachHook(func(h Hook) (HookAction, error) {
				return h.PreRefresh(inst, gen, cty.DynamicVal)
			})
			return ctx
		},
		EndManagedResourceInstanceObjectRefresh: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, refreshedVal cty.Value, diags tfdiags.Diagnostics) {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			eachHook(func(h Hook) (HookAction, error) {
				return h.PostRefresh(inst, gen, cty.DynamicVal, refreshedVal)
			})
		},
		StartManagedResourceInstanceObjectPlanChanges: func(ctx context.Context, addr addrs.AbsResourceInstanceObject) context.Context {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			eachHook(func(h Hook) (HookAction, error) {
				return h.PreDiff(inst, gen, cty.DynamicVal, cty.DynamicVal)
			})
			return ctx
		},
		EndManagedResourceInstanceObjectPlanChanges: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, plannedVal cty.Value, diags tfdiags.Diagnostics) {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			eachHook(func(h Hook) (HookAction, error) {
				// TODO: For now we're just always reporting plans.Update here
				// as a placeholder, since we're shimming hooks here primarily
				// for the benefit of the context tests and so we'll wait to
				// see if any of those tests need more than this before we add
				// that complexity.
				return h.PostDiff(inst, gen, plans.Update, cty.DynamicVal, plannedVal)
			})
		},
		StartDataResourceInstanceRead: func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context {
			eachHook(func(h Hook) (HookAction, error) {
				return h.PreRefresh(addr, addrs.CurrentResourceInstanceObjectGeneration, cty.DynamicVal)
			})
			return ctx
		},
		EndDataResourceInstanceRead: func(ctx context.Context, addr addrs.AbsResourceInstance, resultVal cty.Value, diags tfdiags.Diagnostics) {
			eachHook(func(h Hook) (HookAction, error) {
				return h.PostRefresh(addr, addrs.CurrentResourceInstanceObjectGeneration, cty.DynamicVal, resultVal)
			})
		},
	}
}

func (c *Context) newEngineApply(ctx context.Context, config *configs.Config, plan *plans.Plan, variables InputValues) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	log.Println("[WARN] Using apply implementation from the experimental language runtime")

	if len(plan.ExecutionGraph) == 0 && !plan.Changes.Empty() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Saved plan contains no execution graph",
			"The experimental new apply engine can only apply plans created by the experimental new planning engine.",
		))
		return nil, diags
	}

	configInst, plugins, done, moreDiags := c.newEngineShim(ctx, config, variables, plan.Timestamp, true)
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		return nil, diags
	}

	defer done()

	newState, moreDiags := applying.ApplyPlannedChanges(ctx, plan, configInst, plugins)
	diags = diags.Append(moreDiags)
	return newState, diags
}

type newRuntimeModulesForTesting struct {
	config *configs.Config
}

func (n newRuntimeModulesForTesting) ModuleConfig(ctx context.Context, source addrs.ModuleSource, allowedVersions versions.Set, forCall *addrs.AbsModuleCall) (eval.UncompiledModule, tfdiags.Diagnostics) {
	if forCall == nil {
		// Root Module
		if n.config.Module.ProviderRequirements == nil {
			// Broken tests
			n.config.Module.ProviderRequirements = &configs.RequiredProviders{}
		}
		return eval.PrepareTofu2024Module(source, n.config.Module), nil
	}
	path := forCall.Module.Module().Child(forCall.Call.Name)
	mod := n.config.Descendent(path)

	return eval.PrepareTofu2024Module(source, mod.Module), nil
}
