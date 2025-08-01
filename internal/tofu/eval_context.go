// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/checks"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/provisioners"
	"github.com/opentofu/opentofu/internal/refactoring"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// EvalContext is the interface that is given to eval nodes to execute.
type EvalContext interface {
	// Stopped returns a channel that is closed when evaluation is stopped
	// via tofu.Context.Stop()
	Stopped() <-chan struct{}

	// Path is the current module path.
	Path() addrs.ModuleInstance

	// Hook is used to call hook methods. The callback is called for each
	// hook and should return the hook action to take and the error.
	Hook(func(Hook) (HookAction, error)) error

	// Input is the UIInput object for interacting with the UI.
	Input() UIInput

	// InitProvider initializes the provider with the given address, and returns
	// the implementation of the resource provider or an error.
	//
	// It is an error to initialize the same provider more than once. This
	// method will panic if the module instance address of the given provider
	// configuration does not match the Path() of the EvalContext.
	InitProvider(ctx context.Context, addr addrs.AbsProviderConfig, key addrs.InstanceKey) (providers.Interface, error)

	// Provider gets the provider instance with the given address (already
	// initialized) or returns nil if the provider isn't initialized.
	//
	// This method expects an _absolute_ provider configuration address, since
	// resources in one module are able to use providers from other modules.
	// InitProvider must've been called on the EvalContext of the module
	// that owns the given provider before calling this method.
	Provider(context.Context, addrs.AbsProviderConfig, addrs.InstanceKey) providers.Interface

	// ProviderSchema retrieves the schema for a particular provider, which
	// must have already been initialized with InitProvider.
	//
	// This method expects an _absolute_ provider configuration address, since
	// resources in one module are able to use providers from other modules.
	ProviderSchema(context.Context, addrs.AbsProviderConfig) (providers.ProviderSchema, error)

	// CloseProvider closes provider connections that aren't needed anymore.
	//
	// This method will panic if the module instance address of the given
	// provider configuration does not match the Path() of the EvalContext.
	CloseProvider(context.Context, addrs.AbsProviderConfig) error

	// ConfigureProvider configures the provider with the given
	// configuration. This is a separate context call because this call
	// is used to store the provider configuration for inheritance lookups
	// with ParentProviderConfig().
	//
	// This method will panic if the module instance address of the given
	// provider configuration does not match the Path() of the EvalContext.
	ConfigureProvider(context.Context, addrs.AbsProviderConfig, addrs.InstanceKey, cty.Value) tfdiags.Diagnostics

	// ProviderInput and SetProviderInput are used to configure providers
	// from user input.
	//
	// These methods will panic if the module instance address of the given
	// provider configuration does not match the Path() of the EvalContext.
	ProviderInput(context.Context, addrs.AbsProviderConfig) map[string]cty.Value
	SetProviderInput(context.Context, addrs.AbsProviderConfig, map[string]cty.Value)

	// Provisioner gets the provisioner instance with the given name.
	Provisioner(string) (provisioners.Interface, error)

	// ProvisionerSchema retrieves the main configuration schema for a
	// particular provisioner, which must have already been initialized with
	// InitProvisioner.
	ProvisionerSchema(string) (*configschema.Block, error)

	// CloseProvisioner closes all provisioner plugins.
	CloseProvisioners() error

	// EvaluateBlock takes the given raw configuration block and associated
	// schema and evaluates it to produce a value of an object type that
	// conforms to the implied type of the schema.
	//
	// The "self" argument is optional. If given, it is the referenceable
	// address that the name "self" should behave as an alias for when
	// evaluating. Set this to nil if the "self" object should not be available.
	//
	// The "key" argument is also optional. If given, it is the instance key
	// of the current object within the multi-instance container it belongs
	// to. For example, on a resource block with "count" set this should be
	// set to a different addrs.IntKey for each instance created from that
	// block. Set this to addrs.NoKey if not appropriate.
	//
	// The returned body is an expanded version of the given body, with any
	// "dynamic" blocks replaced with zero or more static blocks. This can be
	// used to extract correct source location information about attributes of
	// the returned object value.
	EvaluateBlock(ctx context.Context, body hcl.Body, schema *configschema.Block, self addrs.Referenceable, keyData InstanceKeyEvalData) (cty.Value, hcl.Body, tfdiags.Diagnostics)

	// EvaluateExpr takes the given HCL expression and evaluates it to produce
	// a value.
	//
	// The "self" argument is optional. If given, it is the referenceable
	// address that the name "self" should behave as an alias for when
	// evaluating. Set this to nil if the "self" object should not be available.
	EvaluateExpr(ctx context.Context, expr hcl.Expression, wantType cty.Type, self addrs.Referenceable) (cty.Value, tfdiags.Diagnostics)

	// EvaluateReplaceTriggeredBy takes the raw reference expression from the
	// config, and returns the evaluated *addrs.Reference along with a boolean
	// indicating if that reference forces replacement.
	EvaluateReplaceTriggeredBy(ctx context.Context, expr hcl.Expression, repData instances.RepetitionData) (*addrs.Reference, bool, tfdiags.Diagnostics)

	// EvaluationScope returns a scope that can be used to evaluate reference
	// addresses in this context.
	EvaluationScope(self addrs.Referenceable, source addrs.Referenceable, keyData InstanceKeyEvalData) *lang.Scope

	// SetRootModuleArgument defines the value for one variable of the root
	// module. The caller must ensure that given value is a suitable
	// "final value" for the variable, which means that it's already converted
	// and validated to match any configured constraints and validation rules.
	//
	// Calling this function multiple times with the same variable address
	// will silently overwrite the value provided by a previous call.
	SetRootModuleArgument(addrs.InputVariable, cty.Value)

	// SetModuleCallArgument defines the value for one input variable of a
	// particular child module call. The caller must ensure that the given
	// value is a suitable "final value" for the variable, which means that
	// it's already converted and validated to match any configured
	// constraints and validation rules.
	//
	// Calling this function multiple times with the same variable address
	// will silently overwrite the value provided by a previous call.
	SetModuleCallArgument(addrs.ModuleCallInstance, addrs.InputVariable, cty.Value)

	// GetVariableValue returns the value provided for the input variable with
	// the given address, or cty.DynamicVal if the variable hasn't been assigned
	// a value yet.
	//
	// Most callers should deal with variable values only indirectly via
	// EvaluationScope and the other expression evaluation functions, but
	// this is provided because variables tend to be evaluated outside of
	// the context of the module they belong to and so we sometimes need to
	// override the normal expression evaluation behavior.
	GetVariableValue(addr addrs.AbsInputVariableInstance) cty.Value

	// Changes returns the writer object that can be used to write new proposed
	// changes into the global changes set.
	Changes() *plans.ChangesSync

	// State returns a wrapper object that provides safe concurrent access to
	// the global state.
	State() *states.SyncState

	// Checks returns the object that tracks the state of any custom checks
	// declared in the configuration.
	Checks() *checks.State

	// RefreshState returns a wrapper object that provides safe concurrent
	// access to the state used to store the most recently refreshed resource
	// values.
	RefreshState() *states.SyncState

	// PrevRunState returns a wrapper object that provides safe concurrent
	// access to the state which represents the result of the previous run,
	// updated only so that object data conforms to current schemas for
	// meaningful comparison with RefreshState.
	PrevRunState() *states.SyncState

	// InstanceExpander returns a helper object for tracking the expansion of
	// graph nodes during the plan phase in response to "count" and "for_each"
	// arguments.
	//
	// The InstanceExpander is a global object that is shared across all of the
	// EvalContext objects for a given configuration.
	InstanceExpander() *instances.Expander

	// MoveResults returns a map describing the results of handling any
	// resource instance move statements prior to the graph walk, so that
	// the graph walk can then record that information appropriately in other
	// artifacts produced by the graph walk.
	//
	// This data structure is created prior to the graph walk and read-only
	// thereafter, so callers must not modify the returned map or any other
	// objects accessible through it.
	MoveResults() refactoring.MoveResults

	// ImportResolver returns a helper object for tracking the resolution of
	// imports, after evaluating the dynamic address and ID of the import targets
	//
	// This data is created during the graph walk, as import target addresses are being resolved
	// Its primary use is for validation at the end of a plan - To make sure all imports have been satisfied
	// and have a configuration
	ImportResolver() *ImportResolver

	// WithPath returns a copy of the context with the internal path set to the
	// path argument.
	WithPath(path addrs.ModuleInstance) EvalContext

	// Returns the currently configured encryption setup
	GetEncryption() encryption.Encryption
}
