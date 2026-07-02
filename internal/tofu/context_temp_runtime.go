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
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/engine/applying"
	"github.com/opentofu/opentofu/internal/engine/planning"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/shared"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
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

func (c *Context) newEngineShim(ctx context.Context, config *configs.Config, inputValuesRaw InputValues, planTimestamp time.Time, allowImpureFunctions bool, applying bool) (*eval.ConfigInstance, plugins.Plugins, func(), tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	rawInput := map[string]cty.Value{}
	for key, value := range inputValuesRaw {
		if !value.Value.IsNull() {
			rawInput[key] = value.Value
		}
	}

	inputValues := exprs.ConstantValuer(cty.ObjectVal(rawInput))

	workspace := ""
	if c.meta != nil {
		workspace = c.meta.Env
	}
	owd := "."
	if c.meta != nil && c.meta.OriginalWorkingDir != "" {
		owd = c.meta.OriginalWorkingDir
	}

	// The current working directory should always be absolute, whether we
	// just looked it up or whether we were relying on ContextMeta's
	// (possibly non-normalized) path.
	owd, err := filepath.Abs(owd)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Failed to get working directory`,
			Detail:   fmt.Sprintf(`The value for the original working directory cannot be determined due to a system error: %s`, err),
		})
		return nil, nil, nil, diags
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
		Applying:           applying,
		Workspace:          workspace,
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

	configInst, _, done, moreDiags := c.newEngineShim(ctx, config, inputValues, time.Time{}, false, false)
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

	configInst, plugins, done, moreDiags := c.newEngineShim(ctx, config, opts.SetVariables, timestamp, false, false)
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

	return &planning.Tracer{
		StartManagedResourceInstanceObjectRefresh: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, prevRoundVal cty.Value) context.Context {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PreRefresh(inst, gen, prevRoundVal)
			})
			return ctx
		},
		EndManagedResourceInstanceObjectRefresh: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, prevRoundVal, refreshedVal cty.Value, diags tfdiags.Diagnostics) {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostRefresh(inst, gen, prevRoundVal, refreshedVal)
			})
		},
		StartManagedResourceInstanceObjectPlanChanges: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, priorVal, configVal cty.Value) context.Context {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				// TODO: We're sending the configVal in the slot where the
				// Hook API expects the "proposed new value", and that isn't
				// quite right. Does that matter for the current real-world
				// use of the hook API?
				// The new runtime intentionally buries the "proposed new value"
				// in the implementation details of the provider call since
				// it's a quirky part of the protocol that we preserve only
				// for compatibility.
				return h.PreDiff(inst, gen, priorVal, configVal)
			})
			return ctx
		},
		EndManagedResourceInstanceObjectPlanChanges: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, action plans.Action, priorVal, plannedVal cty.Value, diags tfdiags.Diagnostics) {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostDiff(inst, gen, action, priorVal, plannedVal)
			})
		},
		StartDataResourceInstanceRead: func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context {
			c.eachHook(func(h Hook) (HookAction, error) {
				// The prior value for a data resource instance is always null
				// because conceptually it is always read anew for each round.
				// (It's retained in the state as a convenience for unusual
				// situations like "tofu console", but the prior state value
				// cannot be used in the main codepath because the protocol
				// includes no way to "upgrade" when the provider schema changes.)
				return h.PreRefresh(addr, addrs.CurrentResourceInstanceObjectGeneration, cty.NullVal(cty.DynamicPseudoType))
			})
			return ctx
		},
		EndDataResourceInstanceRead: func(ctx context.Context, addr addrs.AbsResourceInstance, resultVal cty.Value, diags tfdiags.Diagnostics) {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostRefresh(addr, addrs.CurrentResourceInstanceObjectGeneration, cty.NullVal(cty.DynamicPseudoType), resultVal)
			})
		},

		// We'll also include the [shared.Tracer] we use for both plan and apply.
		Tracer: c.newEngineSharedTracer(),
	}
}

func (c *Context) newEngineApply(ctx context.Context, config *configs.Config, plan *plans.Plan, variables InputValues) (*states.State, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	log.Println("[WARN] Using apply implementation from the experimental language runtime")

	if len(plan.ExecutionGraph) == 0 && !plan.Changes.ResourcesEmpty() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Saved plan contains no execution graph",
			"The experimental new apply engine can only apply plans created by the experimental new planning engine.",
		))
		return nil, diags
	}

	tracer := c.newEngineApplyTracer()
	ctx = applying.ContextWithTracer(ctx, tracer)

	configInst, plugins, done, moreDiags := c.newEngineShim(ctx, config, variables, plan.Timestamp, true, true)
	diags = diags.Append(moreDiags)

	if diags.HasErrors() {
		return nil, diags
	}

	defer done()

	newState, moreDiags := applying.ApplyPlannedChanges(ctx, plan, configInst, plugins)
	diags = diags.Append(moreDiags)
	return newState, diags
}

func (c *Context) newEngineApplyTracer() *applying.Tracer {
	// TODO: For now this just shims to our old Hook API as best we can. Once we
	// start using the new runtime directly instead of shimming it through
	// the old runtime's API we should let the CLI layer be responsible for
	// providing its own planning.PlanTracer directly, which it can then
	// use both to drive its own UI and to centralize our OpenTelemetry tracing
	// logic instead of having it spread all over the codebase.

	// TODO: shimPlanAction is a very rough approximation of deciding a
	// [plans.Action] based on the prior and planned value, dealing with the
	// fact that in the new runtime the "planned action" is primarily a UI
	// thing used by the planning engine to describe to the user what it is
	// proposing to change. The applying engine has no need for this because
	// "planned action" is not represented anywhere in the provider protocol.
	// This is a temporary shim until we decide whose job it should be to decide
	// the planned action under the new runtime... hopefully it becomes purely
	// a UI concern that even the planning engine doesn't need to care about,
	// but that remains to be seen once we design new plan models matching how
	// the new runtime prefers to think about changes.
	shimPlanAction := func(priorVal, plannedVal cty.Value) plans.Action {
		// Note that we don't need to handle the "replace" actions here because
		// by the time we're in the apply phase they've already been decomposed
		// into their separate Create and Destroy legs.
		if priorVal.IsNull() {
			return plans.Create
		}
		if plannedVal.IsNull() {
			return plans.Delete
		}
		return plans.Update
	}

	return &applying.Tracer{
		StartManagedResourceInstanceObjectFinalPlan: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, priorVal, configVal, expectedVal cty.Value) context.Context {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				// TODO: We're sending the expectedVal in the slot where the
				// Hook API expects the "proposed new value", and that isn't
				// quite right. Does that matter for the current real-world
				// use of the hook API?
				// The new runtime intentionally buries the "proposed new value"
				// in the implementation details of the provider call since
				// it's a quirky part of the protocol that we preserve only
				// for compatibility.
				return h.PreDiff(inst, gen, priorVal, expectedVal)
			})
			return ctx
		},
		EndManagedResourceInstanceObjectFinalPlan: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, priorVal, plannedVal cty.Value, diags tfdiags.Diagnostics) {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostDiff(inst, gen, shimPlanAction(priorVal, plannedVal), priorVal, plannedVal)
			})
		},
		StartManagedResourceInstanceObjectApply: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, priorVal, plannedVal cty.Value) context.Context {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PreApply(inst, gen, shimPlanAction(priorVal, plannedVal), priorVal, plannedVal)
			})
			return ctx
		},
		EndManagedResourceInstanceObjectApply: func(ctx context.Context, addr addrs.AbsResourceInstanceObject, resultVal cty.Value, diags tfdiags.Diagnostics) {
			inst := addr.InstanceAddr
			gen := addr.DeposedKey.Generation()
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostApply(inst, gen, resultVal, diags.Err())
			})
			// TODO: the CLI layer and some of our tests also expect to get a
			// PostStateUpdate call each time the state might have changed,
			// but supporting that here would require us to either expose the
			// apply engine's internal working state or to copy it each time
			// we apply something, and we're trying to move away from there
			// being a single big state object that everything is interacting
			// with so we'll need to think about what compromise is best to
			// make here.
		},
		StartDataResourceInstanceRead: func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context {
			c.eachHook(func(h Hook) (HookAction, error) {
				// The prior value for a data resource instance is always null
				// because conceptually it is always read anew for each round.
				// (It's retained in the state as a convenience for unusual
				// situations like "tofu console", but the prior state value
				// cannot be used in the main codepath because the protocol
				// includes no way to "upgrade" when the provider schema changes.)
				return h.PreRefresh(addr, addrs.CurrentResourceInstanceObjectGeneration, cty.NullVal(cty.DynamicPseudoType))
			})
			return ctx
		},
		EndDataResourceInstanceRead: func(ctx context.Context, addr addrs.AbsResourceInstance, resultVal cty.Value, diags tfdiags.Diagnostics) {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostRefresh(addr, addrs.CurrentResourceInstanceObjectGeneration, cty.NullVal(cty.DynamicPseudoType), resultVal)
			})
		},

		// We'll also include the [shared.Tracer] we use for both plan and apply.
		Tracer: c.newEngineSharedTracer(),
	}
}

func (c *Context) newEngineSharedTracer() shared.Tracer {
	// TODO: For now this just shims to our old Hook API as best we can. Once we
	// start using the new runtime directly instead of shimming it through
	// the old runtime's API we should let the CLI layer be responsible for
	// providing its own planning.PlanTracer directly, which it can then
	// use both to drive its own UI and to centralize our OpenTelemetry tracing
	// logic instead of having it spread all over the codebase.

	return shared.Tracer{
		StartEphemeralResourceInstanceOpen: func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PreOpen(addr)
			})
			return ctx
		},
		EndEphemeralResourceInstanceOpen: func(ctx context.Context, addr addrs.AbsResourceInstance, diags tfdiags.Diagnostics) {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostOpen(addr, diags.Err())
			})
		},
		StartEphemeralResourceInstanceRenew: func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PreRenew(addr)
			})
			return ctx
		},
		EndEphemeralResourceInstanceRenew: func(ctx context.Context, addr addrs.AbsResourceInstance, diags tfdiags.Diagnostics) {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostRenew(addr, diags.Err())
			})
		},
		StartEphemeralResourceInstanceClose: func(ctx context.Context, addr addrs.AbsResourceInstance) context.Context {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PreClose(addr)
			})
			return ctx
		},
		EndEphemeralResourceInstanceClose: func(ctx context.Context, addr addrs.AbsResourceInstance, diags tfdiags.Diagnostics) {
			c.eachHook(func(h Hook) (HookAction, error) {
				return h.PostClose(addr, diags.Err())
			})
		},
	}
}

func (c *Context) eachHook(fn func(Hook) (HookAction, error)) {
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
