// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package exec

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"

	"github.com/zclconf/go-cty/cty"
)

// Operations represents the full set of operations that can be used in an
// execution graph, and so implementations of this interface are used to
// actually perform those operations.
//
// This interface essentially acts as "glue" between the execution graph and
// the broader environment where it's being used: the library of available
// provider plugins, the configuration evaluator, etc. The methods in this
// interface directly correspond to the supported execution graph opcodes except
// where stated otherwise in the comments associated with each method.
//
// The design intention is that an implementer of this interface would have
// access to information that comes from outside of the graph and would be able
// to record any state updates that occur but the implementation SHOULD NOT need
// to retain information provided to one method call for direct use by another
// later method call to the same object. Any temporary data should be propagated
// between calls by the caller of this interface, which is typically the
// execution graph processor. If you're implementing a new feature that requires
// additional data to be propagated then you should arrange for it to propagate
// through method arguments and return values rather than keeping sidecar data
// inside your [Operations] implementation.
//
// If you're writing a unit test then try to test at a tighter granularity
// than using this entire interface. Writing a mock implementation of this
// entire interface should be a last resort because that'd add additional
// burden to maintaining this interface over time as our needs for apply-time
// execution grow and change.
type Operations interface {
	//////////////////////////////////////////////////////////////////////////////
	//// Provider-related operations
	//////////////////////////////////////////////////////////////////////////////

	// ProviderInstanceConfig determines what configuration value should be used
	// to configure the provider instance at the given address.
	//
	// Real implementations of this use the configuration evaluator to
	// finalize the provider configuration based on other values that have been
	// previously resolved. A valid execution graph ensures that this method is
	// not called until all of the required upstream values are available.
	ProviderInstanceConfig(
		ctx context.Context,
		instAddr addrs.AbsProviderInstanceCorrect,
	) (*ProviderInstanceConfig, tfdiags.Diagnostics)

	// ProviderInstanceOpen attempts to launch and configure a provider plugin
	// using the given configuration.
	ProviderInstanceOpen(
		ctx context.Context,
		config *ProviderInstanceConfig,
	) (*ProviderClient, tfdiags.Diagnostics)

	// ProviderInstanceClose shuts down a previously-opened provider plugin.
	//
	// A valid execution graph ensures that this is called only after all other
	// operations using the given client have either completed or have been
	// cancelled due to an upstream error, and so implementers can assume the
	// client is not currently being used elsewhere and will not be used again
	// after this method returns.
	ProviderInstanceClose(
		ctx context.Context,
		client *ProviderClient,
	) tfdiags.Diagnostics

	//////////////////////////////////////////////////////////////////////////////
	//// Resource-related operations that are relevant to multiple resource modes.
	//// (mode-specific operations follow below)
	//////////////////////////////////////////////////////////////////////////////

	// ResourceInstanceDesired returns a representation of the "desired state"
	// for the given resource instance, or a nil pointer if the given resource
	// instance is not currently declared at all.
	//
	// Real implementations of this use the configuration evaluator to finalize
	// the resource instance configuration based on other values that have been
	// previously resolved. A valid execution graph ensures that this method is
	// not called until all of the required upstream values are available.
	//
	// This operation is the only one that should return any diagnostics the
	// evaluator returns when producing the [DesiredResourceInstance] object,
	// which should include evaluating any preconditions declared for that
	// resource instance.
	ResourceInstanceDesired(
		ctx context.Context,
		instAddr addrs.AbsResourceInstance,
	) (*eval.DesiredResourceInstance, tfdiags.Diagnostics)

	// ResourceInstancePrior returns a representation of the "prior state" for
	// the given resource instance, or a nil pointer if there was no current
	// object bound to that resource instance address in the prior state.
	//
	// Real implementations of this use the prior state snapshot that was saved
	// as part of the plan that is being applied. That snapshot should already
	// conform to the current version of the schema for its resource type in
	// the associated provider due to having potentially been "upgraded" during
	// the planning phase.
	ResourceInstancePrior(
		ctx context.Context,
		instAddr addrs.AbsResourceInstance,
	) (*ResourceInstanceObject, tfdiags.Diagnostics)

	// ResourceInstancePostconditions tests whether the given object passes
	// any postconditions that were declared for it.
	//
	// This is not a real execution graph operation. Instead, execution graph
	// processing automatically makes calls to this as part of handling the
	// results from [Operations.ManagedApply], [Operations.ReadData], and
	// [Operations.OpenEphemeral] to ensure that postconditions always get
	// handled consistently for all resource modes.
	ResourceInstancePostconditions(
		ctx context.Context,
		result *ResourceInstanceObject,
	) tfdiags.Diagnostics

	//////////////////////////////////////////////////////////////////////////////
	/// Resource-related operations that are relevant only for managed resources.
	//////////////////////////////////////////////////////////////////////////////

	// ManagedFinalPlan uses the given provider client to create the final
	// plan for a change to a managed resource instance object, and then
	// verifies that its result value conforms to what the provider promised
	// during planning, which is given as "plannedVal".
	//
	// "desired" is nil if the expected operation is to delete the object
	// described in "prior". Conversely, "prior" is nil if the expected
	// operation is to create a new object matching "desired". At least one
	// of those arguments is always non-nil, and them both being set represents
	// planning an in-place update to the object.
	//
	// This method must always either return a valid, non-nil final plan object
	// or must return at least one error diagnostic.
	ManagedFinalPlan(
		ctx context.Context,
		desired *eval.DesiredResourceInstance,
		prior *ResourceInstanceObject,
		plannedVal cty.Value,
		providerClient *ProviderClient,
	) (*ManagedResourceObjectFinalPlan, tfdiags.Diagnostics)

	// ManagedApply uses the given provider client to apply the given plan.
	//
	// This operation MUST fully encapsulate all of the externally-visible
	// changes needed to apply a change such that when it returns -- whether
	// successfully or unsuccessfully -- the state has been left in a form that
	// accurately models whatever shape the remote system was left in, including
	// possibly saving a partially-created object returned by a provider so that
	// a future round can plan to attempt to repair it based on updated
	// configuration.
	//
	// If applying the plan fails in a way that causes there to be no new object
	// state to save, and if the "fallback" argument has a non-nil value, then
	// the fallback object (which is always a deposed object) should be
	// reinterpreted as the new current object for the resource instance. This
	// occurs when performing a "create then destroy" replace operation, so
	// that a total failure of the "create" step leaves OpenTofu still tracking
	// the previous object (which was presumably deposed earlier in the same
	// apply phase using ManagedDepose) as the current object.
	//
	// This method must return whatever object was left as "current" in the
	// state, including possibly returning the "current-ized" version of
	// the fallback object when appropriate, or nil if this was a destroy
	// operation that succeeded. When used with the real apply engine the
	// result is propagated back into the configuration evaluator so that
	// downstream resource and provider configurations can make use of the
	// results in their own final plans.
	//
	// Execution graph processing automatically passes the result of this
	// function to [Operations.ResourceInstancePostconditions] when appropriate,
	// propagating any additional diagnostics it returns, and so implementers of
	// this method should not attempt to handle postconditions themselves.
	ManagedApply(
		ctx context.Context,
		plan *ManagedResourceObjectFinalPlan,
		fallback *ResourceInstanceObject,
		providerClient *ProviderClient,
	) (*ResourceInstanceObject, tfdiags.Diagnostics)

	// ManagedDepose transforms the "current" object associated with the given
	// resource instance address into a "deposed" object for the same resource
	// instance, and then returns the description of the now-deposed object.
	//
	// If there is no current object associated with that resource instance,
	// this returns nil without changing anything.
	//
	// When using this as part of a "create then destroy" replace operation,
	// a correct execution graph arranges for the result to be propagated into
	// the "fallback" argument of a subsequent [Operations.ManagedApply] call,
	// so that the deposed object can be restored back to current if the
	// apply operation fails to the extent that no new object is created at all.
	ManagedDepose(
		ctx context.Context,
		instAddr addrs.AbsResourceInstance,
	) (*ResourceInstanceObject, tfdiags.Diagnostics)

	// ManagedAlreadyDeposed returns a deposed object from the prior state,
	// nor nil if there is no such object.
	//
	// This deals with the relatively-uncommon situation where there was already
	// a deposed object present in the state at the beginning of the planning
	// phase, and that object did not get removed as a result of refreshing it.
	// That occurs only when a previous plan/apply round encountered an error
	// partway through a "create then destroy" replace operation where both
	// the newly-created object and the previously-existing object still exist.
	//
	// [Operations.ManagedDepose] deals with the more common case where a
	// previously-"current" object becomes deposed during the apply phase as
	// part of handling a "create then destroy' replace operation.
	ManagedAlreadyDeposed(
		ctx context.Context,
		instAddr addrs.AbsResourceInstance,
		deposedKey states.DeposedKey,
	) (*ResourceInstanceObject, tfdiags.Diagnostics)

	// ManageChangeAddr rebinds the current object associated with
	// currentInstAddr to be associated with newInstAddr instead, and then
	// returns that object with its updated address.
	//
	// This is used in place of [Operations.ResourceInstancePrior] whenever a
	// resource instance address is being moved to a new address. The move
	// and the read from the state are combined into a single action so that
	// we can treat this as an atomic operation where there's no intermediate
	// state where the relevant object is associated with either neither or both
	// of the two addresses.
	//
	// If there is no current object associated with currentInstAddr when
	// this operation executes then it does nothing and returns a nil object
	// with no errors, though in practice the planning engine should not include
	// this operation unless it found an existing object that needed to be
	// moved.
	ManagedChangeAddr(
		ctx context.Context,
		currentInstAddr, newInstAddr addrs.AbsResourceInstance,
	) (*ResourceInstanceObject, tfdiags.Diagnostics)

	//////////////////////////////////////////////////////////////////////////////
	/// Resource-related operations that are relevant only for data resources.
	//////////////////////////////////////////////////////////////////////////////

	// DataRead uses the given provider client to read the latest value for a
	// desired data resource instance.
	//
	// This method always returns a non-nil object unless it returns at least
	// one error diagnostic explaining why it cannot. The result should also
	// be saved to the updated state before this method returns.
	//
	// This operation is used only when it isn't possible to read the data
	// resource value during the planning phase. If the desired resource
	// instance was already known enough to read it during the plan phase then
	// the prior state would already record its result and an so a call
	// to [Operations.ResourceInstancePrior] is sufficient to obtain the value.
	//
	// Execution graph processing automatically passes the result of this
	// function to [Operations.ResourceInstancePostconditions] when appropriate,
	// propagating any additional diagnostics it returns, and so implementers of
	// this method should not attempt to handle postconditions themselves.
	DataRead(
		ctx context.Context,
		desired *eval.DesiredResourceInstance,
		plannedVal cty.Value,
		providerClient *ProviderClient,
	) (*ResourceInstanceObject, tfdiags.Diagnostics)

	//////////////////////////////////////////////////////////////////////////////
	/// Resource-related operations that are relevant only for ephemeral resources.
	//////////////////////////////////////////////////////////////////////////////

	// EphemeralOpen uses the given provider client to "open" the given
	// ephemeral resource instance, making it ready for indirect use by
	// subsequent operations that rely on its results.
	//
	// If the provider requires periodic "renewal" of the ephemeral object
	// then the implementer of this method must arrange for that to happen
	// until either the corresponding call to [EphemeralClose] or until
	// execution has completed without such a call, typically due to an error
	// having occurred along the way. Renewal is considered an implementation
	// detail of whatever is managing a provider's operation, with the execution
	// graph just assuming that ephemeral objects remain valid _somehow_ for
	// the full duration of their use.
	EphemeralOpen(
		ctx context.Context,
		desired *eval.DesiredResourceInstance,
		providerClient *ProviderClient,
	) (*OpenEphemeralResourceInstance, tfdiags.Diagnostics)

	// EphemeralState refines the open ephemeral resource instance into the
	// required resource object state
	//
	// Execution graph processing automatically passes the result of this
	// function to [Operations.ResourceInstancePostconditions] when appropriate,
	// propagating any additional diagnostics it returns, and so implementers of
	// this method should not attempt to handle postconditions themselves.
	EphemeralState(
		ctx context.Context,
		ephemeral *OpenEphemeralResourceInstance,
	) (*ResourceInstanceObject, tfdiags.Diagnostics)

	// EphemeralClose calls Close on the open ephemeral resource instance
	//
	// A valid execution graph ensures that this is called only after all other
	// operations using the given object have either completed or have been
	// cancelled due to an upstream error, and so implementers can assume the
	// object is not currently being used elsewhere and will not be used again
	// after this method returns.
	EphemeralClose(
		ctx context.Context,
		ephemeral *OpenEphemeralResourceInstance,
	) tfdiags.Diagnostics
}
