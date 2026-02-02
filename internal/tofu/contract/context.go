package contract

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/opentofu/opentofu/internal/tofu/hooks"
	"github.com/opentofu/opentofu/internal/tofu/importing"
	"github.com/opentofu/opentofu/internal/tofu/variables"
)

// ContextOpts are the user-configurable options to create a context with
// NewContext.
type ContextOpts struct {
	Meta         *ContextMeta
	Hooks        []hooks.Hook
	Parallelism  int
	Providers    map[addrs.Provider]providers.Factory
	Provisioners map[string]provisioners.Factory
	Encryption   encryption.Encryption
}

// ContextMeta is metadata about the running context. This is information
// that this package or structure cannot determine on its own but exposes
// into OpenTofu in various ways. This must be provided by the Context
// initializer.
type ContextMeta struct {
	Env string // Env is the state environment

	// OriginalWorkingDir is the working directory where the OpenTofu CLI
	// was run from, which may no longer actually be the current working
	// directory if the user included the -chdir=... option.
	//
	// If this string is empty then the original working directory is the same
	// as the current working directory.
	//
	// In most cases we should respect the user's override by ignoring this
	// path and just using the current working directory, but this is here
	// for some exceptional cases where the original working directory is
	// needed.
	OriginalWorkingDir string
}

type Context interface {
	Validate(ctx context.Context, config *configs.Config, varValues variables.InputValues, importTargets []*importing.ImportTarget) tfdiags.Diagnostics
	Plan(ctx context.Context, config *configs.Config, prevRunState *states.State, moveStmts []refactoring.MoveStatement, moveResults refactoring.MoveResults, opts *PlanOpts) (*plans.Plan, tfdiags.Diagnostics)
	Apply(ctx context.Context, plan *plans.Plan, config *configs.Config, setVariables variables.InputValues) (*states.State, tfdiags.Diagnostics)
	Import(ctx context.Context, config *configs.Config, prevRunState *states.State, opts *ImportOpts) (*states.State, tfdiags.Diagnostics)
	Eval(ctx context.Context, config *configs.Config, state *states.State, moduleAddr addrs.ModuleInstance, variables variables.InputValues) (*lang.Scope, tfdiags.Diagnostics)
	Stop()
}

// PlanOpts are the various options that affect the details of how OpenTofu
// will build a plan.
type PlanOpts struct {
	// Mode defines what variety of plan the caller wishes to create.
	// Refer to the documentation of the plans.Mode type and its values
	// for more information.
	Mode plans.Mode

	// SkipRefresh specifies to trust that the current values for managed
	// resource instances in the prior state are accurate and to therefore
	// disable the usual step of fetching updated values for each resource
	// instance using its corresponding provider.
	SkipRefresh bool

	// PreDestroyRefresh indicated that this is being passed to a plan used to
	// refresh the state immediately before a destroy plan.
	// FIXME: This is a temporary fix to allow the pre-destroy refresh to
	// succeed. The refreshing operation during destroy must be a special case,
	// which can allow for missing instances in the state, and avoid blocking
	// on failing condition tests. The destroy plan itself should be
	// responsible for this special case of refreshing, and the separate
	// pre-destroy plan removed entirely.
	PreDestroyRefresh bool

	// SetVariables are the raw values for root module variables as provided
	// by the user who is requesting the run, prior to any normalization or
	// substitution of defaults. See the documentation for the InputValue
	// type for more information on how to correctly populate this.
	SetVariables variables.InputValues

	// If Targets has a non-zero length then it activates targeted planning
	// mode, where OpenTofu will take actions only for resource instances
	// mentioned in this set and any other objects those resource instances
	// depend on.
	//
	// Targeted planning mode is intended for exceptional use only,
	// and so populating this field will cause OpenTofu to generate extra
	// warnings as part of the planning result.
	Targets []addrs.Targetable

	// If Excludes has a non-zero length then it activates targeted planning
	// mode, where OpenTofu will take actions only for resource instances
	// that are not mentioned in this set and are not dependent on targets
	// mentioned in this set.
	//
	// Targeted planning mode is intended for exceptional use only,
	// and so populating this field will cause OpenTofu to generate extra
	// warnings as part of the planning result.
	Excludes []addrs.Targetable

	// ForceReplace is a set of resource instance addresses whose corresponding
	// objects should be forced planned for replacement if the provider's
	// plan would otherwise have been to either update the object in-place or
	// to take no action on it at all.
	//
	// A typical use of this argument is to ask OpenTofu to replace an object
	// which the user has determined is somehow degraded (via information from
	// outside of OpenTofu), thereby hopefully replacing it with a
	// fully-functional new object.
	ForceReplace []addrs.AbsResourceInstance

	// ExternalReferences allows the external caller to pass in references to
	// nodes that should not be pruned even if they are not referenced within
	// the actual graph.
	ExternalReferences []*addrs.Reference

	// ImportTargets is a list of target resources to import. These resources
	// will be added to the plan graph.
	ImportTargets []*importing.ImportTarget

	// RemoveStatements are the list of resources and modules to forget from
	// the state.
	RemoveStatements []*refactoring.RemoveStatement

	// GenerateConfig tells OpenTofu where to write any generated configuration
	// for any ImportTargets that do not have configuration already.
	//
	// If empty, then no config will be generated.
	GenerateConfigPath string
}

// ImportOpts are used as the configuration for Import.
type ImportOpts struct {
	// Targets are the targets to import
	Targets []*importing.ImportTarget

	// SetVariables are the variables set outside of the configuration,
	// such as on the command line, in variables files, etc.
	SetVariables variables.InputValues
}
