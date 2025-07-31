// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tf

import (
	"fmt"

	"github.com/hashicorp/go-uuid"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

func dataStoreResourceSchema() providers.Schema {
	return providers.Schema{
		Block: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"input":            {Type: cty.DynamicPseudoType, Optional: true},
				"output":           {Type: cty.DynamicPseudoType, Computed: true},
				"triggers_replace": {Type: cty.DynamicPseudoType, Optional: true},
				"id":               {Type: cty.String, Computed: true},
			},
		},
	}
}

func validateDataStoreResourceConfig(req providers.ValidateResourceConfigRequest) (resp providers.ValidateResourceConfigResponse) {
	if req.Config.IsNull() {
		return resp
	}

	// Core does not currently validate computed values are not set in the
	// configuration.
	for _, attr := range []string{"id", "output"} {
		if !req.Config.GetAttr(attr).IsNull() {
			resp.Diagnostics = resp.Diagnostics.Append(fmt.Errorf(`%q attribute is read-only`, attr))
		}
	}
	return resp
}

func upgradeDataStoreResourceState(req providers.UpgradeResourceStateRequest) (resp providers.UpgradeResourceStateResponse) {
	ty := dataStoreResourceSchema().Block.ImpliedType()
	val, err := ctyjson.Unmarshal(req.RawStateJSON, ty)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}

	resp.UpgradedState = val
	return resp
}

// nullResourceSchema returns a schema for a null_resource with relevant attributes for type migration.
func nullResourceSchema() providers.Schema {
	return providers.Schema{
		Block: &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"triggers": {Type: cty.Map(cty.String), Optional: true},
				"id":       {Type: cty.String, Computed: true},
			},
		},
	}

}

func moveDataStoreResourceState(req providers.MoveResourceStateRequest) providers.MoveResourceStateResponse {
	var resp providers.MoveResourceStateResponse
	if req.SourceTypeName != "null_resource" || req.TargetTypeName != "terraform_data" {
		resp.Diagnostics = resp.Diagnostics.Append(
			fmt.Errorf("unsupported move: %s -> %s; only move from null_resource to terraform_data is supported",
				req.SourceTypeName, req.TargetTypeName))
		return resp
	}
	nullTy := nullResourceSchema().Block.ImpliedType()
	oldState, err := ctyjson.Unmarshal(req.SourceStateJSON, nullTy)
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	oldStateMap := oldState.AsValueMap()
	newStateMap := map[string]cty.Value{}

	if trigger, ok := oldStateMap["triggers"]; ok && !trigger.IsNull() {
		newStateMap["triggers_replace"] = cty.ObjectVal(trigger.AsValueMap())
	}
	if id, ok := oldStateMap["id"]; ok && !id.IsNull() {
		newStateMap["id"] = id
	}

	currentSchema := dataStoreResourceSchema()
	newState, err := currentSchema.Block.CoerceValue(cty.ObjectVal(newStateMap))
	if err != nil {
		resp.Diagnostics = resp.Diagnostics.Append(err)
		return resp
	}
	resp.TargetState = newState
	resp.TargetPrivate = req.SourcePrivate
	return resp
}

func readDataStoreResourceState(req providers.ReadResourceRequest) (resp providers.ReadResourceResponse) {
	resp.NewState = req.PriorState
	return resp
}

func planDataStoreResourceChange(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
	if req.ProposedNewState.IsNull() {
		// destroy op
		resp.PlannedState = req.ProposedNewState
		return resp
	}

	planned := req.ProposedNewState.AsValueMap()

	input := req.ProposedNewState.GetAttr("input")
	trigger := req.ProposedNewState.GetAttr("triggers_replace")

	switch {
	case req.PriorState.IsNull():
		// Create
		// Set the id value to unknown.
		planned["id"] = cty.UnknownVal(cty.String).RefineNotNull()

		// Output type must always match the input, even when it's null.
		if input.IsNull() {
			planned["output"] = input
		} else {
			planned["output"] = cty.UnknownVal(input.Type())
		}

		resp.PlannedState = cty.ObjectVal(planned)
		return resp

	case !req.PriorState.GetAttr("triggers_replace").RawEquals(trigger):
		// trigger changed, so we need to replace the entire instance
		resp.RequiresReplace = append(resp.RequiresReplace, cty.GetAttrPath("triggers_replace"))
		planned["id"] = cty.UnknownVal(cty.String).RefineNotNull()

		// We need to check the input for the replacement instance to compute a
		// new output.
		if input.IsNull() {
			planned["output"] = input
		} else {
			planned["output"] = cty.UnknownVal(input.Type())
		}

	case !req.PriorState.GetAttr("input").RawEquals(input):
		// only input changed, so we only need to re-compute output
		planned["output"] = cty.UnknownVal(input.Type())
	}

	resp.PlannedState = cty.ObjectVal(planned)
	return resp
}

var testUUIDHook func() string

func applyDataStoreResourceChange(req providers.ApplyResourceChangeRequest) (resp providers.ApplyResourceChangeResponse) {
	if req.PlannedState.IsNull() {
		resp.NewState = req.PlannedState
		return resp
	}

	newState := req.PlannedState.AsValueMap()

	if !req.PlannedState.GetAttr("output").IsKnown() {
		newState["output"] = req.PlannedState.GetAttr("input")
	}

	if !req.PlannedState.GetAttr("id").IsKnown() {
		idString, err := uuid.GenerateUUID()
		// OpenTofu would probably never get this far without a good random
		// source, but catch the error anyway.
		if err != nil {
			diag := tfdiags.AttributeValue(
				tfdiags.Error,
				"Error generating id",
				err.Error(),
				cty.GetAttrPath("id"),
			)

			resp.Diagnostics = resp.Diagnostics.Append(diag)
		}

		if testUUIDHook != nil {
			idString = testUUIDHook()
		}

		newState["id"] = cty.StringVal(idString)
	}

	resp.NewState = cty.ObjectVal(newState)

	return resp
}

// TODO: This isn't very useful even for examples, because terraform_data has
// no way to refresh the full resource value from only the import ID. This
// minimal implementation allows the import to succeed, and can be extended
// once the configuration is available during import.
func importDataStore(req providers.ImportResourceStateRequest) (resp providers.ImportResourceStateResponse) {
	schema := dataStoreResourceSchema()
	v := cty.ObjectVal(map[string]cty.Value{
		"id": cty.StringVal(req.ID),
	})
	state, err := schema.Block.CoerceValue(v)
	resp.Diagnostics = resp.Diagnostics.Append(err)

	resp.ImportedResources = []providers.ImportedResource{
		{
			TypeName: req.TypeName,
			State:    state,
		},
	}
	return resp
}
