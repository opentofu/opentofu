// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/davecgh/go-spew/spew"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/engine/applying"
	"github.com/opentofu/opentofu/internal/engine/planning"
	"github.com/opentofu/opentofu/internal/engine/plugins"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/planfile"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
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
//
// Note that "tofu validate" is implemented outside of the backend abstraction
// and so does not respond to the experiment opt-in environment variable. For
// now, try out validation-related behaviors of the new runtime through
// "tofu plan" instead, which should implement a superset of the validation
// behavior.
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

func (b *Local) opPlanWithExperimentalRuntime(stopCtx context.Context, cancelCtx context.Context, op *backend.Operation, runningOp *backend.RunningOperation) {
	var diags tfdiags.Diagnostics
	log.Println("[WARN] Using plan implementation from the experimental language runtime")

	// Currently we're using the caller's "stopCtx" as the main context, using
	// it both for its values and as a signal for graceful shutdown. This is
	// just to get the closest fit with how current callers of the backend
	// API populate these contexts with what the new runtime is expecting. We
	// should revisit this and make sure this still makes sense before we
	// finalize any implementation here.
	ctx := stopCtx

	if op.PlanFile != nil {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Can't re-plan a saved plan",
			"The plan command was given a saved plan file as its input. This command generates "+
				"a new plan, and so it requires a configuration directory as its argument.",
		))
		op.ReportResult(runningOp, diags)
		return
	}
	// Local planning requires a config, unless we're planning to destroy.
	if op.PlanMode != plans.DestroyMode && !op.HasConfig() {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"No configuration files",
			"Plan requires configuration to be present. Planning without a configuration would "+
				"mark everything for destruction, which is normally not what is desired. If you "+
				"would like to destroy everything, run plan with the -destroy option. Otherwise, "+
				"create a OpenTofu configuration file (.tf file) and try again.",
		))
		op.ReportResult(runningOp, diags)
		return
	}
	if len(op.GenerateConfigOut) > 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Config generation not supported",
			"The experimental language runtime cannot yet support -generate-config-out.",
		))
		op.ReportResult(runningOp, diags)
		return
	}

	// The following is a limited inline reimplementation of the just parts of
	// Local.localRun that we need to start executing the new runtime, since
	// that codepath is currently quite specific to the needs of the old
	// runtime. At a later point we'll want to consolidate these back together
	// again somehow, but this is just enough to help us do basix execution
	// during the "walking skeleton" phase of the project.
	stateMgr, err := b.StateMgr(ctx, op.Workspace)
	if err != nil {
		diags = diags.Append(fmt.Errorf("error loading state: %w", err))
		op.ReportResult(runningOp, diags)
		return
	}
	prevRoundState, err := statemgr.RefreshAndRead(ctx, stateMgr)
	if err != nil {
		diags = diags.Append(fmt.Errorf("error loading state: %w", err))
		op.ReportResult(runningOp, diags)
		return
	}
	if prevRoundState == nil {
		prevRoundState = states.NewState() // this is the first round, starting with an empty state
	}

	plugins := plugins.NewRuntimePlugins(b.ContextOpts.Providers, b.ContextOpts.Provisioners)
	evalCtx := &eval.EvalContext{
		RootModuleDir:      op.ConfigDir,
		OriginalWorkingDir: b.ContextOpts.Meta.OriginalWorkingDir,
		Modules: &newRuntimeModules{
			loader: op.ConfigLoader,
		},
		Providers:    plugins,
		Provisioners: plugins,
	}
	defer func() {
		// We'll call close with a cancel-free context because we do still
		// want to shut the providers down even if we're dealing with
		// graceful shutdown after cancellation.
		err := plugins.Close(context.WithoutCancel(ctx))
		// If a provider fails to close there isn't really much we can do
		// about that... this shouldn't really be possible unless the
		// plugin process already exited for some other reason anyway.
		log.Printf("[ERROR] plugin shutdown failed: %s", err)
	}()

	// The new config-loading system wants to work in terms of module source
	// addresses rather than raw local filenames, so we'll ask the
	// addrs package to parse the path we were given. We need to adjust
	// a little though, because this function was designed for parsing
	// the "source" argument in a module block, not a plain filepath.
	// We should add a function in package addrs that's actually intended for
	// turning arbitrary filesystem paths in to addrs.LocalSource in the long
	// run, but this will do for now.
	configDir := op.ConfigDir
	if !filepath.IsAbs(configDir) {
		configDir = "." + string(filepath.Separator) + configDir
	}
	rootModuleSource, err := addrs.ParseModuleSource(configDir)
	if err != nil {
		diags = diags.Append(fmt.Errorf("invalid root module source address: %w", err))
		op.ReportResult(runningOp, diags)
		return
	}
	configCall := &eval.ConfigCall{
		RootModuleSource: rootModuleSource,
		// TODO: InputValues
		AllowImpureFunctions: false,
		EvalContext:          evalCtx,
	}
	configInst, moreDiags := eval.NewConfigInstance(ctx, configCall)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		op.ReportResult(runningOp, diags)
		return
	}

	plan, moreDiags := planning.PlanChanges(ctx, prevRoundState, configInst, plugins)
	diags = diags.Append(moreDiags)
	// We intentionally continue with errors here because we make a best effort
	// to render a partial plan output even when we have errors, in case
	// the partial plan is helpful for debugging.

	// Even if there are errors we need to handle anything that may be
	// contained within the plan, so only exit if there is no data at all.
	if plan == nil {
		runningOp.PlanEmpty = true
		op.ReportResult(runningOp, diags)
		return
	}

	// Record whether this plan includes any side-effects that could be applied.
	runningOp.PlanEmpty = !plan.CanApply()

	wroteConfig := false
	if path := op.PlanOutPath; path != "" {
		if op.PlanOutBackend == nil {
			// This is always a bug in the operation caller; it's not valid
			// to set PlanOutPath without also setting PlanOutBackend.
			diags = diags.Append(fmt.Errorf(
				"PlanOutPath set without also setting PlanOutBackend (this is a bug in OpenTofu)"),
			)
			op.ReportResult(runningOp, diags)
			return
		}
		plan.Backend = *op.PlanOutBackend

		// We may have updated the state in the refresh step above, but we
		// will freeze that updated state in the plan file for now and
		// only write it if this plan is subsequently applied.
		plannedStateFile := statemgr.PlannedStateUpdate(stateMgr, plan.PriorState)

		// We also include a file containing the state as it existed before
		// we took any action at all, but this one isn't intended to ever
		// be saved to the backend (an equivalent snapshot should already be
		// there) and so we just use a stub state file header in this case.
		// NOTE: This won't be exactly identical to the latest state snapshot
		// in the backend because it's still been subject to state upgrading
		// to make it consumable by the current OpenTofu version, and
		// intentionally doesn't preserve the header info.
		prevStateFile := &statefile.File{
			State: plan.PrevRunState,
		}

		// TEMP: FIXME: The planfile structure currently includes a "config
		// snapshot" which only our traditional config loading codepath
		// knows how to construct. For our "walking skeleton" milestone we'll
		// just leave that snapshot empty and rely on the main configuration
		// directory directly during the apply phase, since we're not sure
		// yet whether we're going to bring our traditional plan file format
		// with us into the new implementation.
		configSnap := configload.NewEmptySnapshot()

		log.Printf("[INFO] backend/local: writing plan output to: %s", path)
		err := planfile.Create(path, planfile.CreateArgs{
			ConfigSnapshot:       configSnap,
			PreviousRunStateFile: prevStateFile,
			StateFile:            plannedStateFile,
			Plan:                 plan,
			DependencyLocks:      op.DependencyLocks,
		}, op.Encryption.Plan())
		if err != nil {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Failed to write plan file",
				fmt.Sprintf("The plan file could not be written: %s.", err),
			))
			op.ReportResult(runningOp, diags)
			return
		}
	}

	// TODO: Actually render the plan. But to do that we need provider schemas
	// and our schema-loading code expects us to be holding an old-style
	// *configs.Config, so we have some more work to do before we can do that.
	// For now, we'll just show the internals of the plan object.
	// (Note that we are expecting that [planning.PlanChanges] will return
	// something other than [plans.Plan] before long, because in our new
	// approach we want to save the execution graph as part of the plan and
	// so our old model is not sufficient. This is just a placeholder for now.)
	spew.Dump(plan)

	// If we've accumulated any diagnostics along the way then we'll show them
	// here just before we show the summary and next steps. This can potentially
	// include errors, because we intentionally try to show a partial plan
	// above even if OpenTofu Core encountered an error partway through
	// creating it.
	op.ReportResult(runningOp, diags)

	if !runningOp.PlanEmpty {
		if wroteConfig {
			op.View.PlanNextStep(op.PlanOutPath, op.GenerateConfigOut)
		} else {
			op.View.PlanNextStep(op.PlanOutPath, "")
		}
	}
}

func (b *Local) opApplyWithExperimentalRuntime(stopCtx context.Context, cancelCtx context.Context, op *backend.Operation, runningOp *backend.RunningOperation) {
	var diags tfdiags.Diagnostics
	log.Println("[WARN] Using apply implementation from the experimental language runtime")

	// Currently we're using the caller's "stopCtx" as the main context, using
	// it both for its values and as a signal for graceful shutdown. This is
	// just to get the closest fit with how current callers of the backend
	// API populate these contexts with what the new runtime is expecting. We
	// should revisit this and make sure this still makes sense before we
	// finalize any implementation here.
	ctx := stopCtx

	if op.PlanFile == nil {
		// TODO: Implement inline planning for the one-shot "tofu apply" command.
		// (For now we only support applying a plan file created by an earlier
		// run of "tofu plan -out=FILENAME".)
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Operation unsupported in experimental language runtime",
			"The command \"tofu apply\" currently requires a saved plan file when using the experimental language runtime.",
		))
		op.ReportResult(runningOp, diags)
		return
	}

	lr, _, opState, contextDiags := b.localRun(ctx, op)
	diags = diags.Append(contextDiags)
	if contextDiags.HasErrors() {
		op.ReportResult(runningOp, diags)
		return
	}
	// the state was locked during successful context creation; unlock the state
	// when the operation completes
	defer func() {
		diags := op.StateLocker.Unlock()
		if diags.HasErrors() {
			op.View.Diagnostics(diags)
			runningOp.Result = backend.OperationFailure
		}
	}()

	// We'll start off with our result being the input state, and replace it
	// with the result state only if we eventually complete the apply
	// operation.
	runningOp.State = lr.InputState

	plan := lr.Plan
	if plan.Errored {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Cannot apply incomplete plan",
			"OpenTofu encountered an error when generating this plan, so it cannot be applied.",
		))
		op.ReportResult(runningOp, diags)
		return
	}
	if len(plan.ExecutionGraph) == 0 {
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Saved plan contains no execution graph",
			"The experimental new apply engine can only apply plans created by the experimental new planning engine.",
		))
		op.ReportResult(runningOp, diags)
		return
	}

	plugins := plugins.NewRuntimePlugins(b.ContextOpts.Providers, b.ContextOpts.Provisioners)
	evalCtx := &eval.EvalContext{
		RootModuleDir:      op.ConfigDir,
		OriginalWorkingDir: b.ContextOpts.Meta.OriginalWorkingDir,
		Modules: &newRuntimeModules{
			loader: op.ConfigLoader,
		},
		Providers:    plugins,
		Provisioners: plugins,
	}
	defer func() {
		// We'll call close with a cancel-free context because we do still
		// want to shut the providers down even if we're dealing with
		// graceful shutdown after cancellation.
		err := plugins.Close(context.WithoutCancel(ctx))
		// If a provider fails to close there isn't really much we can do
		// about that... this shouldn't really be possible unless the
		// plugin process already exited for some other reason anyway.
		log.Printf("[ERROR] plugin shutdown failed: %s", err)
	}()

	// FIXME: The configuration during apply is supposed to come from the
	// saved plan file rather than the real filesystem, but our codepaths for
	// that are all tangled up with the old-style config loader and so we
	// can't use them here. For now we just load the config directly from the
	// working directory just like the plan phase does, but we should eventually
	// implement this properly.
	configDir := op.ConfigDir
	if !filepath.IsAbs(configDir) {
		configDir = "." + string(filepath.Separator) + configDir
	}
	rootModuleSource, err := addrs.ParseModuleSource(configDir)
	if err != nil {
		diags = diags.Append(fmt.Errorf("invalid root module source address: %w", err))
		op.ReportResult(runningOp, diags)
		return
	}
	configCall := &eval.ConfigCall{
		RootModuleSource: rootModuleSource,
		// TODO: InputValues
		AllowImpureFunctions: false,
		EvalContext:          evalCtx,
	}
	configInst, moreDiags := eval.NewConfigInstance(ctx, configCall)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		op.ReportResult(runningOp, diags)
		return
	}

	newState, moreDiags := applying.ApplyPlannedChanges(ctx, plan, configInst, plugins)
	diags = diags.Append(moreDiags)

	// TODO: Actually save the new state. For now we just print it out.
	_ = opState
	spew.Dump(newState)

	op.ReportResult(runningOp, diags)
}

func (b *Local) opRefreshWithExperimentalRuntime(stopCtx context.Context, cancelCtx context.Context, op *backend.Operation, runningOp *backend.RunningOperation) {
	log.Println("[WARN] Using refresh implementation from the experimental language runtime")
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Operation unsupported in experimental language runtime",
		"The command \"tofu refresh\" is not yet supported under the experimental language runtime.",
	))
	op.ReportResult(runningOp, diags)
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
