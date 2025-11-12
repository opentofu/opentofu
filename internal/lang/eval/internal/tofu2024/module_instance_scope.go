// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// The symbols in this file are what define the shape of the top-level symbol
// table in a module instance, containing symbols like "var", "local", "module",
// etc.
//
// This lives out here alongside the functionality in module_instance.go because
// the naming scheme used for these symbols is a surface-language-level concern
// that could vary based on language edition and language experiments, including
// potentially using different symbol table shapes in different modules of
// the same configuration, and so it seems thematically coupled to the
// logic for "compiling" configs.Module into the configgraph types, whereas
// configgraph tries to be relatively agnostic to the design of the surface
// syntax so that it can support configuration trees where different modules
// use different surface language design details.
//
// There's some related context about this in:
//    https://github.com/opentofu/opentofu/pull/2262

type moduleInstanceScope struct {
	inst          *CompiledModuleInstance
	coreFunctions map[string]function.Function

	// TODO: some way to interact with provider-defined functions too, but
	// that's tricky since OpenTofu decided to call them on _configured_
	// providers rather than unconfigured ones and this evaluator otherwise
	// only uses unconfigured providers... so I guess we'll need some sort of
	// upcall glue to ask whatever code is orchestrating the plan or apply
	// phase to call a function on our behalf, or similar, and arrange
	// for functions in the [ConfigInstance.PrepareToPlan] phase to return
	// marked values so we can detect the additional
	// resource-to-provider-instance dependencies those calls imply.
	//
	// (It seems unfortunate that this additional complexity only really
	// currently benefits the opentofu/lua provider, which doesn't seem
	// to be widely used. It would be far simpler if we could just always
	// call functions on the same unconfigured providers we're using for
	// schema fetching and config validation.)
}

var _ exprs.Scope = (*moduleInstanceScope)(nil)

// ResolveFunc implements exprs.Scope.
func (m *moduleInstanceScope) ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
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

	fn, ok := m.coreFunctions[call.Name]
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
func (m *moduleInstanceScope) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	switch ref.Name {

	case "var", "local", "module":
		// For various relatively-simple cases where there's just one level of
		// nested symbol table we use a single shared [exprs.SymbolTable]
		// implementation which then just delegates back to
		// [ModuleInstance.resolveSimpleChildAttr] once it has collected the
		// nested symbol name. Refer to that function for more details on these.
		return exprs.NestedSymbolTable(&moduleInstNestedSymbolTable{topSymbol: ref.Name, topScope: m}), diags

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
			mode:     addrs.ManagedResourceMode,
			topScope: m,
			startRng: ref.SrcRange,
		}), diags
	case "data":
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:     addrs.DataResourceMode,
			topScope: m,
		}), diags
	case "ephemeral":
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:     addrs.EphemeralResourceMode,
			topScope: m,
		}), diags
	default:
		// We treat all unrecognized prefixes as a shorthand for "resource."
		// where the first segment is the resource type name.
		return exprs.NestedSymbolTable(&moduleInstanceResourceSymbolTable{
			mode:     addrs.ManagedResourceMode,
			typeName: ref.Name,
			topScope: m,
		}), diags
	}
}

func (m *moduleInstanceScope) resolveResourceAttr(addr addrs.Resource, rng tfdiags.SourceRange) (exprs.Attribute, tfdiags.Diagnostics) {
	// This function handles references like "aws_instance.foo" and
	// "data.aws_subnet.bar" after the intermediate steps have been
	// collected using [moduleInstanceResourceSymbolTable]. Refer to
	// [ModuleInstance.ResourceAttr] for the beginning of this process.

	var diags tfdiags.Diagnostics
	r, ok := m.inst.resourceNodes[addr]
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

func (m *moduleInstanceScope) resolveSimpleChildAttr(topSymbol string, ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// NOTE: This function only handles top-level symbol names which are
	// delegated to [moduleInstNestedSymbolTable] by
	// [ModuleInstance.ResolveAttr]. Some top-level symbol names are handled
	// separately and so intentionally not included in the following.
	switch topSymbol {

	case "var":
		v, ok := m.inst.inputVariableNodes[addrs.InputVariable{Name: ref.Name}]
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
		v, ok := m.inst.localValueNodes[addrs.LocalValue{Name: ref.Name}]
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
		v, ok := m.inst.moduleCallNodes[addrs.ModuleCall{Name: ref.Name}]
		if !ok {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reference to undeclared module call",
				Detail:   fmt.Sprintf("There is no module call named %q declared in this module.", ref.Name),
				Subject:  &ref.SrcRange,
			})
			return nil, diags
		}
		return exprs.ValueOf(v), diags

	default:
		// We should not get here because there should be a case above for
		// every symbol name that [ModuleInstance.ResolveAttr] delegates
		// to [moduleInstNestedSymbolTable].
		panic(fmt.Sprintf("missing handler for top-level symbol %q", topSymbol))
	}
}

// HandleInvalidStep implements exprs.Scope.
func (m *moduleInstanceScope) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
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
	typeName string
	topScope *moduleInstanceScope
	startRng hcl.Range
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
			mode:     m.mode,
			typeName: ref.Name,
			topScope: m.topScope,
			startRng: m.startRng,
		}), diags
	}
	// We're at the second step and expecting a resource name. We'll now
	// delegate back to the main module instance to handle the reference.
	addr := addrs.Resource{
		Mode: m.mode,
		Type: m.typeName,
		Name: ref.Name,
	}
	return m.topScope.resolveResourceAttr(addr, tfdiags.SourceRangeFromHCL(hcl.RangeBetween(m.startRng, ref.SrcRange)))
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
	topSymbol string
	topScope  *moduleInstanceScope
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
	return m.topScope.resolveSimpleChildAttr(m.topSymbol, ref)
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

// inputVariableValidationScope is a specialized [exprs.Scope] implementation
// that forces returning a constant value when accessing a specific input
// variable directly, but otherwise just passes everything else through from
// a parent scope.
//
// This is used for evaluating validation rules for an [InputVariable], where
// we need to be able to evaluate an expression referring to the variable
// as part of deciding the final value of the variable and so if we didn't
// handle it directly then there would be a self-reference error.
type inputVariableValidationScope struct {
	varTable    exprs.SymbolTable
	wantName    string
	parentScope exprs.Scope
	finalVal    cty.Value
}

var _ exprs.Scope = (*inputVariableValidationScope)(nil)
var _ exprs.SymbolTable = (*inputVariableValidationScope)(nil)

// HandleInvalidStep implements exprs.Scope.
func (i *inputVariableValidationScope) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	return i.parentScope.HandleInvalidStep(rng)
}

// ResolveAttr implements exprs.Scope.
func (i *inputVariableValidationScope) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	if i.varTable == nil {
		// We're currently at the top-level scope where we're looking for
		// the "var." prefix to represent accessing any input variable at all.
		attr, diags := i.parentScope.ResolveAttr(ref)
		if diags.HasErrors() {
			return attr, diags
		}
		nestedTable := exprs.NestedSymbolTableFromAttribute(attr)
		if nestedTable != nil && ref.Name == "var" {
			// We'll return another instance of ourselves but with i.varTable
			// now populated to represent that the next step should try
			// to look up an input variable.
			return exprs.NestedSymbolTable(&inputVariableValidationScope{
				varTable:    nestedTable,
				wantName:    i.wantName,
				parentScope: i.parentScope,
				finalVal:    i.finalVal,
			}), diags
		}
		// If it's anything other than the "var" prefix then we'll just return
		// whatever the parent scope returned directly, because we don't
		// need to be involved anymore.
		return attr, diags
	}

	// If we get here then we're now nested under the "var." prefix, but
	// we only need to get involved if the reference is to the variable
	// we're currently validating.
	if ref.Name == i.wantName {
		return exprs.ValueOf(exprs.ConstantValuer(i.finalVal)), nil
	}
	return i.varTable.ResolveAttr(ref)
}

// ResolveFunc implements exprs.Scope.
func (i *inputVariableValidationScope) ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	return i.parentScope.ResolveFunc(call)
}

func instanceLocalScope(parentScope exprs.Scope, repData instances.RepetitionData) exprs.Scope {
	return &instanceOverlayScope{
		repData: repData,
		parent:  parentScope,
	}
}

type instanceOverlayScope struct {
	repData instances.RepetitionData
	parent  exprs.Scope
}

// HandleInvalidStep implements exprs.Scope.
func (i *instanceOverlayScope) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	return i.parent.HandleInvalidStep(rng)
}

// ResolveAttr implements exprs.Scope.
func (i *instanceOverlayScope) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	// NOTE: The error messages we return here make some assumptions about
	// what surface language features cause each of these fields to be
	// popualated, which is technically a layering violation because that's
	// the responsibility of whatever provided the [InstanceSelector] that
	// led us here, but we accept it for now out of pragmatism and will make
	// this more complex only if a future edition of the language significantly
	// changes how these things work.
	switch ref.Name {
	case "each":
		var diags tfdiags.Diagnostics
		if i.repData.EachKey == cty.NilVal {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reference to unavailable local symbol",
				Detail:   "The symbol \"each\" is available only when defining multiple instances using the \"for_each\" meta-argument.",
				Subject:  ref.SrcRange.Ptr(),
			})
			return nil, diags
		}
	case "count":
		var diags tfdiags.Diagnostics
		if i.repData.CountIndex == cty.NilVal {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Reference to unavailable local symbol",
				Detail:   "The symbol \"count\" is available only when defining multiple instances using the \"count\" meta-argument.",
				Subject:  ref.SrcRange.Ptr(),
			})
			return nil, diags
		}
	default:
		// Everything else is delegated to the parent scope.
		return i.parent.ResolveAttr(ref)
	}

	return exprs.NestedSymbolTable(&instanceLocalSymbolTable{
		repData:     i.repData,
		firstSymbol: ref.Name,
	}), nil
}

// ResolveFunc implements exprs.Scope.
func (i *instanceOverlayScope) ResolveFunc(call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	// no extra functions in this local scope
	return i.parent.ResolveFunc(call)
}

type instanceLocalSymbolTable struct {
	repData     instances.RepetitionData
	firstSymbol string
}

// HandleInvalidStep implements exprs.SymbolTable.
func (i *instanceLocalSymbolTable) HandleInvalidStep(rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	switch i.firstSymbol {
	case "each":
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   "The \"each\" object only has the attributes \"key\" and \"value\".",
			Subject:  rng.ToHCL().Ptr(),
		})
	case "count":
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   "The \"count\" object only has the attribute \"index\".",
			Subject:  rng.ToHCL().Ptr(),
		})
	default:
		// There aren't any other top-level symbols that should get delegated
		// into here, so this should be unreachable.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   "This reference is invalid, but we cannot explain why due to a bug in OpenTofu.",
			Subject:  rng.ToHCL().Ptr(),
		})
	}
	return diags
}

// ResolveAttr implements exprs.SymbolTable.
func (i *instanceLocalSymbolTable) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	switch i.firstSymbol {
	case "each":
		switch ref.Name {
		case "key":
			return exprs.ValueOf(exprs.ConstantValuer(i.repData.EachKey)), diags
		case "value":
			return exprs.ValueOf(exprs.ConstantValuer(i.repData.EachValue)), diags
		default:
			return nil, i.HandleInvalidStep(tfdiags.SourceRangeFromHCL(ref.SourceRange()))
		}
	case "count":
		switch ref.Name {
		case "index":
			return exprs.ValueOf(exprs.ConstantValuer(i.repData.CountIndex)), diags
		default:
			return nil, i.HandleInvalidStep(tfdiags.SourceRangeFromHCL(ref.SourceRange()))
		}
	default:
		// There aren't any other top-level symbols that should get delegated
		// into here, so this should be unreachable.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference",
			Detail:   "This reference is invalid, but we cannot explain why due to a bug in OpenTofu.",
			Subject:  &ref.SrcRange,
		})
		return nil, diags
	}
}
