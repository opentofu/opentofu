// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0
package evalchecks

import (
	"fmt"
	"runtime"

	"github.com/hashicorp/hcl/v2"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

const (
	errInvalidUnknownDetailMap     = "The \"for_each\" map includes keys derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full set of keys that will identify the instances of this resource.\n\nWhen working with unknown values in for_each, it's better to define the map keys statically in your configuration and place apply-time results only in the map values.\n\n"
	errInvalidUnknownDetailSet     = "The \"for_each\" set includes values derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full set of keys that will identify the instances of this resource.\n\nWhen working with unknown values in for_each, it's better to use a map value where the keys are defined statically in your configuration and where only the values contain apply-time results.\n\n"
	errInvalidUnknownDetailTuple   = "The \"for_each\" tuple includes values derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full tuple of keys that will identify the instances of this resource.\n\nWhen working with unknown values in for_each, it's better to use a map value where the keys are defined statically in your configuration and where only the values contain apply-time results.\n\n"
	errInvalidUnknownDetailDynamic = "The \"for_each\" value includes keys or set values that cannot be determined until apply, and so OpenTofu cannot determine what will identify the instances of this resource.\n\nWhen working with unknown values in for_each, it's better to use a map value where the keys are defined statically in your configuration and where only the values contain apply-time results\n\n"
)

type ContextFunc func(refs []*addrs.Reference) (*hcl.EvalContext, tfdiags.Diagnostics)

// EvaluateForEachExpression is our standard mechanism for interpreting an
// expression given for a "for_each" argument on a resource or a module. This
// should be called during expansion in order to determine the final keys and
// values.
//
// EvaluateForEachExpression differs from EvaluateForEachExpressionValue by
// returning an error if the count value is not known, and converting the
// cty.Value to a map[string]cty.Value for compatibility with other calls.
//
// If excludableAddr is non-nil then the unknown value error will include
// an additional idea to exclude that address using the -exclude
// planning option to converge over multiple plan/apply rounds.
func EvaluateForEachExpression(expr hcl.Expression, ctx ContextFunc, excludableAddr addrs.Targetable) (map[string]cty.Value, tfdiags.Diagnostics) {
	const unknownsNotAllowed = false
	const tupleNotAllowed = false
	forEachVal, diags := EvaluateForEachExpressionValue(expr, ctx, unknownsNotAllowed, tupleNotAllowed, excludableAddr)
	// forEachVal might be unknown, but if it is then there should already
	// be an error about it in diags, which we'll return below.

	if forEachVal.IsNull() || !forEachVal.IsKnown() || markSafeLengthInt(forEachVal) == 0 {
		// we check length, because an empty set return a nil map
		return map[string]cty.Value{}, diags
	}

	return forEachVal.AsValueMap(), diags
}

// EvaluateForEachExpressionValue is like EvaluateForEachExpression
// except that it returns a cty.Value map or set which can be unknown.
// The 'allowTuple' argument is used to support evaluating for_each from tuple
// values, and is currently supported when using for_each in import blocks.
//
// If excludableAddr is non-nil then any unknown-value-related error will
// include an additional idea to exclude that address using the -exclude
// planning option to converge over multiple plan/apply rounds.
func EvaluateForEachExpressionValue(expr hcl.Expression, ctx ContextFunc, allowUnknown bool, allowTuple bool, excludableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	nullMap := cty.NullVal(cty.Map(cty.DynamicPseudoType))

	if expr == nil {
		return nullMap, diags
	}

	refs, moreDiags := lang.ReferencesInExpr(addrs.ParseRef, expr)
	diags = diags.Append(moreDiags)

	hclCtx, moreDiags := ctx(refs)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() { // Can't continue if we don't even have a valid scope
		return nullMap, diags
	}

	forEachVal, forEachDiags := expr.Value(hclCtx)
	diags = diags.Append(forEachDiags)

	// Normally, we'd add the type check diags here, but we are deferring that for ordering
	typeVal, typeCheckDiags := performTypeChecks(expr, hclCtx, allowUnknown, allowTuple, forEachVal)

	resultValue, valueDiags := performValueChecks(expr, hclCtx, allowUnknown, allowTuple, forEachVal, typeVal, excludableAddr)

	// See above comment
	diags = diags.Append(valueDiags)
	diags = diags.Append(typeCheckDiags)

	return resultValue, diags
}

// performTypeChecks checks if the type is valid and returns the proper value and the unknown message to be used depending on the type
func performTypeChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, allowUnknown bool, allowTuple bool, forEachVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	ty := forEachVal.Type()
	var diags tfdiags.Diagnostics

	allowedTypes := ty == cty.DynamicPseudoType || (ty.IsMapType() || ty.IsObjectType()) || ty.IsSetType()
	allowedTypesMessage := "map, or set of strings"
	if allowTuple {
		allowedTypes = allowedTypes || ty.IsTupleType()
		allowedTypesMessage = "map, set of strings, or a tuple"
	}

	if !allowedTypes {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      fmt.Sprintf(`The given "for_each" argument value is unsuitable: the "for_each" argument must be a %s, and you have provided a value of type %s.`, allowedTypesMessage, ty.FriendlyName()),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
		})
		return cty.NullVal(ty), diags
	}

	if ty.IsSetType() {
		setVal, setDiags := performSetTypeChecks(expr, hclCtx, allowUnknown, forEachVal)
		return setVal, setDiags
	}

	return forEachVal, diags
}

func performValueChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, allowUnknown bool, allowTuple bool, forEachVal cty.Value, typeCheckVal cty.Value, excludableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	resultVal := typeCheckVal
	ty := forEachVal.Type()
	var errInvalidUnknownDetail string

	// Perform top-level type checks
	switch {
	case ty == cty.DynamicPseudoType:
		errInvalidUnknownDetail = errInvalidUnknownDetailDynamic
	case ty.IsMapType() || ty.IsObjectType():
		errInvalidUnknownDetail = errInvalidUnknownDetailMap
	case ty.IsSetType():
		errInvalidUnknownDetail = errInvalidUnknownDetailSet
	case ty.IsTupleType() && allowTuple:
		errInvalidUnknownDetail = errInvalidUnknownDetailTuple
	default:
	}

	if ty.IsSetType() {
		setVal, setCheckDiags := performSetValueChecks(expr, hclCtx, resultVal)
		diags = diags.Append(setCheckDiags)
		resultVal = setVal
	}

	isUnderstoodType := errInvalidUnknownDetail != ""
	// Testing if the original value, the type check result or if the set check above are unknown
	if (!resultVal.IsKnown() || !forEachVal.IsKnown()) && isUnderstoodType {
		if !allowUnknown {
			diags = diags.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Invalid for_each argument",
				Detail:      errInvalidUnknownDetail + forEachCommandLineExcludeSuggestion(excludableAddr),
				Subject:     expr.Range().Ptr(),
				Expression:  expr,
				EvalContext: hclCtx,
				Extra:       DiagnosticCausedByUnknown(true),
			})
		}
		resultVal = cty.UnknownVal(ty)
	}

	// Check the original value for null
	if forEachVal.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      `The given "for_each" argument value is unsuitable: the given "for_each" argument value is null.`,
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
		})
		resultVal = cty.NullVal(ty)
	}

	// If a whole map is marked, or a set contains marked values (which means the set is then marked)
	// give an error diagnostic as this value cannot be used in for_each
	if forEachVal.HasMark(marks.Sensitive) {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      "Sensitive values, or values derived from sensitive values, cannot be used as for_each arguments. If used, the sensitive value could be exposed as a resource instance key.",
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
			Extra:       DiagnosticCausedBySensitive(true),
		})
		resultVal = cty.NullVal(ty)
	}

	return resultVal, diags
}

// performSetTypeChecks does checks when we have a Set type, as sets have some gotchas
func performSetTypeChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, allowUnknown bool, forEachVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	typeVal := forEachVal
	var diags tfdiags.Diagnostics
	ty := forEachVal.Type()

	// We do not accept sets containing other type than strings or if it can't be determined, we do accept it
	if ty.ElementType() != cty.String && ty.ElementType() != cty.DynamicPseudoType {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      fmt.Sprintf(`The given "for_each" argument value is unsuitable: "for_each" supports sets of strings, but you have provided a set containing type %s.`, forEachVal.Type().ElementType().FriendlyName()),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
		})
		typeVal = cty.NullVal(ty)
	}

	return typeVal, diags
}

func performSetValueChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, forEachVal cty.Value) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	// Since we're using a multi-error approach, we try to add as much of information as possible. The ElementIterator code below can't iterate on null or unknown values, that's why we test if these conditions are present and return earlier.
	// since we can't use a set values that are unknown, we treat the entire set as unknown
	ty := forEachVal.Type()
	if !forEachVal.IsWhollyKnown() {
		return cty.UnknownVal(ty), diags
	}

	if forEachVal.IsNull() {
		return forEachVal, diags
	}

	// A set of strings may contain null, which makes it impossible to
	// convert to a map, so we must return an error
	it := forEachVal.ElementIterator()
	for it.Next() {
		item, _ := it.Element()
		if item.IsNull() {
			diags = diags.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Invalid for_each argument",
				Detail:      `The given "for_each" argument value is unsuitable: "for_each" sets must not contain null values.`,
				Subject:     expr.Range().Ptr(),
				Expression:  expr,
				EvalContext: hclCtx,
			})
			return cty.NullVal(ty), diags
		}
	}
	return forEachVal, diags
}

// markSafeLengthInt allows calling LengthInt on marked values safely
func markSafeLengthInt(val cty.Value) int {
	v, _ := val.UnmarkDeep()
	return v.LengthInt()
}

// Returns some English-language text describing a workaround using the -exclude
// planning option to converge over two plan/apply rounds when for_each has an
// unknown value.
//
// This is intended only for when a for_each value is too unknown for
// planning to proceed, in [EvaluateForEachExpression] or [EvaluateForEachExpressionValue].
// The message always begins with "Alternatively, " because it's intended to be
// appended to one of either [errInvalidUnknownDetailMap] or [errInvalidUnknownDetailSet].
//
// If excludableAddr is non-nil then the message will refer to it directly, giving
// a full copy-pastable command line argument. Otherwise, the message is a generic
// one without any specific address indicated.
func forEachCommandLineExcludeSuggestion(excludableAddr addrs.Targetable) string {
	// We use an extra indirection here so that we can write tests that make
	// the same assertions on all development platforms.
	return forEachCommandLineExcludeSuggestionImpl(excludableAddr, runtime.GOOS)
}

func forEachCommandLineExcludeSuggestionImpl(excludableAddr addrs.Targetable, goos string) string {
	if excludableAddr == nil {
		// We use -target for this case because we can't be sure that the
		// object we're complaining about even has its own addrs.Targetable
		// address, and so the user might need to target only what it depends
		// on instead.
		return `Alternatively, you could use the -target option to first apply only the resources that for_each depends on, and then apply normally to converge.`
	}

	return fmt.Sprintf(
		`Alternatively, you could use the planning option -exclude=%s to first apply without this object, and then apply normally to converge.`,
		commandLineArgumentsSuggestion([]string{excludableAddr.String()}, goos),
	)
}
