// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

type AbsReference struct {
	Module      ModuleInstance
	Subject     Referenceable
	SourceRange tfdiags.SourceRange
	Remaining   hcl.Traversal
}

// DisplayString returns a string that approximates the subject and remaining
// traversal of the receiver in a way that resembles the OpenTofu CLI
// syntax that could've produced it.
func (r *AbsReference) DisplayString() string {
	localRef := r.LocalReference()
	if r.Module.IsRoot() {
		return localRef.DisplayString()
	}
	return r.Module.String() + ":" + localRef.DisplayString()
}

// LocalReference returns a [Reference] representation of the same reference,
// discarding the address of the module instance it belongs to.
func (r *AbsReference) LocalReference() *Reference {
	return &Reference{
		Subject:     r.Subject,
		SourceRange: r.SourceRange,
		Remaining:   r.Remaining,
	}
}

// ParseAbsRef attempts to parse the given bytes as an absolute reference,
// as would be used with the "-watch=..." command line option on various
// commands that interact with the core language runtime.
//
// If no error diagnostics are returned, the returned reference includes the
// address that was extracted, the source range it was extracted from, and any
// remaining relative traversal that was not consumed as part of the
// reference.
//
// If error diagnostics are returned then the Reference value is invalid and
// must not be used.
func ParseAbsRef(src []byte, filename string, start hcl.Pos) (*AbsReference, tfdiags.Diagnostics) {
	moduleInstRng, _, localRefRng, diags := splitAbsRefAddr(src, filename, start)
	if diags.HasErrors() {
		return nil, diags
	}
	moduleInstSrc := src[moduleInstRng.Start.Byte-start.Byte : moduleInstRng.End.Byte-start.Byte]
	localRefSrc := src[localRefRng.Start.Byte-start.Byte : localRefRng.End.Byte-start.Byte]

	var hclDiags hcl.Diagnostics
	var moduleInstTraversal hcl.Traversal
	if len(moduleInstSrc) != 0 {
		moduleInstTraversal, hclDiags = hclsyntax.ParseTraversalAbs(moduleInstSrc, moduleInstRng.Filename, moduleInstRng.Start)
		diags = diags.Append(hclDiags)
	}
	localRefTraversal, hclDiags := hclsyntax.ParseTraversalAbs(localRefSrc, localRefRng.Filename, localRefRng.Start)
	diags = diags.Append(hclDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	var moreDiags tfdiags.Diagnostics
	moduleInst := RootModuleInstance
	if len(moduleInstTraversal) != 0 {
		moduleInst, moreDiags = ParseModuleInstance(moduleInstTraversal)
		diags = diags.Append(moreDiags)
	}
	localRef, moreDiags := ParseRef(localRefTraversal)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		return nil, diags
	}

	var overallRng tfdiags.SourceRange
	if moduleInstRng.Empty() {
		overallRng = localRef.SourceRange
	} else {
		overallRng = tfdiags.SourceRangeFromHCL(hcl.RangeBetween(moduleInstRng, localRef.SourceRange.ToHCL()))
	}

	return &AbsReference{
		Module:      moduleInst,
		Subject:     localRef.Subject,
		Remaining:   localRef.Remaining,
		SourceRange: overallRng,
	}, diags
}

// ParseAbsRefStr wraps [ParseAbsRef], parsing the given string without
// any source location information, such as when the string came from
// a command line option.
func ParseAbsRefStr(src string) (*AbsReference, tfdiags.Diagnostics) {
	return ParseAbsRef([]byte(src), "", hcl.InitialPos)
}

// splitAbsRefAddr takes a byte slice containing the source representation
// of an [AbsReference] and returns source ranges for the module instance part,
// the colon delimiter, and the local reference part respectively.
//
// If the colon part is zero-length then no suitable colon delimiter was found,
// in which case this is likely to be a reference to something in the root
// module.
func splitAbsRefAddr(src []byte, filename string, start hcl.Pos) (moduleInst, colon, localRef hcl.Range, diags tfdiags.Diagnostics) {
	tokens, hclDiags := hclsyntax.LexExpression(src, filename, start)
	diags = diags.Append(hclDiags)
	if hclDiags.HasErrors() {
		return moduleInst, colon, localRef, diags
	}
	emptyRange := hcl.Range{
		Filename: filename,
		Start:    start,
		End:      start,
	}
	moduleInst = emptyRange
	colon = emptyRange
	localRef = emptyRange
	if len(tokens) == 0 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid absolute reference address",
			Detail:   "Cannot use an empty string as an absolute reference address.",
			Subject:  emptyRange.Ptr(),
		})
		return moduleInst, colon, emptyRange, diags
	}

	// When the input is valid, the first [hclsyntax.TokenColon] should mark
	// the boundary between the module instance and the local reference, but
	// if the input is invalid we'll be able to generate a better error
	// message if we make some effort to ignore colons that appear to be
	// nested inside any sort of bracketing pair, since then HCL's traversal
	// parser can report the misplaced colon(s) in its usual way, taking more
	// context into account.
	var openBrackets []hclsyntax.TokenType
	for i, token := range tokens {
		if len(openBrackets) > 0 && openBrackets[len(openBrackets)-1] == token.Type {
			openBrackets = openBrackets[:len(openBrackets)-1]
		}
		switch token.Type {
		case hclsyntax.TokenColon:
			if len(openBrackets) != 0 {
				break
			}
			// This is our candidate colon, so everything before this is
			// moduleInst and everything after is localRef.
			colon = token.Range
			if moduleInstTokens := tokens[:i]; len(moduleInstTokens) != 0 {
				moduleInst = hcl.RangeBetween(moduleInstTokens[0].Range, moduleInstTokens[len(moduleInstTokens)-1].Range)
			}
			if localRefTokens := tokens[i+1:]; len(localRefTokens) != 0 {
				localRef = hcl.RangeBetween(localRefTokens[0].Range, localRefTokens[len(localRefTokens)-1].Range)
			}
			return moduleInst, colon, localRef, diags
		case hclsyntax.TokenOParen:
			openBrackets = append(openBrackets, hclsyntax.TokenCParen)
		case hclsyntax.TokenOBrack:
			openBrackets = append(openBrackets, hclsyntax.TokenCBrack)
		case hclsyntax.TokenOBrace:
			openBrackets = append(openBrackets, hclsyntax.TokenCBrace)
		case hclsyntax.TokenOQuote:
			openBrackets = append(openBrackets, hclsyntax.TokenCQuote)
		case hclsyntax.TokenOHeredoc:
			openBrackets = append(openBrackets, hclsyntax.TokenCHeredoc)
		case hclsyntax.TokenTemplateInterp, hclsyntax.TokenTemplateControl:
			openBrackets = append(openBrackets, hclsyntax.TokenTemplateSeqEnd)
		}
	}

	// If we fall out here then it seems like we don't have a candidate
	// colon at all, and so we'll assume this is intended to be a root module
	// reference without any explicit module instance address.
	localRef = hcl.RangeBetween(tokens[0].Range, tokens[len(tokens)-1].Range)
	return moduleInst, colon, localRef, diags
}
