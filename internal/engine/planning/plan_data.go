// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package planning

import (
	"context"
	"fmt"

	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/engine/internal/exec"
	"github.com/opentofu/opentofu/internal/lang/eval"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/plans/objchange"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func (p *planGlue) planDesiredDataResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance) (*resourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	tracer := contextTracer(ctx)
	if cb := tracer.StartDataResourceInstancePlanning; cb != nil {
		ctx = cb(ctx, inst.Addr)
	}
	if cb := tracer.EndDataResourceInstancePlanning; cb != nil {
		defer func() { // closure to delay evaluating diags until we return
			cb(ctx, inst.Addr, diags)
		}()
	}

	configMeta := p.oracle.ResourceInstanceObjectMeta(ctx, inst.Addr.CurrentObject())
	if configMeta == nil {
		// Should not happen: the evaluator is required to always produce
		// non-nil metadata for a desired object.
		panic(fmt.Sprintf("no metadata available for desired object %s", inst.Addr))
	}
	// Data resource instances don't have meaningful prior state (we store it
	// only for ancillary uses like the "tofu console" command) and so the
	// metadata is always exclusively from the configuration.
	meta := exec.BuildResourceInstanceObjectMeta(inst.Addr.CurrentObject(), configMeta, (*states.ResourceInstanceObjectFullSrc)(nil))

	ret := &resourceInstanceObject{
		Addr:               inst.Addr.CurrentObject(),
		PrevRoundAddr:      inst.Addr.CurrentObject(),
		ConfigDependencies: addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:           meta.Provider,

		// We'll start off with a completely-unknown placeholder value, but
		// we might refine this to be more specific as we learn more below.
		PlaceholderValue: cty.DynamicVal,

		// NOTE: PlannedChange remains nil until we actually produce a plan,
		// so early returns with errors are not guaranteed to have a valid
		// change object. Evaluation falls back on using PlaceholderValue
		// when no planned change is present.
	}
	for dep := range inst.RequiredResourceInstances.All() {
		ret.ConfigDependencies.Add(dep.CurrentObject())
	}

	unmarkedConfigVal, _ := inst.ConfigVal.UnmarkDeep()

	/// DataResourceType) ValidateConfig
	// TODO resourceType.ValidateConfig
	validateDiags := p.planCtx.providers.ValidateResourceConfig(ctx, meta.Provider, addrs.DataResourceMode, meta.ResourceType, unmarkedConfigVal)
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	providerInstUnmarked, providerInstMarks := meta.ProviderInstance.Unmark()
	// TODO: What should we do with these marks, if anything?
	_ = providerInstMarks
	providerInstAddr, ok := providerInstUnmarked.ValueOk()
	if !ok {
		// TODO: Record that this was deferred because we don't yet know which
		// provider instance it belongs to.
		return ret, diags
	}

	providerClient, moreDiags := p.providerClient(ctx, providerInstAddr)
	if providerClient == nil {
		moreDiags = moreDiags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Provider instance not available",
			fmt.Sprintf("Cannot plan %s because its associated provider instance %s cannot initialize.", inst.Addr, providerInstAddr),
			nil,
		))
	}
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return ret, diags
	}

	resourceType := resources.NewDataResourceType(meta.Provider, inst.Addr.Resource.Resource.Type, providerClient)

	// The equivalent of "refreshing" a data resource is just to discard it
	// completely, because we only retain the previous result in state snapshots
	// to support unusual situations like "tofu console"; it's not expected that
	// data resource instances persist between rounds and they cannot because
	// the protocol doesn't include any way to "upgrade" them if the provider
	// schema has changed since previous round.
	// FIXME: State is still using the weird old representation of provider
	// instance addresses, so we can't actually populate the provider instance
	// arguments properly here.
	p.planCtx.refreshedState.SetResourceInstanceCurrent(inst.Addr, nil, addrs.AbsProviderConfig{}, providerInstAddr.Key)

	readCtx := ctx
	if cb := tracer.StartDataResourceInstanceRead; cb != nil {
		readCtx = cb(ctx, inst.Addr)
	}
	requiredChanges := addrs.CollectSet(objchange.PrereqChangesForValue(inst.ConfigVal))
	depsPending := len(requiredChanges) != 0
	configKnown := inst.ConfigVal.IsWhollyKnown()
	if depsPending || !configKnown {
		var reason plans.ResourceInstanceChangeActionReason
		switch {
		case !configKnown:
			// log.Printf("[TRACE] planDataSource: %s configuration not fully known yet, so deferring to apply phase", n.Addr)
			reason = plans.ResourceInstanceReadBecauseConfigUnknown
		case depsPending:
			// NOTE: depsPending can be true at the same time as configKnown
			// is false; configKnown takes precedence because it's more
			// specific.
			// log.Printf("[TRACE] planDataSource: %s configuration is fully known, at least one dependency has changes pending", n.Addr)
			reason = plans.ResourceInstanceReadBecauseDependencyPending
		}

		// The configuration for this data resource instance is relying on
		// values that won't be finalized until the apply phase, so we'll need
		// to delay reading this until the apply phase.
		// Note that this is not "deferral" in the sense of "deferred actions":
		// that terminology refers to skipping any actions for a particular
		// resource instance _even in the apply phase_ of this round, whereas
		// "delaying" here just means that it gets read in the apply phase
		// instead of during the plan phase.
		//
		// TODO: We should also used [derivedFromDeferredVal] somewhere in this
		// function to handle when this is derived from something that _is_
		// being completely deferred in this round, in which case we must also
		// defer reading this data resource instance to a future round.
		ret, moreDiags := p.planDelayedDataResourceInstance(readCtx, inst, providerInstAddr, providerClient, reason, ret)
		diags = diags.Append(moreDiags)
		return ret, diags
	}

	// node_resource_abstract_instance.go:L2309

	validateDiags = resourceType.ValidateConfig(ctx, inst.ConfigVal)
	// FIXME Needs that InConfigBody thing, see resourceType.Read
	// for more details, that's gotta be fixed too.
	diags = diags.Append(validateDiags)
	if diags.HasErrors() {
		return ret, diags
	}

	// TODO run PreApply hook here

	resp, readDiags := resourceType.Read(ctx, &resources.DataResourceReadRequest{
		ResourceAddress: inst.Addr,
		ConfigValue:     inst.ConfigVal,
	}, inst.Addr.CurrentObject(), p.planCtx.evalCtx.Encryption)
	diags = diags.Append(readDiags)

	if cb := tracer.EndDataResourceInstanceRead; cb != nil {
		resultVal := cty.DynamicVal
		if resp.Result != cty.NilVal {
			// Note: resourceType.Read applies "sensitive" marks to Result
			resultVal = resp.Result
		}
		cb(readCtx, inst.Addr, resultVal, diags)
	}

	// TODO run PostApply hook here

	// obtain schema for encoding
	schema, schemaDiags := resourceType.LoadSchema(ctx)
	diags = diags.Append(schemaDiags)
	if schemaDiags.HasErrors() {
		return ret, diags
	}

	src, err := ctyjson.Marshal(resp.Result, schema.Block.ImpliedType())
	if err != nil {
		// We just checked for type conformance in the Read, so getting into this
		// codepath is probably a bug.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to encode result of data resource read",
			fmt.Sprintf("Failed to encode state for %s after data resource read: %s.", inst.Addr, tfdiags.FormatError(err)),
		))
	}

	dataResourceState := &states.ResourceInstanceObjectFullSrc{
		Value: states.ValueJSONWithMetadata{
			ValueJSON:      src,
			SensitivePaths: resp.SensitivePaths,
		},
		Status:               resp.Status,
		ProviderInstanceAddr: providerInstAddr,
		ResourceType:         configMeta.ResourceType,
		SchemaVersion:        uint64(schema.Version),
		// TODO derive this from inst.ConfigVal.... somehow...
		// Dependencies:         resp.Dependencies,
	}

	p.planCtx.refreshedState.SetResourceInstanceObjectFull(inst.Addr.CurrentObject(), dataResourceState)
	p.planCtx.upgradedState.SetResourceInstanceObjectFull(inst.Addr.CurrentObject(), dataResourceState)

	// Since we've already read the data source during the planning phase,
	// we don't need a PlannedChange here and can instead just use the result
	// as the PlaceholderValue.
	ret.PlaceholderValue = resp.Result

	return ret, diags
}

// planDelayedDataResourceInstance deals with the situation where a data
// resource instance has a configuration that includes values that won't be
// finalized and known until the apply phase.
//
// In that case we produce a planned action to read the resource instance during
// the apply phase, and then use a marked placeholder for ongoing evaluation.
//
// This is called by [planDesiredDataResourceInstance] after it has already
// partially-constructed the [resourceInstanceObject] to return, so that's
// passed in as "ret" and then modified in-place before returning it. The
// caller is expected to then just return that result verbatim.
func (p *planGlue) planDelayedDataResourceInstance(ctx context.Context, inst *eval.DesiredResourceInstance, providerAddr addrs.AbsProviderInstanceCorrect, providerClient providers.Interface, reason plans.ResourceInstanceChangeActionReason, ret *resourceInstanceObject) (*resourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// TODO: Check if we're doing refresh-only, and skip accordingly

	resourceType := resources.NewDataResourceType(providerAddr.Config.Config.Provider, inst.Addr.Resource.Resource.Type, providerClient)

	schema, schemaDiags := resourceType.LoadSchema(ctx)
	if schemaDiags.HasErrors() {
		// We don't return the schema-loading diagnostics directly here because
		// they should have already been returned by earlier code, but we do
		// return a more specific error to make it clear that this specific
		// resource instance was unplannable because of the problem.
		diags = diags.Append(tfdiags.AttributeValue(
			tfdiags.Error,
			"Resource type schema unavailable",
			fmt.Sprintf(
				"Cannot plan %s because provider %s failed to return the schema for its resource type %q.",
				inst.Addr, providerAddr.Config.Config.Provider, inst.Addr.Resource.Resource.Type,
			),
			nil, // this error belongs to the whole resource config
		))
		return ret, diags
	}

	unmarkedConfigVal, configMarkPaths := inst.ConfigVal.UnmarkDeepWithPaths()
	proposedNewVal := objchange.PlannedUnknownObject(schema.Block, unmarkedConfigVal)
	proposedNewVal = proposedNewVal.MarkWithPaths(configMarkPaths)

	// Apply detects that the data source will need to be read by the After
	// value containing unknowns from PlanDataResourceObject.
	ret.PlannedChange = &plans.ResourceInstanceChange{
		Addr:         inst.Addr,
		PrevRunAddr:  inst.Addr,
		ProviderAddr: providerAddr.Config.Module.ProviderConfigDefault(providerAddr.Config.Config.Provider),
		Action:       plans.Read,
		Before:       cty.NullVal(schema.Block.ImpliedType()),
		After:        proposedNewVal,
		ActionReason: reason,
	}
	ret.ProviderInst = providerAddr

	// TODO post-diff hook

	return ret, diags

	// Still TODO:
	// The placeholder value for any computed attribute in
	// the object we return should also be annotated with
	// [objchange.ValuePendingChange] using this data resource instance's
	// address, so that any downstream data resource instance that derives
	// from the results of this one will also get delayed to the apply
	// phase.

	// TODO: It would be nice to also report the requiredChanges set in a way
	// that would allow us to enumerate in the UI exactly which managed
	// resource instances are blocking the reading of this data resource
	// instance, but that's less important than making sure the generated
	// execution graph respects those dependencies.
	//
	// When we do this note that there can be unknown values in the config
	// even when there aren't any required changes, such as if for some
	// reason the data resource configuration includes a call to an
	// impure function like "timestamp", so we should make sure the UI still
	// does something sensible when requiredChanges is empty.
}

func (p *planGlue) planOrphanDataResourceInstance(_ context.Context, addr addrs.AbsResourceInstance, state *states.ResourceInstanceObjectFullSrc) (*resourceInstanceObject, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	return &resourceInstanceObject{
		Addr:               addr.CurrentObject(),
		PrevRoundAddr:      addr.CurrentObject(),
		ConfigDependencies: addrs.MakeSet[addrs.AbsResourceInstanceObject](),
		Provider:           state.ProviderInstanceAddr.Config.Config.Provider,
		PlaceholderValue:   cty.NullVal(cty.DynamicPseudoType),
		// An orphan data object is always just discarded completely, because
		// OpenTofu retains them only for esoteric uses like the "tofu console"
		// command: they are not actually expected to persist between rounds.
		// Therefore upgrade and refresh state are not set
	}, diags
}
