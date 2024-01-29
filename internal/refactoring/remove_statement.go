// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package refactoring

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type RemoveStatement struct {
	From      addrs.ConfigRemovable
	DeclRange tfdiags.SourceRange
}

// GetEndpointsToForget recurses through the modules of the given configuration
// and returns an array of all "removed" addresses within, in a
// deterministic but undefined order.
// We also validate that the removed modules/resources configuration blocks were removed.
func GetEndpointsToForget(rootCfg *configs.Config) ([]addrs.ConfigRemovable, tfdiags.Diagnostics) {
	rm := findRemoveStatements(rootCfg, nil)
	diags := validateRemoveStatements(rootCfg, rm)
	removedAddresses := make([]addrs.ConfigRemovable, len(rm))
	for i, rs := range rm {
		removedAddresses[i] = rs.From
	}
	return removedAddresses, diags
}

func findRemoveStatements(cfg *configs.Config, into []*RemoveStatement) []*RemoveStatement {
	modAddr := cfg.Path

	for _, rc := range cfg.Module.Removed {
		var removedEndpoint *RemoveStatement
		switch FromAddress := rc.From.RelSubject.(type) {
		case addrs.ConfigResource:
			// Get the absolute address of the resource by appending the module config address
			// to the resource's relative address
			absModule := make(addrs.Module, 0, len(modAddr)+len(FromAddress.Module))
			absModule = append(absModule, modAddr...)
			absModule = append(absModule, FromAddress.Module...)

			var absConfigResource addrs.ConfigRemovable = addrs.ConfigResource{
				Resource: FromAddress.Resource,
				Module:   absModule,
			}

			removedEndpoint = &RemoveStatement{From: absConfigResource, DeclRange: tfdiags.SourceRangeFromHCL(rc.DeclRange)}

		case addrs.Module:
			// Get the absolute address of the module by appending the module config address
			// to the module itself
			var absModule = make(addrs.Module, 0, len(modAddr)+len(FromAddress))
			absModule = append(absModule, modAddr...)
			absModule = append(absModule, FromAddress...)
			removedEndpoint = &RemoveStatement{From: absModule, DeclRange: tfdiags.SourceRangeFromHCL(rc.DeclRange)}

		default:
			panic(fmt.Sprintf("unhandled address type %T", FromAddress))
		}

		into = append(into, removedEndpoint)

	}

	for _, childCfg := range cfg.Children {
		into = findRemoveStatements(childCfg, into)
	}

	return into
}

// validateRemoveStatements validates that the removed modules/resources configuration blocks were removed.
func validateRemoveStatements(cfg *configs.Config, removeStatements []*RemoveStatement) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	for _, rs := range removeStatements {
		fromAddr := rs.From
		if fromAddr == nil {
			// Invalid value should've been caught during original
			// configuration decoding, in the configs package.
			panic(fmt.Sprintf("incompatible Remove endpoint in %s", rs.DeclRange.ToHCL()))
		}

		// validate that a resource/module with this address doesn't exist in the config
		switch fromAddr := fromAddr.(type) {
		case addrs.ConfigResource:
			moduleConfig := cfg.Descendent(fromAddr.Module)
			if moduleConfig != nil && moduleConfig.Module.ResourceByAddr(fromAddr.Resource) != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Removed resource block still exists",
					Detail: fmt.Sprintf(
						"This statement declares a removal of the resource %s, but this resource block still exists in the configuration. Please remove the resource block.",
						fromAddr,
					),
					Subject: rs.DeclRange.ToHCL().Ptr(),
				})
			}
		case addrs.Module:
			if cfg.Descendent(fromAddr) != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Removed module block still exists",
					Detail: fmt.Sprintf(
						"This statement declares a removal of the module %s, but this module block still exists in the configuration. Please remove the module block.",
						fromAddr,
					),
					Subject: rs.DeclRange.ToHCL().Ptr(),
				})
			}
		default:
			panic(fmt.Sprintf("incompatible Remove endpoint address type in %s", rs.DeclRange.ToHCL()))
		}
	}

	return diags
}
