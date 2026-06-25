// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu2024

import (
	"context"
	"fmt"
	"iter"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/didyoumean"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/eval/internal/evalglue"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/marks"
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
	inst              *CompiledModuleInstance
	coreFunctions     map[string]function.Function
	providerFunctions func(context.Context, addrs.ProviderFunction, hcl.Range) (function.Function, tfdiags.Diagnostics)

	// tofu attrs
	applying  bool
	workspace string

	// path attrs
	workingDir string
	rootDir    string
	sourceDir  string
}

var _ exprs.Scope = (*moduleInstanceScope)(nil)

// ResolveFunc implements exprs.Scope.
func (m *moduleInstanceScope) ResolveFunc(ctx context.Context, call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	parsed := addrs.ParseFunction(call.Name)
	// Ensure that there is at least one namespace (default core)
	parsed = parsed.FullyQualified()

	switch parsed.Namespaces[0] {
	case addrs.FunctionNamespaceCore:
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
	case addrs.FunctionNamespaceProvider:
		pf, err := parsed.AsProviderFunction()
		if err != nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid function format",
				Detail:   err.Error(),
				Subject:  &call.NameRange,
			})
			return function.Function{}, diags
		}
		if m.providerFunctions == nil {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Provider functions not supported here",
				Detail:   "Provider functions may only be used in specific contexts",
				Subject:  &call.NameRange,
			})
			return function.Function{}, diags
		}
		fn, moreDiags := m.providerFunctions(ctx, pf, call.NameRange)
		diags = diags.Append(moreDiags)
		return fn, diags
	default:
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unknown function namespace",
			Detail:   fmt.Sprintf("Function %q does not exist within a valid namespace (%s)", parsed, strings.Join(addrs.FunctionNamespaces, ",")),
			Subject:  &call.NameRange,
		})
		return function.Function{}, diags
	}
}

// ResolveAttr implements exprs.Scope.
func (m *moduleInstanceScope) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	switch ref.Name {

	case "var", "local", "module", "path", "terraform", "tofu":
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
	case "terraform", "tofu":
		var val cty.Value

		// Mostly copied from the tofu/evaluate.go
		switch ref.Name {
		case "applying":
			val = cty.BoolVal(m.applying).Mark(marks.Ephemeral)

		case "workspace":
			val = cty.StringVal(m.workspace)

		// case "env": intentionally removed as it has been deprecated for a *long* time.
		default:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Invalid %q attribute", topSymbol),
				Detail:   fmt.Sprintf(`The %q object does not have an attribute named %q. The only supported attributes are %s.workspace and %s.applying`, topSymbol, ref.Name, topSymbol, topSymbol),
				Subject:  &ref.SrcRange,
			})
		}

		if val == cty.NilVal {
			return nil, diags
		}
		return exprs.ValueOf(exprs.ConstantValuer(val)), diags
	case "path":
		var val cty.Value

		// Mostly copied from the tofu/evaluate.go
		switch ref.Name {

		case "cwd":
			val = cty.StringVal(filepath.ToSlash(m.workingDir))

		case "module":
			val = cty.StringVal(filepath.ToSlash(m.sourceDir))

		case "root":
			val = cty.StringVal(filepath.ToSlash(m.rootDir))

		default:
			suggestion := didyoumean.NameSuggestion(ref.Name, []string{"cwd", "module", "root"})
			if suggestion != "" {
				suggestion = fmt.Sprintf(" Did you mean %q?", suggestion)
			}
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Invalid "path" attribute`,
				Detail:   fmt.Sprintf(`The "path" object does not have an attribute named %q.%s`, ref.Name, suggestion),
				Subject:  &ref.SrcRange,
			})
		}

		if val == cty.NilVal {
			return nil, diags
		}
		return exprs.ValueOf(exprs.ConstantValuer(val)), diags
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
	case "terraform":
		return "terraform"
	case "tofu":
		return "tofu"
	case "path":
		return "path"
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
func (i *inputVariableValidationScope) ResolveFunc(ctx context.Context, call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	return i.parentScope.ResolveFunc(ctx, call)
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
func (i *instanceOverlayScope) ResolveFunc(ctx context.Context, call *hcl.StaticCall) (function.Function, tfdiags.Diagnostics) {
	// no extra functions in this local scope
	return i.parent.ResolveFunc(ctx, call)
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

// moduleCallResourceInstances is a shim that allows a caller that is holding
// an [exprs.Scope] whose concrete type is [*moduleInstanceScope] (or one of
// our several wrappers of it) to enumerate all of the resource instances
// declared beneath the given module call, using direct analysis of the compiled
// module structure instead of using expression evaluation.
//
// This function needs to be able to resolve instance selection expressions
// for any module calls and resources starting at the given call address and
// so the given context must have a valid grapheval worker.
//
// This is here to help deal with the special treatment we give to module call
// references in a depends_on argument, where we treat that as depending on
// every resource instance declared inside the module regardless of whether
// any of the output values depend on those resource instances. This is a quirky
// oddity we inherited from the old language runtime that we're preserving for
// backward-compatibility.
//
// If the given scope is not a known wrapper of [*moduleInstanceScope] then this
// just returns a zero-length sequence so that tests which don't involve this
// special behavior can safely use a testing-specific fake scope instead of the
// full scope implementation.
func moduleCallResourceInstancesDeep(ctx context.Context, scope exprs.Scope, callAddr addrs.ModuleCall) iter.Seq[*configgraph.ResourceInstance] {
	thisModuleInst := compiledModuleInstanceFromScope(scope)
	if thisModuleInst == nil {
		return func(yield func(*configgraph.ResourceInstance) bool) {}
	}

	return func(yield func(*configgraph.ResourceInstance) bool) {
		for _, calledModuleInst := range thisModuleInst.ChildModuleInstancesForCall(ctx, callAddr) {
			// TODO: using evalglue.ResourceInstancesDeep here is probably not right
			// because it immediately starts a new grapheval worker without also
			// starting a new goroutine. We need to figure out how the non-goroutine
			// coroutines started by for loops over iter.Seq fit in to the grapheval
			// rules: they have some characteristics in common with goroutines but
			// are not capable of running asynchronously with the calling goroutine.
			for resourceInst := range evalglue.ResourceInstancesDeep(ctx, calledModuleInst) {
				if !yield(resourceInst) {
					return
				}
			}
		}
	}
}

// moduleCallResourceInstances is a shim that allows a caller that is holding
// an [exprs.Scope] whose concrete type is [*moduleInstanceScope] (or one of
// our several wrappers of it) to enumerate all of the resource instances
// declared beneath the given module call instance, using direct analysis of
// the compiled module structure instead of using expression evaluation.
//
// This function needs to be able to resolve instance selection expressions
// for any module calls and resources starting at the given call address and
// so the given context must have a valid grapheval worker.
//
// This is here to help deal with the special treatment we give to module call
// references in a depends_on argument, where we treat that as depending on
// every resource instance declared inside the module regardless of whether
// any of the output values depend on those resource instances. This is a quirky
// oddity we inherited from the old language runtime that we're preserving for
// backward-compatibility.
//
// If the given scope is not a known wrapper of [*moduleInstanceScope] then this
// just returns a zero-length sequence so that tests which don't involve this
// special behavior can safely use a testing-specific fake scope instead of the
// full scope implementation.
func moduleCallInstanceResourceInstancesDeep(ctx context.Context, scope exprs.Scope, callInstAddr addrs.ModuleCallInstance) iter.Seq[*configgraph.ResourceInstance] {
	thisModuleInst := compiledModuleInstanceFromScope(scope)
	if thisModuleInst == nil {
		return func(yield func(*configgraph.ResourceInstance) bool) {}
	}

	calledModuleInst := thisModuleInst.ChildModuleInstance(ctx, callInstAddr)
	if calledModuleInst == nil {
		// No such instance then, and so no resource instances beneath it.
		return func(yield func(*configgraph.ResourceInstance) bool) {}
	}
	// TODO: using evalglue.ResourceInstancesDeep here is probably not right
	// because it immediately starts a new grapheval worker without also
	// starting a new goroutine. We need to figure out how the non-goroutine
	// coroutines started by for loops over iter.Seq fit in to the grapheval
	// rules: they have some characteristics in common with goroutines but
	// are not capable of running asynchronously with the calling goroutine.
	return evalglue.ResourceInstancesDeep(ctx, calledModuleInst)
}

// compiledModuleInstanceFromScope knows how to extract a
// *CompiledModuleInstance from the various concrete [exprs.Scope]
// implementations in this package.
//
// If the given scope is not one this function knows how to handle then it
// returns nil.
func compiledModuleInstanceFromScope(scope exprs.Scope) *CompiledModuleInstance {
	switch scope := scope.(type) {
	case *moduleInstanceScope:
		return scope.inst
	case *instanceOverlayScope:
		return compiledModuleInstanceFromScope(scope.parent)
	case *inputVariableValidationScope:
		return compiledModuleInstanceFromScope(scope.parentScope)
	default:
		return nil
	}
}
