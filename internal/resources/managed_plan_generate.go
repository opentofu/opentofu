// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package resources

import (
	"context"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/genconfig"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// This is all ported from node_resource_plan_instance.go

func (rt *ManagedResourceType) GenerateConfigFor(ctx context.Context, value cty.Value, providerInstance addrs.AbsProviderInstanceCorrect, dispAddr addrs.AbsResourceInstance) (string, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	schema, moreDiags := rt.LoadSchema(ctx)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return "", diags
	}

	// Generate the HCL string first, then parse the HCL body from it.
	// First we generate the contents of the resource block for use within
	// the planning node. Then we wrap it in an enclosing resource block to
	// pass into the plan for rendering.
	generatedHCLAttributes, generatedDiags := generateHCLStringAttributes(dispAddr, value, providerInstance, schema.Block)
	diags = diags.Append(generatedDiags)

	generatedConfigHCL := genconfig.WrapResourceContents(dispAddr, generatedHCLAttributes)
	return generatedConfigHCL, diags
}

// generateHCLStringAttributes produces a string in HCL format for the given
// resource state and schema without the surrounding block.
func generateHCLStringAttributes(addr addrs.AbsResourceInstance, val cty.Value, providerInstance addrs.AbsProviderInstanceCorrect, schema *configschema.Block) (string, tfdiags.Diagnostics) {
	filteredSchema := schema.Filter(
		configschema.FilterOr(
			configschema.FilterReadOnlyAttribute,
			configschema.FilterDeprecatedAttribute,

			// The legacy SDK adds an Optional+Computed "id" attribute to the
			// resource schema even if not defined in provider code.
			// During validation, however, the presence of an extraneous "id"
			// attribute in config will cause an error.
			// Remove this attribute so we do not generate an "id" attribute
			// where there is a risk that it is not in the real resource schema.
			//
			// TRADEOFF: Resources in which there actually is an
			// Optional+Computed "id" attribute in the schema will have that
			// attribute missing from generated config.
			configschema.FilterHelperSchemaIdAttribute,
		),
		configschema.FilterDeprecatedBlock,
	)

	providerAddr := addrs.LocalProviderConfig{
		LocalName: providerInstance.Config.Config.Provider.Type,
		Alias:     providerInstance.Config.Config.Alias,
	}

	return genconfig.GenerateResourceContents(addr, filteredSchema, providerAddr, val)
}
