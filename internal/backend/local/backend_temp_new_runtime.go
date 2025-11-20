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
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configload"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/engine/planning"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
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

	plugins := &newRuntimePlugins{
		// TODO: ...
	}
	evalCtx := &eval.EvalContext{
		RootModuleDir:      op.ConfigDir,
		OriginalWorkingDir: b.ContextOpts.Meta.OriginalWorkingDir,
		Modules: &newRuntimeModules{
			loader: op.ConfigLoader,
		},
		Providers:    plugins,
		Provisioners: plugins,
	}

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

	plan, moreDiags := planning.PlanChanges(ctx, prevRoundState, configInst)
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
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Saved plan files not supported",
			"The experimental language runtime cannot yet support -out=PLANFILE.",
		))
		op.ReportResult(runningOp, diags)
		return
	}

	// TODO: Actually render the plan. But to do that we need provider schemas
	// and our schema-loading code expects us to be holding an old-style
	// *configs.Config, so we have some more work to do before we can do that.

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
	log.Println("[WARN] Using apply implementation from the experimental language runtime")
	var diags tfdiags.Diagnostics
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"Operation unsupported in experimental language runtime",
		"The command \"tofu apply\" is not yet supported under the experimental language runtime.",
	))
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
	diags = diags.Append(tfdiags.Sourceless(
		tfdiags.Error,
		"New runtime codepath can't load modules yet",
		fmt.Sprintf("Cannot load %q. The experimental codepath for the new language runtime can't actually load modules yet, making it actually rather useless.", source),
	))
	return nil, diags
}

type newRuntimePlugins struct {
}

var _ eval.Providers = (*newRuntimePlugins)(nil)
var _ eval.Provisioners = (*newRuntimePlugins)(nil)

// NewConfiguredProvider implements evalglue.Providers.
func (n *newRuntimePlugins) NewConfiguredProvider(ctx context.Context, provider addrs.Provider, configVal cty.Value) (providers.Configured, tfdiags.Diagnostics) {
	panic("unimplemented")
}

// ProviderConfigSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ProviderConfigSchema(ctx context.Context, provider addrs.Provider) (*providers.Schema, tfdiags.Diagnostics) {
	panic("unimplemented")
}

// ResourceTypeSchema implements evalglue.Providers.
func (n *newRuntimePlugins) ResourceTypeSchema(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string) (*providers.Schema, tfdiags.Diagnostics) {
	panic("unimplemented")
}

// ValidateProviderConfig implements evalglue.Providers.
func (n *newRuntimePlugins) ValidateProviderConfig(ctx context.Context, provider addrs.Provider, configVal cty.Value) tfdiags.Diagnostics {
	panic("unimplemented")
}

// ValidateResourceConfig implements evalglue.Providers.
func (n *newRuntimePlugins) ValidateResourceConfig(ctx context.Context, provider addrs.Provider, mode addrs.ResourceMode, typeName string, configVal cty.Value) tfdiags.Diagnostics {
	panic("unimplemented")
}

// ProvisionerConfigSchema implements evalglue.Provisioners.
func (n *newRuntimePlugins) ProvisionerConfigSchema(ctx context.Context, typeName string) (*configschema.Block, tfdiags.Diagnostics) {
	panic("unimplemented")
}
