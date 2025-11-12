// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package addrs

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// Reference describes a reference to an address with source location
// information.
type Reference struct {
	Subject     Referenceable
	SourceRange tfdiags.SourceRange
	Remaining   hcl.Traversal
}

// DisplayString returns a string that approximates the subject and remaining
// traversal of the receiver in a way that resembles the OpenTofu language
// syntax that could've produced it.
//
// It's not guaranteed to actually be a valid OpenTofu language expression,
// since the intended use here is primarily for UI messages such as
// diagnostics.
func (r *Reference) DisplayString() string {
	if len(r.Remaining) == 0 {
		// Easy case: we can just return the subject's string.
		return r.Subject.String()
	}

	var ret strings.Builder
	ret.WriteString(r.Subject.String())
	for _, step := range r.Remaining {
		switch tStep := step.(type) {
		case hcl.TraverseRoot:
			ret.WriteString(tStep.Name)
		case hcl.TraverseAttr:
			ret.WriteByte('.')
			ret.WriteString(tStep.Name)
		case hcl.TraverseIndex:
			ret.WriteByte('[')
			switch tStep.Key.Type() {
			case cty.String:
				ret.WriteString(fmt.Sprintf("%q", tStep.Key.AsString()))
			case cty.Number:
				bf := tStep.Key.AsBigFloat()
				ret.WriteString(bf.Text('g', 10))
			}
			ret.WriteByte(']')
		}
	}
	return ret.String()
}

// ParseRef attempts to extract a referenceable address from the prefix of the
// given traversal, which must be an absolute traversal or this function
// will panic.
//
// If no error diagnostics are returned, the returned reference includes the
// address that was extracted, the source range it was extracted from, and any
// remaining relative traversal that was not consumed as part of the
// reference.
//
// If error diagnostics are returned then the Reference value is invalid and
// must not be used.
func ParseRef(traversal hcl.Traversal) (*Reference, tfdiags.Diagnostics) {
	ref, diags := parseRef(traversal)

	// Normalize a little to make life easier for callers.
	if ref != nil {
		if len(ref.Remaining) == 0 {
			ref.Remaining = nil
		}
	}

	return ref, diags
}

// ParseRefFromTestingScope adds check blocks and outputs into the available
// references returned by ParseRef.
//
// The testing files and functionality have a slightly expanded referencing
// scope and so should use this function to retrieve references.
func ParseRefFromTestingScope(traversal hcl.Traversal) (*Reference, tfdiags.Diagnostics) {
	root := traversal.RootName()

	var diags tfdiags.Diagnostics
	var reference *Reference

	switch root {
	case "output":
		reference, diags = parseSingleAttrRef(traversal, func(name string) Referenceable {
			return OutputValue{Name: name}
		})
	case "check":
		reference, diags = parseSingleAttrRef(traversal, func(name string) Referenceable {
			return Check{Name: name}
		})
	default:
		// If it's not an output or a check block, then just parse it as normal.
		return ParseRef(traversal)
	}

	if reference != nil && len(reference.Remaining) == 0 {
		reference.Remaining = nil
	}
	return reference, diags
}

// ParseRefStr is a helper wrapper around ParseRef that takes a string
// and parses it with the HCL native syntax traversal parser before
// interpreting it.
//
// This should be used only in specialized situations since it will cause the
// created references to not have any meaningful source location information.
// If a reference string is coming from a source that should be identified in
// error messages then the caller should instead parse it directly using a
// suitable function from the HCL API and pass the traversal itself to
// ParseRef.
//
// Error diagnostics are returned if either the parsing fails or the analysis
// of the traversal fails. There is no way for the caller to distinguish the
// two kinds of diagnostics programmatically. If error diagnostics are returned
// the returned reference may be nil or incomplete.
func ParseRefStr(str string) (*Reference, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return nil, diags
	}

	ref, targetDiags := ParseRef(traversal)
	diags = diags.Append(targetDiags)
	return ref, diags
}

// ParseRefStrFromTestingScope matches ParseRefStr except it supports the
// references supported by ParseRefFromTestingScope.
func ParseRefStrFromTestingScope(str string) (*Reference, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return nil, diags
	}

	ref, targetDiags := ParseRefFromTestingScope(traversal)
	diags = diags.Append(targetDiags)
	return ref, diags
}

func parseRef(traversal hcl.Traversal) (*Reference, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	root := traversal.RootName()
	rootRange := traversal[0].SourceRange()

	switch root {
	case "count":
		return parseSingleAttrRef(traversal, func(name string) Referenceable {
			return CountAttr{Name: name}
		})
	case "each":
		return parseSingleAttrRef(traversal, func(name string) Referenceable {
			return ForEachAttr{Name: name}
		})
	case "data":
		if len(traversal) < 3 {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid reference",
				Detail:   `The "data" object must be followed by two attribute names: the data source type and the resource name.`,
				Subject:  traversal.SourceRange().Ptr(),
			})
			return nil, diags
		}
		remain := traversal[1:] // trim off "data" so we can use our shared resource reference parser
		return parseResourceRef(DataResourceMode, rootRange, remain)
	case "ephemeral":
		if len(traversal) < 3 {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid reference",
				Detail:   `The "ephemeral" object must be followed by two attribute names: the ephemeral resource type and its name.`,
				Subject:  traversal.SourceRange().Ptr(),
			})
			return nil, diags
		}
		remain := traversal[1:] // trim off "ephemeral" so we can use our shared resource reference parser
		return parseResourceRef(EphemeralResourceMode, rootRange, remain)
	case "resource":
		// This is an alias for the normal case of just using a managed resource
		// type as a top-level symbol, which will serve as an escape mechanism
		// if a later edition of the OpenTofu language introduces a new
		// reference prefix that conflicts with a resource type name in an
		// existing provider. In that case, the edition upgrade tool can
		// rewrite foo.bar into resource.foo.bar to ensure that "foo" remains
		// interpreted as a resource type name rather than as the new reserved
		// word.
		if len(traversal) < 3 {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid reference",
				Detail:   `The "resource" object must be followed by two attribute names: the resource type and the resource name.`,
				Subject:  traversal.SourceRange().Ptr(),
			})
			return nil, diags
		}
		remain := traversal[1:] // trim off "resource" so we can use our shared resource reference parser
		return parseResourceRef(ManagedResourceMode, rootRange, remain)
	case "local":
		return parseSingleAttrRef(traversal, func(name string) Referenceable {
			return LocalValue{Name: name}
		})
	case "module":
		return parseModuleCallRef(traversal)
	case "path":
		return parseSingleAttrRef(traversal, func(name string) Referenceable {
			return PathAttr{Name: name}
		})
	case "self":
		return &Reference{
			Subject:     Self,
			SourceRange: tfdiags.SourceRangeFromHCL(rootRange),
			Remaining:   traversal[1:],
		}, diags
	case "terraform":
		return parseSingleAttrRef(traversal, func(name string) Referenceable {
			return NewTerraformAttr(IdentTerraform, name)
		})
	case "tofu":
		return parseSingleAttrRef(traversal, func(name string) Referenceable {
			return NewTerraformAttr(IdentTofu, name)
		})
	case "var":
		return parseSingleAttrRef(traversal, func(name string) Referenceable {
			return InputVariable{Name: name}
		})
	case "template", "lazy", "arg":
		// These names are all pre-emptively reserved in the hope of landing
		// some version of "template values" or "lazy expressions" feature
		// before the next opt-in language edition, but don't yet do anything.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reserved symbol name",
			Detail:   fmt.Sprintf("The symbol name %q is reserved for use in a future OpenTofu version. If you are using a provider that already uses this as a resource type name, add the prefix \"resource.\" to force interpretation as a resource type name.", root),
			Subject:  rootRange.Ptr(),
		})
		return nil, diags
	default:
		function := ParseFunction(root)
		if function.IsNamespace(FunctionNamespaceProvider) {
			pf, err := function.AsProviderFunction()
			if err != nil {
				return nil, diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Unable to parse provider function",
					Detail:   err.Error(),
					Subject:  rootRange.Ptr(),
				})
			}
			return &Reference{
				Subject:     pf,
				SourceRange: tfdiags.SourceRangeFromHCL(rootRange),
			}, diags
		}
		return parseResourceRef(ManagedResourceMode, rootRange, traversal)
	}
}

func parseResourceRef(mode ResourceMode, startRange hcl.Range, traversal hcl.Traversal) (*Reference, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if len(traversal) < 2 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   `A reference to a resource type must be followed by at least one attribute access, specifying the resource name.`,
			Subject:  hcl.RangeBetween(traversal[0].SourceRange(), traversal[len(traversal)-1].SourceRange()).Ptr(),
		})
		return nil, diags
	}

	var typeName, name string
	switch tt := traversal[0].(type) { // Could be either root or attr, depending on our resource mode
	case hcl.TraverseRoot:
		typeName = tt.Name
	case hcl.TraverseAttr:
		typeName = tt.Name
	default:
		// If it isn't a TraverseRoot then it must be a "data" reference.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   `The "data" object does not support this operation.`,
			Subject:  traversal[0].SourceRange().Ptr(),
		})
		return nil, diags
	}

	attrTrav, ok := traversal[1].(hcl.TraverseAttr)
	if !ok {
		var what string
		switch mode {
		case DataResourceMode:
			what = "a data source"
		case EphemeralResourceMode:
			what = "an ephemeral resource"
		default:
			what = "a resource type"
		}
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   fmt.Sprintf(`A reference to %s must be followed by at least one attribute access, specifying the resource name.`, what),
			Subject:  traversal[1].SourceRange().Ptr(),
		})
		return nil, diags
	}
	name = attrTrav.Name
	rng := hcl.RangeBetween(startRange, attrTrav.SrcRange)
	remain := traversal[2:]

	resourceAddr := Resource{
		Mode: mode,
		Type: typeName,
		Name: name,
	}
	resourceInstAddr := ResourceInstance{
		Resource: resourceAddr,
		Key:      NoKey,
	}

	if len(remain) == 0 {
		// This might actually be a reference to the collection of all instances
		// of the resource, but we don't have enough context here to decide
		// so we'll let the caller resolve that ambiguity.
		return &Reference{
			Subject:     resourceAddr,
			SourceRange: tfdiags.SourceRangeFromHCL(rng),
		}, diags
	}

	if idxTrav, ok := remain[0].(hcl.TraverseIndex); ok {
		var err error
		resourceInstAddr.Key, err = ParseInstanceKey(idxTrav.Key)
		if err != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid index key",
				Detail:   fmt.Sprintf("Invalid index for resource instance: %s.", err),
				Subject:  &idxTrav.SrcRange,
			})
			return nil, diags
		}
		remain = remain[1:]
		rng = hcl.RangeBetween(rng, idxTrav.SrcRange)
	}

	return &Reference{
		Subject:     resourceInstAddr,
		SourceRange: tfdiags.SourceRangeFromHCL(rng),
		Remaining:   remain,
	}, diags
}

func parseModuleCallRef(traversal hcl.Traversal) (*Reference, tfdiags.Diagnostics) {
	// The following is a little circuitous just so we can reuse parseSingleAttrRef
	// for this slightly-odd case while keeping it relatively simple for all of the
	// other cases that use it: we first get the information we need wrapped up
	// in a *Reference and then unpack it to perform further work below.
	callRef, diags := parseSingleAttrRef(traversal, func(name string) Referenceable {
		return ModuleCallInstance{
			Call: ModuleCall{
				Name: name,
			},
			Key: NoKey,
		}
	})
	if diags.HasErrors() {
		return nil, diags
	}

	// A traversal starting with "module" can either be a reference to an
	// entire module, or to a single output from a module instance,
	// depending on what we find after this introducer.
	callInstance := callRef.Subject.(ModuleCallInstance) //nolint:errcheck // This was constructed directly above by call to parseSingleAttrRef
	callRange := callRef.SourceRange
	remain := callRef.Remaining

	if len(remain) == 0 {
		// Reference to an entire module. Might alternatively be a
		// reference to a single instance of a particular module, but the
		// caller will need to deal with that ambiguity since we don't have
		// enough context here.
		return &Reference{
			Subject:     callInstance.Call,
			SourceRange: callRange,
			Remaining:   remain,
		}, diags
	}

	if idxTrav, ok := remain[0].(hcl.TraverseIndex); ok {
		var err error
		callInstance.Key, err = ParseInstanceKey(idxTrav.Key)
		if err != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid index key",
				Detail:   fmt.Sprintf("Invalid index for module instance: %s.", err),
				Subject:  &idxTrav.SrcRange,
			})
			return nil, diags
		}
		remain = remain[1:]

		if len(remain) == 0 {
			// Also a reference to an entire module instance, but we have a key
			// now.
			return &Reference{
				Subject:     callInstance,
				SourceRange: tfdiags.SourceRangeFromHCL(hcl.RangeBetween(callRange.ToHCL(), idxTrav.SrcRange)),
				Remaining:   remain,
			}, diags
		}
	}

	if attrTrav, ok := remain[0].(hcl.TraverseAttr); ok {
		remain = remain[1:]
		return &Reference{
			Subject: ModuleCallInstanceOutput{
				Name: attrTrav.Name,
				Call: callInstance,
			},
			SourceRange: tfdiags.SourceRangeFromHCL(hcl.RangeBetween(callRange.ToHCL(), attrTrav.SrcRange)),
			Remaining:   remain,
		}, diags
	}

	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid reference",
		Detail:   "Module instance objects do not support this operation.",
		Subject:  remain[0].SourceRange().Ptr(),
	})
	return nil, diags
}

func parseSingleAttrRef(traversal hcl.Traversal, makeAddr func(name string) Referenceable) (*Reference, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	root := traversal.RootName()
	rootRange := traversal[0].SourceRange()

	// NOTE: In a previous version of this file parseSingleAttrRef only returned the component parts
	// of a *Reference and then the callers assembled them, which caused the main parseRef function
	// to return a non-nil result (with mostly-garbage field values) even in the error cases.
	// We've preserved that oddity for now because our code complexity refactoring efforts should
	// not change the externally-observable behavior, but to guarantee that we'd need to review
	// all uses of parseRef to make sure that they aren't depending on getting a non-nil *Reference
	// along with error diagnostics. :(

	if len(traversal) < 2 {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   fmt.Sprintf("The %q object cannot be accessed directly. Instead, access one of its attributes.", root),
			Subject:  &rootRange,
		})
		return &Reference{Subject: makeAddr("")}, diags
	}
	if attrTrav, ok := traversal[1].(hcl.TraverseAttr); ok {
		subjectAddr := makeAddr(attrTrav.Name)
		return &Reference{
			Subject:     subjectAddr,
			SourceRange: tfdiags.SourceRangeFromHCL(hcl.RangeBetween(rootRange, attrTrav.SrcRange)),
			Remaining:   traversal[2:],
		}, diags
	}
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid reference",
		Detail:   fmt.Sprintf("The %q object does not support this operation.", root),
		Subject:  traversal[1].SourceRange().Ptr(),
	})
	return &Reference{Subject: makeAddr("")}, diags
}
