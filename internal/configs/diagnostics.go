// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"slices"

	hcVersion "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
)

// incompatibleModuleDiagnosticExtra is the type used for a value in the
// "ExtraInfo" field of a diagnostic to mark it as representing that a module
// is somehow incompatible with the current version of OpenTofu.
//
// When loading a module, if any of the diagnostics have a value of this type
// in their extra info then we discard all other diagnostics because they are
// possibly describing attempts to use language features that are not available
// in the current version of OpenTofu.
//
// There is only one value of this type, which is its zero value.
type incompatibleModuleDiagnosticExtra struct{}

// finalizeModuleLoadDiagnostics should be called at the end of loading and
// merging together all of the files for a module, to make final adjustments
// before returning diagnostics for presentation to the user.
//
// The return value shares a backing array with the given diagnostics, so
// the caller must treat the given slice as invalid after passing it to this
// function and must use the return value in place of it.
func finalizeModuleLoadDiagnostics(diags hcl.Diagnostics) hcl.Diagnostics {
	// This is currently focused only on noticing whether there are any
	// "version mismatch" diagnostics and, if so, discarding any other
	// diagnostics.

	haveVersionMismatches := slices.ContainsFunc(diags, isIncompatibleModuleDiagnostic)
	if !haveVersionMismatches {
		// In the common case where there are no version mismatches, we just
		// return the given diagnostics back verbatim.
		return diags
	}

	// If we get here then we modify the backing array in-place to remove
	// any diagnostics that are not talking about incompatibility.
	return slices.DeleteFunc(diags, func(diag *hcl.Diagnostic) bool {
		return !isIncompatibleModuleDiagnostic(diag)
	})
}

func isIncompatibleModuleDiagnostic(diag *hcl.Diagnostic) bool {
	_, ok := diag.Extra.(incompatibleModuleDiagnosticExtra)
	return ok
}

// checkVersionRequirements does minimal parsing of the given body for
// the different ways that module authors are allowed to specify which versions
// of OpenTofu a module is compatible with, returning error diagnostics if
// any declaration excludes the current version of OpenTofu.
//
// This is guaranteed to not return any error diagnostics if all of the
// declarations it finds allow the current version of OpenTofu.
//
// This is intended to maximize the chance that we'll be able to read the
// requirements (syntax errors notwithstanding) even if the config file contains
// constructs that might've been added in future OpenTofu versions
//
// This is a "best effort" sort of method which will check constraints it is
// able to find, but might not succeed if the given body is too invalid to
// be processed at all.
func checkVersionRequirements(body hcl.Body, expectedVersion *hcVersion.Version) hcl.Diagnostics {
	rootContent, _, diags := body.PartialContent(configFileVersionConstraintSniffRootSchema)

	incompatibleDiag := func(rng hcl.Range) *hcl.Diagnostic {
		return &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Incompatible module",
			Detail:   fmt.Sprintf("This module is not compatible with OpenTofu v%s.\n\nTo proceed, either choose another supported OpenTofu version or update this version constraint. Version constraints are normally set for good reason, so updating the constraint may lead to other errors or unexpected behavior.", expectedVersion.String()),
			Subject:  rng.Ptr(),
			// This "Extra" is used by [finalizeModuleLoadDiagnostics] to
			// discard all other diagnostics whenever at least one
			// incompatibility-related diagnostic is present.
			Extra: incompatibleModuleDiagnosticExtra{},
		}
	}

	for _, block := range rootContent.Blocks {
		switch block.Type {
		case "language":
			// New-style language block. In this case we're looking for nested
			// blocks of type "compatible_with", which may or may not contain
			// OpenTofu version constraints.
			content, _, blockDiags := block.Body.PartialContent(configFileModernVersionConstraintSniffSchema)
			diags = append(diags, blockDiags...)
			for _, nestedBlock := range content.Blocks {
				if nestedBlock.Type != "compatible_with" {
					continue
				}
				constraint, constraintDiags := decodeLanguageCompatibleWithOpenTofu(nestedBlock)
				diags = append(diags, constraintDiags...)
				if constraint != nil {
					validDiags := validateOpenTofuCoreVersionConstraint(*constraint)
					diags = append(diags, validDiags...)
					if !validDiags.HasErrors() && !constraint.Required.Check(expectedVersion) {
						diags = diags.Append(incompatibleDiag(constraint.DeclRange))
					}
				}
			}

		case "terraform":
			// Legacy style of version constraint, using the required_version
			// argument in a "terraform" block. We only pay attention to these
			// in OpenTofu-specific files, because otherwise we assume they
			// are intended to constrain our predecessor instead.
			if ext := tofuFileExt(block.DefRange.Filename); ext == "" {
				continue // not an OpenTofu-specific file
			}

			content, _, blockDiags := block.Body.PartialContent(configFileLegacyVersionConstraintSniffSchema)
			diags = append(diags, blockDiags...)

			attr, exists := content.Attributes["required_version"]
			if !exists {
				continue
			}

			constraint, constraintDiags := decodeVersionConstraint(attr)
			diags = append(diags, constraintDiags...)
			if !constraintDiags.HasErrors() {
				validDiags := validateOpenTofuCoreVersionConstraint(constraint)
				diags = append(diags, validDiags...)
				if !validDiags.HasErrors() && !constraint.Required.Check(expectedVersion) {
					diags = diags.Append(incompatibleDiag(constraint.DeclRange))
				}
			}
		}
	}

	return diags
}

// configFileVersionConstraintSniffRootSchema is a schema for
// sniffCoreVersionRequirements and sniffActiveExperiments.
var configFileVersionConstraintSniffRootSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "terraform"},
		{Type: "language"},
	},
}

// configFileLegacyVersionConstraintSniffSchema is a schema for checkVersionRequirements
var configFileLegacyVersionConstraintSniffSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "required_version"},
	},
}

// configFileModernVersionConstraintSniffSchema is a schema for checkVersionRequirements
var configFileModernVersionConstraintSniffSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "compatible_with"},
	},
}
