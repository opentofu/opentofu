// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// stateTransformArgs is a struct for convenience that holds the parameters required for state transformations
type stateTransformArgs struct {
	// addr is the current/latest address of the resource
	addr addrs.AbsResourceInstance
	// prevAddr is the previous address of the resource
	prevAddr  addrs.AbsResourceInstance
	provider  providers.Interface
	objectSrc *states.ResourceInstanceObjectSrc
}

// providerStateTransform transforms the state given the current and the previous AbsResourceInstance, Provider and ResourceInstanceObjectSrc
// and returns a new state as cty.Value, new private state as []byte and diagnostics
type providerStateTransform func(args stateTransformArgs) (cty.Value, []byte, tfdiags.Diagnostics)

// upgradeResourceState will, if necessary, run the provider-defined upgrade
// logic against the given state object to make it compliant with the
// current schema version.
//
// If any errors occur during upgrade, error diagnostics are returned. In that
// case it is not safe to proceed with using the original state object.
func upgradeResourceState(args stateTransformArgs, currentSchema *configschema.Block, currentSchemaVersion uint64) (*states.ResourceInstanceObjectSrc, tfdiags.Diagnostics) {
	return transformResourceState(args, currentSchema, currentSchemaVersion, upgradeResourceStateTransform)
}

// upgradeResourceStateTransform is a providerStateTransform that upgrades the state via provider upgrade logic
func upgradeResourceStateTransform(args stateTransformArgs) (cty.Value, []byte, tfdiags.Diagnostics) {
	log.Printf("[TRACE] upgradeResourceStateTransform: address: %s", args.addr)
	req := providers.UpgradeResourceStateRequest{
		TypeName: args.addr.Resource.Resource.Type,

		// TODO: The internal schema version representations are all using
		// uint64 instead of int64, but unsigned integers aren't friendly
		// to all protobuf target languages so in practice we use int64
		// on the wire. In future we will change all of our internal
		// representations to int64 too.
		Version:         int64(args.objectSrc.SchemaVersion),
		RawStateFlatmap: args.objectSrc.AttrsFlat,
		RawStateJSON:    args.objectSrc.AttrsJSON,
	}

	resp := args.provider.UpgradeResourceState(req)
	diags := resp.Diagnostics
	if diags.HasErrors() {
		log.Printf("[TRACE] upgradeResourceStateTransform: failed - address: %s", args.addr)
		return cty.NilVal, nil, diags
	}

	return resp.UpgradedState, args.objectSrc.Private, diags
}

// moveResourceState moves the state from one type or provider to another
// this function runs the provider-defined logic for MoveResourceState and performs the additional validation defined int transformResourceState
// If any error occurs during the validation or state transformation, error diagnostics are returned.
// Otherwise, the new state object is returned.
func moveResourceState(params stateTransformArgs, currentSchema *configschema.Block, currentSchemaVersion uint64) (*states.ResourceInstanceObjectSrc, tfdiags.Diagnostics) {
	return transformResourceState(params, currentSchema, currentSchemaVersion, moveResourceStateTransform)
}

// moveResourceStateTransform is a providerStateTransform that moves the state via provider move logic
func moveResourceStateTransform(args stateTransformArgs) (cty.Value, []byte, tfdiags.Diagnostics) {
	log.Printf("[TRACE] moveResourceStateTransform: new address: %s, previous address: %s", args.addr, args.prevAddr)
	req := providers.MoveResourceStateRequest{
		SourceProviderAddress: args.prevAddr.Resource.Resource.ImpliedProvider(),
		SourceTypeName:        args.prevAddr.Resource.Resource.Type,
		SourceSchemaVersion:   args.objectSrc.SchemaVersion,
		SourceStateJSON:       args.objectSrc.AttrsJSON,
		SourceStateFlatmap:    args.objectSrc.AttrsFlat,
		SourcePrivate:         args.objectSrc.Private,
		TargetTypeName:        args.addr.Resource.Resource.Type,
	}
	resp := args.provider.MoveResourceState(req)
	diags := resp.Diagnostics
	if diags.HasErrors() {
		log.Printf("[TRACE] moveResourceStateTransform: failed - new address: %s, previous address: %s", args.addr, args.prevAddr)
		return cty.NilVal, nil, diags
	}
	return resp.TargetState, resp.TargetPrivate, diags
}

// transformResourceState transforms the state based on passed in providerStateTransform and returns new ResourceInstanceObjectSrc
// the function takes in the current and previous AbsResourceInstance, Provider, ResourceInstanceObjectSrc, current schema and current version
func transformResourceState(args stateTransformArgs, currentSchema *configschema.Block, currentSchemaVersion uint64, stateTransform providerStateTransform) (*states.ResourceInstanceObjectSrc, tfdiags.Diagnostics) {
	if args.addr.Resource.Resource.Mode != addrs.ManagedResourceMode {
		// We only do state upgrading for managed resources.
		// This was a part of the normal workflow in older versions and
		// returned early, so we are only going to log the error for now.
		log.Printf("[ERROR] data resource %s should not require state upgrade", args.addr)
		return args.objectSrc, nil
	}

	// Remove any attributes from state that are not present in the schema.
	// This was previously taken care of by the provider, but data sources do
	// not go through the UpgradeResourceState process.
	//
	// Legacy flatmap state is already taken care of during conversion.
	// If the schema version is be changed, then allow the provider to handle
	// removed attributes.
	if len(args.objectSrc.AttrsJSON) > 0 && args.objectSrc.SchemaVersion == currentSchemaVersion {
		args.objectSrc.AttrsJSON = stripRemovedStateAttributes(args.objectSrc.AttrsJSON, currentSchema.ImpliedType())
	}

	// TODO: This should eventually use a proper FQN.
	providerType := args.addr.Resource.Resource.ImpliedProvider()
	if args.objectSrc.SchemaVersion > currentSchemaVersion {
		log.Printf("[TRACE] transformResourceState: can't downgrade state for %s from version %d to %d", args.addr, args.objectSrc.SchemaVersion, currentSchemaVersion)
		var diags tfdiags.Diagnostics
		return nil, diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Resource instance managed by newer provider version",
			// This is not a very good error message, but we don't retain enough
			// information in state to give good feedback on what provider
			// version might be required here. :(
			fmt.Sprintf("The current state of %s was created by a newer provider version than is currently selected. Upgrade the %s provider to work with this state.", args.addr, providerType),
		))
	}

	// If we get down here then we need to upgrade the state, with the
	// provider's help.
	// If this state was originally created by a version of OpenTofu prior to
	// v0.12, this also includes translating from legacy flatmap to new-style
	// representation, since only the provider has enough information to
	// understand a flatmap built against an older schema.
	if args.objectSrc.SchemaVersion != currentSchemaVersion {
		log.Printf("[TRACE] transformResourceState: upgrading state for %s from version %d to %d using provider %q", args.addr, args.objectSrc.SchemaVersion, currentSchemaVersion, providerType)
	} else {
		log.Printf("[TRACE] transformResourceState: schema version of %s is still %d; calling provider %q for any other minor fixups", args.addr, currentSchemaVersion, providerType)
	}

	newState, newPrivate, diags := stateTransform(args)
	if diags.HasErrors() {
		return nil, diags
	}

	// After upgrading, the new value must conform to the current schema. When
	// going over RPC this is actually already ensured by the
	// marshaling/unmarshaling of the new value, but we'll check it here
	// anyway for robustness, e.g. for in-process providers.
	if errs := newState.Type().TestConformance(currentSchema.ImpliedType()); len(errs) > 0 {
		for _, err := range errs {
			diags = diags.Append(tfdiags.Sourceless(
				tfdiags.Error,
				"Invalid resource state upgrade",
				fmt.Sprintf("The %s provider upgraded the state for %s from a previous version, but produced an invalid result: %s.", providerType, args.addr, tfdiags.FormatError(err)),
			))
		}
		return nil, diags
	}
	newSrc, err := args.objectSrc.CompleteUpgrade(newState, currentSchema.ImpliedType(), currentSchemaVersion)
	if err != nil {
		// We already checked for type conformance above, so getting into this
		// codepath should be rare and is probably a bug somewhere under CompleteUpgrade.
		diags = diags.Append(tfdiags.Sourceless(
			tfdiags.Error,
			"Failed to encode result of resource state upgrade",
			fmt.Sprintf("Failed to encode state for %s after resource schema upgrade: %s.", args.addr, tfdiags.FormatError(err)),
		))
	}
	// Assign new private state returned from state transformation
	newSrc.Private = newPrivate
	return newSrc, diags
}

// stripRemovedStateAttributes deletes any attributes no longer present in the
// schema, so that the json can be correctly decoded.
func stripRemovedStateAttributes(state []byte, ty cty.Type) []byte {
	jsonMap := map[string]interface{}{}
	err := json.Unmarshal(state, &jsonMap)
	if err != nil {
		// we just log any errors here, and let the normal decode process catch
		// invalid JSON.
		log.Printf("[ERROR] UpgradeResourceState: stripRemovedStateAttributes: %s", err)
		return state
	}

	// if no changes were made, we return the original state to ensure nothing
	// was altered in the marshaling process.
	if !removeRemovedAttrs(jsonMap, ty) {
		return state
	}

	js, err := json.Marshal(jsonMap)
	if err != nil {
		// if the json map was somehow mangled enough to not marshal, something
		// went horribly wrong
		panic(err)
	}

	return js
}

// strip out the actual missing attributes, and return a bool indicating if any
// changes were made.
func removeRemovedAttrs(v interface{}, ty cty.Type) bool {
	modified := false
	// we're only concerned with finding maps that correspond to object
	// attributes
	switch v := v.(type) {
	case []interface{}:
		switch {
		// If these aren't blocks the next call will be a noop
		case ty.IsListType() || ty.IsSetType():
			eTy := ty.ElementType()
			for _, eV := range v {
				modified = removeRemovedAttrs(eV, eTy) || modified
			}
		}
		return modified
	case map[string]interface{}:
		switch {
		case ty.IsMapType():
			// map blocks aren't yet supported, but handle this just in case
			eTy := ty.ElementType()
			for _, eV := range v {
				modified = removeRemovedAttrs(eV, eTy) || modified
			}
			return modified

		case ty == cty.DynamicPseudoType:
			log.Printf("[DEBUG] UpgradeResourceState: ignoring dynamic block: %#v\n", v)
			return false

		case ty.IsObjectType():
			attrTypes := ty.AttributeTypes()
			for attr, attrV := range v {
				attrTy, ok := attrTypes[attr]
				if !ok {
					log.Printf("[DEBUG] UpgradeResourceState: attribute %q no longer present in schema", attr)
					delete(v, attr)
					modified = true
					continue
				}

				modified = removeRemovedAttrs(attrV, attrTy) || modified
			}
			return modified
		default:
			// This shouldn't happen, and will fail to decode further on, so
			// there's no need to handle it here.
			log.Printf("[WARN] UpgradeResourceState: unexpected type %#v for map in json state", ty)
			return false
		}
	}
	return modified
}
