// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// This file contains methods of [ModuleInstance] related to its
// implementation of [exprs.Scope], and other supporting functions and types
// used by module instances when acting in that role.
var _ exprs.Scope = (*ModuleInstance)(nil)

// ResolveFunc implements exprs.Scope.
func (m *ModuleInstance) ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if strings.Contains(call.Name, "::") {
		// TODO: Implement provider-defined functions, which use the
		// "provider::" prefix.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Call to unsupported function",
			Detail:   "This new experimental codepath doesn't support non-core functions yet.",
			Subject:  &call.NameRange,
		})
		return function.Function{}, diags
	}

	fn, ok := m.CoreFunctions[call.Name]
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Call to unsupported function",
			Detail:   fmt.Sprintf("There is no core function named %q in this version of OpenTofu.", call.Name),
			Subject:  &call.NameRange,
		})
		return function.Function{}, diags
	}

	return fn, diags
}

// ResolveAttr implements exprs.Scope.
func (m *ModuleInstance) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	switch ref.Name {

	case "var", "local", "module":
		// For various relatively-simple cases where there's just one level of
		// nested symbol table we use a single shared [exprs.SymbolTable]
		// implementation which then just delegates back to
		// [ModuleInstance.resolveSimpleChildAttr] once it has collected the
		// nested symbol name. Refer to that function for more details on these.
		return exprs.NestedSymbolTable(&moduleInstNestedSymbolTable{topSymbol: ref.Name, moduleInst: m}), diags

	case "each", "count", "self":
		// These symbols are not included in a module instance's global symbol
		// table at all, but we treat them as special here just so we can
		// return a different error message that implies that they are valid
		// in some other contexts even though they aren't valid here.
		//
		// Situations where these symbols _are_ available should be handled
		// by creating another [exprs.Scope] implementation which wraps this
		// one, handling these local symbols itself while delegating everything
		// else to this [ModuleInstance.ResolveAttr] for handling as normal.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to unavailable local symbol",
			Detail:   fmt.Sprintf("The symbol %q is not available in this location. It is available only locally in certain special parts of the language.", ref.Name),
			Subject:  &ref.SrcRange,
		})
		return nil, diags

		// All of these resource-related symbols ultimately end up in
		// [ModuleInstance.resolveResourceAttr] after indirecting through
		// one or two more attribute steps.
	case "resource":
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:       addrs.ManagedResourceMode,
			moduleInst: m,
			startRng:   ref.SrcRange,
		}), diags
	case "data":
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:       addrs.DataResourceMode,
			moduleInst: m,
		}), diags
	case "ephemeral":
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:       addrs.EphemeralResourceMode,
			moduleInst: m,
		}), diags
	default:
		// We treat all unrecognized prefixes as a shorthand for "resource."
		// where the first segment is the resource type name.
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:       addrs.ManagedResourceMode,
			typeName:   ref.Name,
			moduleInst: m,
		}), diags
	}
}

func (m *ModuleInstance) resolveResourceAttr(addr addrs.Resource, rng tfdiags.SourceRange) (exprs.Attribute, tfdiags.Diagnostics) {
	// This function handles references like "aws_instance.foo" and
	// "data.aws_subnet.bar" after the intermediate steps have been
	// collected using [moduleInstanceResourceSymbolTable]. Refer to
	// [ModuleInstance.ResourceAttr] for the beginning of this process.

	var diags tfdiags.Diagnostics
	r, ok := m.ResourceNodes[addr]
	if !ok {
		// TODO: Try using "didyoumean" with resource types and names that
		// _are_ declared in the module to see if we can suggest an alternatve.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared resource variable",
			Detail:   fmt.Sprintf("There is no declaration of resource %s in this module.", addr),
			Subject:  rng.ToHCL().Ptr(),
		})
		return nil, diags
	}
	return exprs.ValueOf(r), diags
}

func (m *ModuleInstance) resolveSimpleChildAttr(topSymbol string, ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// NOTE: This function only handles top-level symbol names which are
	// delegated to [moduleInstNestedSymbolTable] by
	// [ModuleInstance.ResolveAttr]. Some top-level symbol names are handled
	// separately and so intentionally not included in the following.
	switch topSymbol {

	case "var":
		v, ok := m.InputVariableNodes[addrs.InputVariable{Name: ref.Name}]
		if !ok {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reference to undeclared input variable",
				Detail:   fmt.Sprintf("There is no input variable named %q declared in this module.", ref.Name),
				Subject:  &ref.SrcRange,
			})
			return nil, diags
		}
		return exprs.ValueOf(v), diags

	case "local":
		v, ok := m.LocalValueNodes[addrs.LocalValue{Name: ref.Name}]
		if !ok {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reference to undeclared local value",
				Detail:   fmt.Sprintf("There is no local value named %q declared in this module.", ref.Name),
				Subject:  &ref.SrcRange,
			})
			return nil, diags
		}
		return exprs.ValueOf(v), diags

	case "module":
		// TODO: Handle this
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Module call references not yet supported",
			Detail:   "This experimental new implementation does not yet support referring to module calls.",
			Subject:  &ref.SrcRange,
		})
		return nil, diags

	default:
		// We should not get here because there should be a case above for
		// every symbol name that [ModuleInstance.ResolveAttr] delegates
		// to [moduleInstNestedSymbolTable].
		panic(fmt.Sprintf("missing handler for top-level symbol %q", topSymbol))
	}
}

// HandleInvalidStep implements exprs.Scope.
func (m *ModuleInstance) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	// We can't actually get here in normal use because this is a top-level
	// scope and HCL only allows attribute-shaped access to top-level symbols,
	// which would be handled by [ModuleInstance.ResolveAttr] instead.
	//
	// This is here primarily for completeness/robustness, but should be
	// reachable only in the presence of weird hand-written [hcl.Traversal]
	// values that could not be produced by the HCL parsers.
	var diags tfdiags.Diagnostics
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid global reference",
		Detail:   "Only static access to predeclared names is allowed in this scope.",
		Subject:  rng.ToHCL().Ptr(),
	})
	return diags
}

type moduleInstanceResourceSymbolTable struct {
	mode addrs.ResourceMode
	// We reuse this type for both the first step like "data." and the
	// second step like "data.foo.". typeName is the empty string for
	// the first step, and then populated in the second step.
	typeName   string
	moduleInst *ModuleInstance
	startRng   hcl.Range
}

var _ exprs.SymbolTable = (*moduleInstanceResourceSymbolTable)(nil)

// HandleInvalidStep implements exprs.SymbolTable.
func (m *moduleInstanceResourceSymbolTable) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if m.typeName == "" {
		// We're at the first step and expecting a resource type name, then.
		adjective := ""
		switch m.mode {
		case addrs.ManagedResourceMode:
			adjective = "managed "
		case addrs.DataResourceMode:
			adjective = "data "
		case addrs.EphemeralResourceMode:
			adjective = "ephemeral "
		default:
			// We'll just omit any adjective if it isn't one we know, though
			// we should ideally update the above if we add a new resource mode.
		}
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference to resource",
			Detail:   fmt.Sprintf("An attribute access is required here, naming the type of %sresource to refer to.", adjective),
			Subject:  rng.ToHCL().Ptr(),
		})
	} else {
		// We're at the second step and expecting a resource name.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference to resource",
			Detail:   fmt.Sprintf("An attribute access is required here, giving the name of the %q resource to refer to.", m.typeName),
			Subject:  rng.ToHCL().Ptr(),
		})
	}
	return diags
}

// ResolveAttr implements exprs.SymbolTable.
func (m *moduleInstanceResourceSymbolTable) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	if m.typeName == "" {
		// We're at the first step and expecting a resource type name, then.
		// We'll return a new instance with the given type name populated
		// so that we can collect the resource name from the next step.
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:       m.mode,
			typeName:   ref.Name,
			moduleInst: m.moduleInst,
			startRng:   m.startRng,
		}), diags
	}
	// We're at the second step and expecting a resource name. We'll now
	// delegate back to the main module instance to handle the reference.
	addr := addrs.Resource{
		Mode: m.mode,
		Type: m.typeName,
		Name: ref.Name,
	}
	return m.moduleInst.resolveResourceAttr(addr, tfdiags.SourceRangeFromHCL(hcl.RangeBetween(m.startRng, ref.SrcRange)))
}

// moduleInstNestedSymbolTable is a common implementation for all of the
// various "simple" nested symbol table prefixes in a module instance's
// top-level scope, handling the typical case where there's a fixed prefix
// symbol followed by a single child symbol, as in "var.foo".
//
// This does not handle more complicated cases like resource references
// where there are multiple levels of nesting. Refer to
// [ModuleInstance.ResolveAttr] to learn how each of the top-level symbols
// is handled, and what subset of them are handled by this type.
type moduleInstNestedSymbolTable struct {
	topSymbol  string
	moduleInst *ModuleInstance
}

var _ exprs.SymbolTable = (*moduleInstNestedSymbolTable)(nil)

// HandleInvalidStep implements exprs.SymbolTable.
func (m *moduleInstNestedSymbolTable) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	noun := nounForModuleInstanceGlobalSymbol(m.topSymbol)
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid reference to " + noun,
		Detail:   fmt.Sprintf("Reference to %s requires an attribute name.", noun),
		Subject:  rng.ToHCL().Ptr(),
	})
	return diags
}

// ResolveAttr implements exprs.SymbolTable.
func (m *moduleInstNestedSymbolTable) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	// Now we just delegate back to the original module instance, so that
	// we can keep all of the symbol-table-related code relatively close
	// together.
	return m.moduleInst.resolveSimpleChildAttr(m.topSymbol, ref)
}

func nounForModuleInstanceGlobalSymbol(symbol string) string {
	// This is a kinda-gross way to handle this. For example, it means that
	// callers generating error messages must use awkward grammar to avoid
	// dealing with "an input variable" vs "a local value".
	//
	// Can we find a better way while still reusing at least some code
	// between all of these relatively-simple symbol tables? Maybe it's
	// worth treating at least a few more of these as special just to
	// get some better error messages for the more common situations.
	switch symbol {
	case "var":
		return "input variable"
	case "local":
		return "local value"
	case "module":
		return "module call"
	default:
		return "attribute" // generic fallback that we should avoid using by adding new names above as needed
	}
}
