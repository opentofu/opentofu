// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/dynblock"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/blocktoattr"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// ExpandBlock expands any "dynamic" blocks present in the given body. The
// result is a body with those blocks expanded, ready to be evaluated with
// EvalBlock.
//
// If the returned diagnostics contains errors then the result may be
// incomplete or invalid.
func (s *Scope) ExpandBlock(body hcl.Body, schema *configschema.Block) (hcl.Body, tfdiags.Diagnostics) {
	spec := schema.DecoderSpec()

	traversals := dynblock.ExpandVariablesHCLDec(body, spec)
	refs, diags := References(s.ParseRef, traversals)

	ctx, ctxDiags := s.EvalContext(refs)
	diags = diags.Append(ctxDiags)

	return dynblock.Expand(body, ctx), diags
}

// EvalBlock evaluates the given body using the given block schema and returns
// a cty object value representing its contents. The type of the result conforms
// to the implied type of the given schema.
//
// This function does not automatically expand "dynamic" blocks within the
// body. If that is desired, first call the ExpandBlock method to obtain
// an expanded body to pass to this method.
//
// If the returned diagnostics contains errors then the result may be
// incomplete or invalid.
func (s *Scope) EvalBlock(body hcl.Body, schema *configschema.Block) (cty.Value, tfdiags.Diagnostics) {
	spec := schema.DecoderSpec()

	refs, diags := ReferencesInBlock(s.ParseRef, body, schema)

	ctx, ctxDiags := s.EvalContext(refs)
	diags = diags.Append(ctxDiags)
	if diags.HasErrors() {
		// We'll stop early if we found problems in the references, because
		// it's likely evaluation will produce redundant copies of the same errors.
		return cty.UnknownVal(schema.ImpliedType()), diags
	}

	// HACK: In order to remain compatible with some assumptions made in
	// Terraform v0.11 and earlier about the approximate equivalence of
	// attribute vs. block syntax, we do a just-in-time fixup here to allow
	// any attribute in the schema that has a list-of-objects or set-of-objects
	// kind to potentially be populated instead by one or more nested blocks
	// whose type is the attribute name.
	body = blocktoattr.FixUpBlockAttrs(body, schema)

	val, evalDiags := hcldec.Decode(body, spec, ctx)
	diags = diags.Append(s.enhanceFunctionDiags(evalDiags))

	return val, diags
}

// EvalSelfBlock evaluates the given body only within the scope of the provided
// object and instance key data. References to the object must use self, and the
// key data will only contain count.index or each.key. The static values for
// terraform and path will also be available in this context.
func (s *Scope) EvalSelfBlock(body hcl.Body, self cty.Value, schema *configschema.Block, keyData instances.RepetitionData) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	spec := schema.DecoderSpec()

	vals := make(map[string]cty.Value)
	vals["self"] = self

	if !keyData.CountIndex.IsNull() {
		vals["count"] = cty.ObjectVal(map[string]cty.Value{
			"index": keyData.CountIndex,
		})
	}
	if !keyData.EachKey.IsNull() {
		vals["each"] = cty.ObjectVal(map[string]cty.Value{
			"key": keyData.EachKey,
		})
	}

	refs, refDiags := References(s.ParseRef, hcldec.Variables(body, spec))
	diags = diags.Append(refDiags)

	terraformAttrs := map[string]cty.Value{}
	pathAttrs := map[string]cty.Value{}

	// We could always load the static values for Path and Terraform values,
	// but we want to parse the references so that we can get source ranges for
	// user diagnostics.
	for _, ref := range refs {
		// we already loaded the self value
		if ref.Subject == addrs.Self {
			continue
		}

		switch subj := ref.Subject.(type) {
		case addrs.PathAttr:
			val, valDiags := normalizeRefValue(s.Data.GetPathAttr(subj, ref.SourceRange))
			diags = diags.Append(valDiags)
			pathAttrs[subj.Name] = val

		case addrs.TerraformAttr:
			val, valDiags := normalizeRefValue(s.Data.GetTerraformAttr(subj, ref.SourceRange))
			diags = diags.Append(valDiags)
			terraformAttrs[subj.Name] = val

		case addrs.CountAttr, addrs.ForEachAttr:
			// each and count have already been handled.

		default:
			// This should have been caught in validation, but point the user
			// to the correct location in case something slipped through.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  `Invalid reference`,
				Detail:   fmt.Sprintf("The reference to %q is not valid in this context", ref.Subject),
				Subject:  ref.SourceRange.ToHCL().Ptr(),
			})
		}
	}

	vals["path"] = cty.ObjectVal(pathAttrs)
	vals["terraform"] = cty.ObjectVal(terraformAttrs)
	vals["tofu"] = cty.ObjectVal(terraformAttrs)

	ctx := &hcl.EvalContext{
		Variables: vals,
		// TODO consider if any provider functions make sense here
		Functions: s.Functions(),
	}

	val, decDiags := hcldec.Decode(body, schema.DecoderSpec(), ctx)
	diags = diags.Append(s.enhanceFunctionDiags(decDiags))
	return val, diags
}

// EvalExpr evaluates a single expression in the receiving context and returns
// the resulting value. The value will be converted to the given type before
// it is returned if possible, or else an error diagnostic will be produced
// describing the conversion error.
//
// Pass an expected type of cty.DynamicPseudoType to skip automatic conversion
// and just obtain the returned value directly.
//
// If the returned diagnostics contains errors then the result may be
// incomplete, but will always be of the requested type.
func (s *Scope) EvalExpr(expr hcl.Expression, wantType cty.Type) (cty.Value, tfdiags.Diagnostics) {
	refs, diags := ReferencesInExpr(s.ParseRef, expr)

	ctx, ctxDiags := s.EvalContext(refs)
	diags = diags.Append(ctxDiags)
	if diags.HasErrors() {
		// We'll stop early if we found problems in the references, because
		// it's likely evaluation will produce redundant copies of the same errors.
		return cty.UnknownVal(wantType), diags
	}

	val, evalDiags := expr.Value(ctx)
	diags = diags.Append(s.enhanceFunctionDiags(evalDiags))

	if wantType != cty.DynamicPseudoType {
		var convErr error
		val, convErr = convert.Convert(val, wantType)
		if convErr != nil {
			val = cty.UnknownVal(wantType)
			diags = diags.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Incorrect value type",
				Detail:      fmt.Sprintf("Invalid expression value: %s.", tfdiags.FormatError(convErr)),
				Subject:     expr.Range().Ptr(),
				Expression:  expr,
				EvalContext: ctx,
			})
		}
	}

	return val, diags
}

// Identify and enhance any function related dialogs produced by a hcl.EvalContext
func (s *Scope) enhanceFunctionDiags(diags hcl.Diagnostics) hcl.Diagnostics {
	out := make(hcl.Diagnostics, len(diags))
	for i, diag := range diags {
		out[i] = diag

		if funcExtra, ok := diag.Extra.(hclsyntax.FunctionCallUnknownDiagExtra); ok {
			funcName := funcExtra.CalledFunctionName()
			// prefix::stuff::
			fullNamespace := funcExtra.CalledFunctionNamespace()

			if len(fullNamespace) == 0 {
				// Not a namespaced function, no enhancements necessary
				continue
			}

			// Insert the enhanced copy of diag into diags
			enhanced := *diag
			out[i] = &enhanced

			// Update enhanced with additional details

			fn := addrs.ParseFunction(fullNamespace + funcName)

			if fn.IsNamespace(addrs.FunctionNamespaceCore) {
				// Error is in core namespace, mirror non-core equivalent
				enhanced.Summary = "Call to unknown function"
				enhanced.Detail = fmt.Sprintf("There is no builtin (%s::) function named %q.", addrs.FunctionNamespaceCore, funcName)
			} else if fn.IsNamespace(addrs.FunctionNamespaceProvider) {
				if _, err := fn.AsProviderFunction(); err != nil {
					// complete mismatch or invalid prefix
					enhanced.Summary = "Invalid function format"
					enhanced.Detail = err.Error()
				}
			} else {
				enhanced.Summary = "Unknown function namespace"
				enhanced.Detail = fmt.Sprintf("Function %q does not exist within a valid namespace (%s)", fn, strings.Join(addrs.FunctionNamespaces, ","))
			}
			// Function / Provider not found handled by eval_context_builtin.go
		}
	}
	return out
}

// EvalReference evaluates the given reference in the receiving scope and
// returns the resulting value. The value will be converted to the given type before
// it is returned if possible, or else an error diagnostic will be produced
// describing the conversion error.
//
// Pass an expected type of cty.DynamicPseudoType to skip automatic conversion
// and just obtain the returned value directly.
//
// If the returned diagnostics contains errors then the result may be
// incomplete, but will always be of the requested type.
func (s *Scope) EvalReference(ref *addrs.Reference, wantType cty.Type) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// We cheat a bit here and just build an EvalContext for our requested
	// reference with the "self" address overridden, and then pull the "self"
	// result out of it to return.
	ctx, ctxDiags := s.evalContext(nil, []*addrs.Reference{ref}, ref.Subject)
	diags = diags.Append(ctxDiags)
	val := ctx.Variables["self"]
	if val == cty.NilVal {
		val = cty.DynamicVal
	}

	var convErr error
	val, convErr = convert.Convert(val, wantType)
	if convErr != nil {
		val = cty.UnknownVal(wantType)
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Incorrect value type",
			Detail:   fmt.Sprintf("Invalid expression value: %s.", tfdiags.FormatError(convErr)),
			Subject:  ref.SourceRange.ToHCL().Ptr(),
		})
	}

	return val, diags
}

// EvalContext constructs a HCL expression evaluation context whose variable
// scope contains sufficient values to satisfy the given set of references.
//
// Most callers should prefer to use the evaluation helper methods that
// this type offers, but this is here for less common situations where the
// caller will handle the evaluation calls itself.
func (s *Scope) EvalContext(refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
	return s.evalContext(nil, refs, s.SelfAddr)
}

// EvalContextWithParent is exactly the same as EvalContext except the resulting hcl.EvalContext
// will be derived from the given parental hcl.EvalContext. It will enable different hcl mechanisms
// to iteratively lookup target functions and variables in EvalContext's parent.
// See Traversal.TraverseAbs (hcl) or FunctionCallExpr.Value (hcl/hclsyntax) for more details.
func (s *Scope) EvalContextWithParent(p *hcl.EvalContext, refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics) {
	return s.evalContext(p, refs, s.SelfAddr)
}

func (s *Scope) evalContext(parent *hcl.EvalContext, refs []*addrs.Reference, selfAddr addrs.Referenceable) (*hcl.EvalContext, tfdiags.Diagnostics) {
	if s == nil {
		panic("attempt to construct EvalContext for nil Scope")
	}

	var diags tfdiags.Diagnostics

	// Calling NewChild() on a nil parent will
	// produce an EvalContext with no parent.
	ctx := parent.NewChild()
	ctx.Functions = make(map[string]function.Function)
	ctx.Variables = make(map[string]cty.Value)

	for name, fn := range s.Functions() {
		ctx.Functions[name] = fn
	}

	// Easy path for common case where there are no references at all.
	if len(refs) == 0 {
		return ctx, diags
	}

	// First we'll do static validation of the references. This catches things
	// early that might otherwise not get caught due to unknown values being
	// present in the scope during planning.
	staticDiags := s.Data.StaticValidateReferences(refs, selfAddr, s.SourceAddr)
	diags = diags.Append(staticDiags)
	if staticDiags.HasErrors() {
		return ctx, diags
	}

	// The reference set we are given has not been de-duped, and so there can
	// be redundant requests in it for two reasons:
	//  - The same item is referenced multiple times
	//  - Both an item and that item's container are separately referenced.
	// We will still visit every reference here and ask our data source for
	// it, since that allows us to gather a full set of any errors and
	// warnings, but once we've gathered all the data we'll then skip anything
	// that's redundant in the process of populating our values map.
	varBuilder := s.newEvalVarBuilder()

	for _, ref := range refs {
		if ref.Subject == addrs.Self {
			diags.Append(varBuilder.putSelfValue(selfAddr, ref))
			continue
		}

		if subj, ok := ref.Subject.(addrs.ProviderFunction); ok {
			// Inject function directly into context
			if _, ok := ctx.Functions[subj.String()]; !ok {
				fn, fnDiags := s.ProviderFunctions(subj, ref.SourceRange)
				diags = diags.Append(fnDiags)

				if !fnDiags.HasErrors() {
					ctx.Functions[subj.String()] = *fn
				}
			}

			continue
		}

		diags = diags.Append(varBuilder.putValueBySubject(ref))
	}

	varBuilder.buildAllVariablesInto(ctx.Variables)

	return ctx, diags
}

type evalVarBuilder struct {
	s *Scope

	dataResources    map[string]map[string]cty.Value
	managedResources map[string]map[string]cty.Value
	wholeModules     map[string]cty.Value
	inputVariables   map[string]cty.Value
	localValues      map[string]cty.Value
	outputValues     map[string]cty.Value
	pathAttrs        map[string]cty.Value
	terraformAttrs   map[string]cty.Value
	countAttrs       map[string]cty.Value
	forEachAttrs     map[string]cty.Value
	checkBlocks      map[string]cty.Value
	self             cty.Value
}

func (s *Scope) newEvalVarBuilder() *evalVarBuilder {
	return &evalVarBuilder{
		s: s,

		dataResources:    map[string]map[string]cty.Value{},
		managedResources: map[string]map[string]cty.Value{},
		wholeModules:     map[string]cty.Value{},
		inputVariables:   map[string]cty.Value{},
		localValues:      map[string]cty.Value{},
		outputValues:     map[string]cty.Value{},
		pathAttrs:        map[string]cty.Value{},
		terraformAttrs:   map[string]cty.Value{},
		countAttrs:       map[string]cty.Value{},
		forEachAttrs:     map[string]cty.Value{},
		checkBlocks:      map[string]cty.Value{},
	}
}

func (b *evalVarBuilder) putSelfValue(selfAddr addrs.Referenceable, ref *addrs.Reference) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	if selfAddr == nil {
		return diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  `Invalid "self" reference`,
			// This detail message mentions some current practice that
			// this codepath doesn't really "know about". If the "self"
			// object starts being supported in more contexts later then
			// we'll need to adjust this message.
			Detail:  `The "self" object is not available in this context. This object can be used only in resource provisioner, connection, and postcondition blocks.`,
			Subject: ref.SourceRange.ToHCL().Ptr(),
		})
	}

	if selfAddr == addrs.Self {
		// Programming error: the self address cannot alias itself.
		panic("scope SelfAddr attempting to alias itself")
	}

	// self can only be used within a resource instance
	subj, ok := selfAddr.(addrs.ResourceInstance)
	if !ok {
		panic("BUG: self addr must be a resource instance, got " + reflect.TypeOf(selfAddr).String())
	}

	val, valDiags := normalizeRefValue(b.s.Data.GetResource(subj.ContainingResource(), ref.SourceRange))

	diags = diags.Append(valDiags)

	// Self is an exception in that it must always resolve to a
	// particular instance. We will still insert the full resource into
	// the context below.
	var hclDiags hcl.Diagnostics
	// We should always have a valid self index by this point, but in
	// the case of an error, self may end up as a cty.DynamicValue.
	switch k := subj.Key.(type) {
	case addrs.IntKey:
		b.self, hclDiags = hcl.Index(val, cty.NumberIntVal(int64(k)), ref.SourceRange.ToHCL().Ptr())
	case addrs.StringKey:
		b.self, hclDiags = hcl.Index(val, cty.StringVal(string(k)), ref.SourceRange.ToHCL().Ptr())
	default:
		b.self = val
	}
	diags = diags.Append(hclDiags)

	return diags
}

func (b *evalVarBuilder) putValueBySubject(ref *addrs.Reference) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics

	rawSubj := ref.Subject
	rng := ref.SourceRange

	// This type switch must cover all of the "Referenceable" implementations
	// in package addrs, however we are removing the possibility of
	// Instances beforehand.
	switch addr := rawSubj.(type) {
	case addrs.ResourceInstance:
		rawSubj = addr.ContainingResource()
	case addrs.ModuleCallInstance:
		rawSubj = addr.Call
	case addrs.ModuleCallInstanceOutput:
		rawSubj = addr.Call.Call
	}

	var normDiags tfdiags.Diagnostics

	switch subj := rawSubj.(type) {
	case addrs.Resource:
		diags = diags.Append(b.putResourceValue(subj, rng))

	case addrs.ModuleCall:
		b.wholeModules[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetModule(subj, rng))

	case addrs.InputVariable:
		b.inputVariables[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetInputVariable(subj, rng))

	case addrs.LocalValue:
		b.localValues[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetLocalValue(subj, rng))

	case addrs.PathAttr:
		b.pathAttrs[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetPathAttr(subj, rng))

	case addrs.TerraformAttr:
		b.terraformAttrs[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetTerraformAttr(subj, rng))

	case addrs.CountAttr:
		b.countAttrs[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetCountAttr(subj, rng))

	case addrs.ForEachAttr:
		b.forEachAttrs[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetForEachAttr(subj, rng))

	case addrs.OutputValue:
		b.outputValues[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetOutput(subj, rng))

	case addrs.Check:
		b.outputValues[subj.Name], normDiags = normalizeRefValue(b.s.Data.GetCheckBlock(subj, rng))

	default:
		// Should never happen
		panic(fmt.Errorf("Scope.buildEvalContext cannot handle address type %T", rawSubj))
	}

	diags = diags.Append(normDiags)

	return diags
}

func (b *evalVarBuilder) putResourceValue(res addrs.Resource, rng tfdiags.SourceRange) tfdiags.Diagnostics {
	var into map[string]map[string]cty.Value

	switch res.Mode {
	case addrs.ManagedResourceMode:
		into = b.managedResources
	case addrs.DataResourceMode:
		into = b.dataResources
	case addrs.InvalidResourceMode:
		panic("BUG: got invalid resource mode")
	default:
		panic(fmt.Errorf("BUG: got undefined ResourceMode %s", res.Mode))
	}

	val, diags := normalizeRefValue(b.s.Data.GetResource(res, rng))

	if into[res.Type] == nil {
		into[res.Type] = make(map[string]cty.Value)
	}
	into[res.Type][res.Name] = val

	return diags
}

func (b *evalVarBuilder) buildAllVariablesInto(vals map[string]cty.Value) {
	// Managed resources are exposed in two different locations. The primary
	// is at the top level where the resource type name is the root of the
	// traversal, but we also expose them under "resource" as an escaping
	// technique if we add a reserved name in a future language edition which
	// conflicts with someone's existing provider.
	for k, v := range buildResourceObjects(b.managedResources) {
		vals[k] = v
	}
	vals["resource"] = cty.ObjectVal(buildResourceObjects(b.managedResources))

	vals["data"] = cty.ObjectVal(buildResourceObjects(b.dataResources))
	vals["module"] = cty.ObjectVal(b.wholeModules)
	vals["var"] = cty.ObjectVal(b.inputVariables)
	vals["local"] = cty.ObjectVal(b.localValues)
	vals["path"] = cty.ObjectVal(b.pathAttrs)
	vals["terraform"] = cty.ObjectVal(b.terraformAttrs)
	vals["tofu"] = cty.ObjectVal(b.terraformAttrs)
	vals["count"] = cty.ObjectVal(b.countAttrs)
	vals["each"] = cty.ObjectVal(b.forEachAttrs)

	// Checks and outputs are conditionally included in the available scope, so
	// we'll only write out their values if we actually have something for them.
	if len(b.checkBlocks) > 0 {
		vals["check"] = cty.ObjectVal(b.checkBlocks)
	}

	if len(b.outputValues) > 0 {
		vals["output"] = cty.ObjectVal(b.outputValues)
	}

	if b.self != cty.NilVal {
		vals["self"] = b.self
	}
}

func buildResourceObjects(resources map[string]map[string]cty.Value) map[string]cty.Value {
	vals := make(map[string]cty.Value)
	for typeName, nameVals := range resources {
		vals[typeName] = cty.ObjectVal(nameVals)
	}
	return vals
}

func normalizeRefValue(val cty.Value, diags tfdiags.Diagnostics) (cty.Value, tfdiags.Diagnostics) {
	if diags.HasErrors() {
		// If there are errors then we will force an unknown result so that
		// we can still evaluate and catch type errors but we'll avoid
		// producing redundant re-statements of the same errors we've already
		// dealt with here.
		return cty.UnknownVal(val.Type()), diags
	}
	return val, diags
}
