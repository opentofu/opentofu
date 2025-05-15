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
	Destroy   bool
	DeclRange tfdiags.SourceRange
	// Provisioners here are used only to be able to return these to the transformer that is injecting these
	// in the nodes that are supporting this kind of operation. To get a better understanding of the usage of this field,
	// check ResourceRemovedProvisioners
	Provisioners []*configs.Provisioner
}

// FindRemoveStatements recurses through the modules of the given configuration
// and returns an array of all "removed" addresses within, in a
// deterministic but undefined order.
// We also validate that the removed modules/resources configuration blocks were removed.
func FindRemoveStatements(rootCfg *configs.Config) ([]*RemoveStatement, tfdiags.Diagnostics) {
	rm := findRemoveStatements(rootCfg, nil)
	diags := validateRemoveStatements(rootCfg, rm)
	return rm, diags
}

// FindResourceRemovedStatement returns the RemoveStatement if found for the given resAddr.
// This function is searching in the Config for any "removed" block targeting the given resource.
// This method shouldn't be concerned if resAddr is pointing to a "module" or a "data" block because all of
// these will be validated way before this function is going to be called.
func FindResourceRemovedStatement(rootCfg *configs.Config, resAddr addrs.ConfigResource) *RemoveStatement {
	rm := findRemoveStatements(rootCfg, nil)
	// no need to call validateRemoveStatements again since these should have been validated in the plan phase
	for _, rs := range rm {
		if rs.From.TargetContains(resAddr) {
			return rs
		}
	}
	return nil
}

// FindResourceRemovedBlockProvisioners is returning the provisioners of the RemoveStatement found by calling FindResourceRemovedStatement
func FindResourceRemovedBlockProvisioners(rootCfg *configs.Config, resAddr addrs.ConfigResource) []*configs.Provisioner {
	if rs := FindResourceRemovedStatement(rootCfg, resAddr); rs != nil {
		return rs.Provisioners
	}
	return nil
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

			removedEndpoint = &RemoveStatement{From: absConfigResource, Destroy: rc.Destroy, DeclRange: tfdiags.SourceRangeFromHCL(rc.DeclRange), Provisioners: rc.Provisioners}

		case addrs.Module:
			// Get the absolute address of the module by appending the module config address
			// to the module itself
			var absModule = make(addrs.Module, 0, len(modAddr)+len(FromAddress))
			absModule = append(absModule, modAddr...)
			absModule = append(absModule, FromAddress...)
			removedEndpoint = &RemoveStatement{From: absModule, Destroy: rc.Destroy, DeclRange: tfdiags.SourceRangeFromHCL(rc.DeclRange), Provisioners: rc.Provisioners}

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
