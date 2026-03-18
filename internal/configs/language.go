// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configs

import (
	"fmt"
	"strings"

	hcVersion "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"

	"github.com/opentofu/opentofu/version"
)

// validateLanguageBlock checks the validity of a "language" block and returns
// any diagnostics related to it.
//
// Note that this DOES NOT check whether the version constraints in the block
// match the current version of OpenTofu. Instead that happens as part of
// [checkVersionRequirements], which we run separately before other decoding
// work to maximize the chance of us being able to report that the module is
// declared incompatible instead of complaining about use of a language feature
// this version doesn't understand.
//
// Currently we do not retain any of the information from a language block
// after validating it. Instead, we interpret it just enough to generate useful
// error messages if we encounter something that seems like how we expect these
// language features might be used in future versions of OpenTofu.
func validateLanguageBlock(block *hcl.Block, override bool) hcl.Diagnostics {
	var diags hcl.Diagnostics

	if override {
		// Language blocks are not allowed in override files, because we want
		// each module to have a clear central definition of what it's
		// compatible with and what language features it intends to use.
		//
		// These settings have whole-module scope, so allowing overrides would
		// have potentially-surprising effects on other declarations elsewhere
		// in the module.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Language selections in override file",
			Detail:   "Language-related settings in \"language\" blocks are not allowed in override files. Place these settings in a normal configuration file.",
			Subject:  block.DefRange.Ptr(),
		})
		return diags
	}

	content, moreDiags := block.Body.Content(languageBlockSchema)
	diags = append(diags, moreDiags...)

	if attr, ok := content.Attributes["edition"]; ok {
		// OpenTofu does not currently make any real use of language editions,
		// since there is only one "living" edition of the language right now.
		// This is reserved just so that if we decide to introduce a new edition
		// later then older versions of OpenTofu will return a more helpful
		// error message, rather than just returning a generic about the
		// argument being unrecognized.
		kw := hcl.ExprAsKeyword(attr.Expr)
		currentVersion := version.SemVer.String()
		const firstEdition = "tofu2024"
		switch {
		case kw == "": // (the expression wasn't a keyword at all)
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid language edition",
				Detail: fmt.Sprintf(
					"The \"edition\" argument expects a bare language edition keyword. OpenTofu %s supports only language edition %s, which is the default.",
					currentVersion, firstEdition,
				),
				Subject: attr.Expr.Range().Ptr(),
			})
		case strings.HasPrefix(kw, "TF"):
			// OpenTofu's predecessor was accepting "TF2021" as its single valid
			// language edition keyword at the time we forked from it, so we'll
			// use a specialized error message for this just in case someone
			// found that in their documentation and tried to use it in OpenTofu.
			// Note that this would appear only if someone tried to use a
			// keyword like this in the OpenTofu-defined "language" block, so
			// it seems unlikely that anyone would actually see this in practice,
			// but if it _does_ come up then it'd be weird to tell the operator
			// that it requires "a different version of OpenTofu CLI".
			//
			// The syntax that our predecessor would've used -- a "language"
			// argument inside a "terraform" block -- is still accepted by
			// OpenTofu, but now completely ignored because we can't predict
			// how future versions of their language would use that.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsupported language edition",
				Detail: fmt.Sprintf(
					"OpenTofu v%s does not support language edition %q. This module may be intended for use with other software.",
					currentVersion, firstEdition,
				),
				Subject: attr.Expr.Range().Ptr(),
			})
		case kw != firstEdition:
			rel := "different"
			if kw > firstEdition {
				rel = "newer"
			}
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsupported language edition",
				Detail: fmt.Sprintf(
					"OpenTofu v%s only supports language edition %s. This module requires a %s version of OpenTofu CLI.",
					currentVersion, firstEdition, rel,
				),
				Subject: attr.Expr.Range().Ptr(),
			})
		}
	}

	if attr, ok := content.Attributes["experiments"]; ok {
		moreDiags := decodeReservedExperimentsAttr(attr)
		diags = append(diags, moreDiags...)
	}

	var compatibleWithOpenTofu *VersionConstraint
	for _, nestedBlock := range content.Blocks {
		switch nestedBlock.Type {
		case "compatible_with":
			// Note that we don't actually check whether the declared version
			// constraint matches the current version of OpenTofu here, because
			// that should have been checked by some earlier call to
			// [checkVersionRequirements], which extracts the same information
			// we're reading here in a cautious way that's more likely to
			// succeed in a module intended for a later OpenTofu version.
			//
			// The checks here are just about whether the declarations are
			// valid regardless of which versions it allows.
			if compatibleWithOpenTofu != nil {
				// Each language block should have at most one compatible_with
				// block referring to OpenTofu, but we'll ignore blocks that
				// don't mention OpenTofu at all.
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Duplicate compatible_with block",
					Detail: fmt.Sprintf(
						"Each language block may have at most one compatible_with block referring to OpenTofu. The OpenTofu version constraint was already declared at %s.",
						compatibleWithOpenTofu.DeclRange,
					),
					Subject: block.DefRange.Ptr(),
				})
				continue
			}
			constraint, moreDiags := decodeLanguageCompatibleWithOpenTofu(nestedBlock)
			diags = append(diags, moreDiags...)
			if !moreDiags.HasErrors() {
				if constraint.Required.Check(hcVersion.Must(hcVersion.NewVersion("1.11.0"))) {
					// This language feature was added in OpenTofu v1.12, so it
					// isn't suitable for describing compatibility with earlier
					// versions of OpenTofu. We'll return a warning to help module
					// authors notice that even if they are only testing with newer
					// versions of OpenTofu.
					diags = diags.Append(&hcl.Diagnostic{
						Severity: hcl.DiagWarning,
						Summary:  "Ineffective version constraint",
						Detail:   "The compatible_with block was added in OpenTofu v1.12.0, so any constraint specified this way should exclude earlier versions of OpenTofu, such as by including \">= 0.12.0\".\n\nIf your module must be compatible with earlier versions of OpenTofu, use the required_version argument in a \"terraform\" block in a file named with the .tofu suffix, which is an older way to specify OpenTofu version constraints.",
						Subject:  constraint.DeclRange.Ptr(),
					})
				}
			}
			compatibleWithOpenTofu = constraint
		default:
			// It should not be possible to get here because HCL should've
			// rejected any other block types as not being in the schema.
			panic(fmt.Sprintf("unexpected block type %q", nestedBlock.Type))
		}
	}

	return diags
}

// decodeLanguageCompatibleWithOpenTofu takes a [hcl.Block] representing a
// "compatible_with" block inside a "language" block and attempts to recognize
// an "opentofu" argument within it, returning its associated version constraint
// if present.
//
// This function intentionally silently ignores anything else appearing in that
// block so that additional arguments can be used by other software that works
// with OpenTofu modules.
func decodeLanguageCompatibleWithOpenTofu(block *hcl.Block) (*VersionConstraint, hcl.Diagnostics) {
	var ret *VersionConstraint
	content, _, diags := block.Body.PartialContent(languageCompatibleWithSchema)
	if attr, ok := content.Attributes["opentofu"]; ok {
		constraint, moreDiags := decodeVersionConstraint(attr)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			ret = &constraint
		}
	}
	return ret, diags
}

func validateOpenTofuCoreVersionConstraint(constraint VersionConstraint) hcl.Diagnostics {
	var diags hcl.Diagnostics
	// We don't permit writing prerelease versions in the version
	// constraint arguments. We don't actually know why this rule is
	// here but it was inherited from our predecessor and preserved
	// for consistency until we know a reason to allow it.
	for _, required := range constraint.Required {
		if required.Prerelease() {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid required_version constraint",
				Detail: fmt.Sprintf(
					"Prerelease version constraints are not supported: %s. Remove the prerelease information from the constraint. Prerelease versions of OpenTofu will match constraints using their version core only.",
					required.String(),
				),
				Subject: constraint.DeclRange.Ptr(),
			})
		}
	}
	return diags
}

// decodeReservedExperimentsAttr decodes the "experiments" attribute in a
// "language" block just enough to return error messages if it's being used
// in ways we expect we might use it in future versions of OpenTofu.
func decodeReservedExperimentsAttr(attr *hcl.Attribute) hcl.Diagnostics {
	var diags hcl.Diagnostics

	exprs, moreDiags := hcl.ExprList(attr.Expr)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return diags
	}

	for _, expr := range exprs {
		kw := hcl.ExprAsKeyword(expr)
		if kw == "" {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid experiment keyword",
				Detail:   "Elements of \"experiments\" must all be keywords representing active experiments.",
				Subject:  expr.Range().Ptr(),
			})
			continue
		}
		// The current version of OpenTofu does not support any language
		// experiments, so we'll just reject anything we find in here.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unknown experiment keyword",
			Detail:   fmt.Sprintf("There is no current experiment with the keyword %q.", kw),
			Subject:  expr.Range().Ptr(),
		})
	}

	return diags
}

var languageBlockSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "edition"},
		{Name: "experiments"},
	},
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "compatible_with"},
	},
}

var languageCompatibleWithSchema = &hcl.BodySchema{
	// This describes only the subset that OpenTofu uses. This block should be
	// decoded using [hcl.Body.PartialContent] so as to ignore anything that's
	// not included in this schema.
	Attributes: []hcl.AttributeSchema{
		{Name: "opentofu"},
	},
}
