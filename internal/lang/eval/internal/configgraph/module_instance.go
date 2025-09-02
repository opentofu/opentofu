// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"
	"strings"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ModuleInstance struct {
	// Any other kinds of "node" we add in future will likely need coverage
	// added in both [ModuleInstance.CheckAll] and
	// [ModuleInstance.AnnounceAllGraphevalRequests].
	InputVariableNodes map[addrs.InputVariable]*InputVariable
	LocalValueNodes    map[addrs.LocalValue]*LocalValue
	OutputValueNodes   map[addrs.OutputValue]*OutputValue
	ResourceNodes      map[addrs.Resource]*Resource

	CoreFunctions map[string]function.Function

	// moduleSourceAddr is the source address of the module this is an
	// instance of, which will be used as the base address for resolving
	// any relative local source addresses in child calls.
	//
	// This must always be either [addrs.ModuleSourceLocal] or
	// [addrs.ModuleSourceRemote]. If the module was discovered indirectly
	// through an [addrs.ModuleSourceRegistry] then this records the
	// remote address that the registry address was resolved to, to ensure
	// that local source addresses will definitely resolve within exactly
	// the same remote package.
	ModuleSourceAddr addrs.ModuleSource

	// callDeclRange is used for module instances that are produced because
	// of a "module" block in a parent module, or by some similar mechanism
	// like a .tftest.hcl "run" block, which can then be used as a source
	// range for the overall object value representing the module instance's
	// results.
	//
	// This is left as nil for module instances that are created implicitly,
	// such as a root module which is being "called" directly from OpenTofu CLI
	// in a command like "tofu plan".
	CallDeclRange *tfdiags.SourceRange
}

var _ exprs.Valuer = (*ModuleInstance)(nil)
var _ exprs.Scope = (*ModuleInstance)(nil)

// StaticCheckTraversal implements exprs.Valuer.
func (m *ModuleInstance) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	if len(traversal) == 0 {
		return nil // empty traversal is always valid
	}

	var diags tfdiags.Diagnostics

	// The Value representation of a module instance is an object with an
	// attribute for each output value, and so the first step traverses
	// through that first level of attributes.
	outputName, ok := exprs.TraversalStepAttributeName(traversal[0])
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid reference to output value",
			Detail:   "A module instance is represented by an object value whose attributes match the names of the output values declared inside the module.",
			Subject:  traversal[0].SourceRange().Ptr(),
		})
		return diags
	}

	output, ok := m.OutputValueNodes[addrs.OutputValue{Name: outputName}]
	if !ok {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to undeclared output value",
			Detail:   fmt.Sprintf("The child module does not declare any output value named %q.", outputName),
			Subject:  traversal[0].SourceRange().Ptr(),
		})
		return diags
	}
	diags = diags.Append(
		exprs.StaticCheckTraversalThroughType(traversal[1:], output.ResultTypeConstraint()),
	)
	return diags
}

// Value implements exprs.Valuer.
func (m *ModuleInstance) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// The following is mechanically similar to evaluating an object constructor
	// expression gathering all of the output value results into a single
	// object, but because we're not using the expression evaluator to do it
	// we need to explicitly discard indirect diagnostics with
	// [diagsHandledElsewhere].
	attrs := make(map[string]cty.Value, len(m.OutputValueNodes))
	for addr, ov := range m.OutputValueNodes {
		attrs[addr.Name] = diagsHandledElsewhere(ov.Value(ctx))
	}
	return cty.ObjectVal(attrs), nil
}

// ValueSourceRange implements exprs.Valuer.
func (m *ModuleInstance) ValueSourceRange() *tfdiags.SourceRange {
	return m.CallDeclRange
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

// ResolveAttr implements exprs.Scope.
func (m *ModuleInstance) ResolveAttr(ref hcl.TraverseAttr) (exprs.Attribute, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	switch ref.Name {

	case "var", "local", "module":
		// For various relatively-simple cases where there's just one-level of
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
		// by creating another [Scope] implementation which wraps this one,
		// handling these local symbols itself while delegating everything
		// else to [ModuleInstance.ResolveAttr] for handling as normal.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Reference to unavailable local symbol",
			Detail:   fmt.Sprintf("The symbol %q is not available in this location. It is available only locally in certain special parts of the language.", ref.Name),
			Subject:  &ref.SrcRange,
		})
		return nil, diags

	default:
		// TODO: Once we support resource references this case should be treated
		// as the beginning of a reference to a managed resource, as a
		// shorthand omitting the "resource." prefix.
		diags = diags.Append(fmt.Errorf("no support for %q references yet", ref.Name))
		return nil, diags
	}
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

// CheckAll visits this module and everything it contains to drive evaluation
// of all of the expressions in the configuration and collect any diagnostics
// they return.
//
// We can implement this as a just concurrent _tree_ walk rather than as a
// graph walk because the expression dependency relationships will get handled
// automatically behind the scenes as the different objects try to resolve
// their [OnceValuer] objects.
//
// This function, and the other downstream CheckAll methods it delegates to,
// therefore only need to worry about making sure that every blocking evaluation
// is happening in a separate goroutine so that the blocking calls can all
// resolve in whatever order makes sense for the dependency graph implied by the
// configuration.
func (m *ModuleInstance) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	// This method is an implementation of [allChecker], but we don't mention
	// that in the docs above because it's an unexported type that would
	// therefore be weird to mention in our exported docs.
	var cg checkGroup
	for _, n := range m.InputVariableNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range m.LocalValueNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range m.OutputValueNodes {
		cg.CheckChild(ctx, n)
	}
	for _, n := range m.ResourceNodes {
		cg.CheckChild(ctx, n)
	}
	return cg.Complete(ctx)
}

// AnnounceAllGraphevalRequests calls announce for each [grapheval.Once],
// [OnceValuer], or other [workgraph.RequestID] anywhere in the tree under this
// object.
//
// This is used only when [workgraph] detects a self-dependency or failure to
// resolve and we want to find a nice human-friendly name and optional source
// range to use to describe each of the requests that were involved in the
// problem.
func (m *ModuleInstance) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	// A ModuleInstance does not have any grapheval requests of its own,
	// but all of our child nodes might.
	for _, n := range m.InputVariableNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range m.LocalValueNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range m.OutputValueNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
	for _, n := range m.ResourceNodes {
		n.AnnounceAllGraphevalRequests(announce)
	}
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
	noun := nounForModuleGlobalSymbol(m.topSymbol)
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

func nounForModuleGlobalSymbol(symbol string) string {
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
