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
	errInvalidUnknownDetailMap   = "The \"for_each\" map includes keys derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full set of keys that will identify the instances of this resource.\n\nWhen working with unknown values in for_each, it's better to define the map keys statically in your configuration and place apply-time results only in the map values.\n\n"
	errInvalidUnknownDetailSet   = "The \"for_each\" set includes values derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full set of keys that will identify the instances of this resource.\n\nWhen working with unknown values in for_each, it's better to use a map value where the keys are defined statically in your configuration and where only the values contain apply-time results.\n\n"
	errInvalidUnknownDetailTuple = "The \"for_each\" tuple includes values derived from resource attributes that cannot be determined until apply, and so OpenTofu cannot determine the full tuple of keys that will identify the instances of this resource.\n\nWhen working with unknown values in for_each, it's better to use a map value where the keys are defined statically in your configuration and where only the values contain apply-time results.\n\n"
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
	var hclCtx *hcl.EvalContext
	hclCtx, moreDiags = ctx(refs)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() { // Can't continue if we don't even have a valid scope
		return nullMap, diags
	}

	forEachVal, forEachDiags := expr.Value(hclCtx)
	diags = diags.Append(forEachDiags)

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
	}

	var allowedTypesMessage string

	if allowTuple {
		allowedTypesMessage = "map, set of strings, or a tuple"
	} else {
		allowedTypesMessage = "map, or set of strings"
	}

	if forEachVal.IsNull() {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      fmt.Sprintf(`The given "for_each" argument value is unsuitable: the given "for_each" argument value is null. A %s is allowed.`, allowedTypesMessage),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
		})
	}

	// Type Checks
	ty := forEachVal.Type()
	var typeCheckVal cty.Value
	var typeCheckDiags tfdiags.Diagnostics
	switch {
	case ty.IsSetType():
		typeCheckVal, typeCheckDiags = performSetTypeChecks(expr, hclCtx, allowUnknown, forEachVal, excludableAddr)
	case ty.IsObjectType() || ty.IsMapType():
		typeCheckVal, typeCheckDiags = performMapTypeChecks(expr, hclCtx, allowUnknown, forEachVal, excludableAddr)
	case ty.IsTupleType() && allowTuple:
		// tuples are allowed for imports
		typeCheckVal, typeCheckDiags = performTupleTypeChecks(expr, hclCtx, allowUnknown, forEachVal, excludableAddr)
	case ty == cty.DynamicPseudoType:
		typeCheckVal, typeCheckDiags = performDynamicTypeChecks(expr, hclCtx, allowUnknown, forEachVal, excludableAddr)
	default:
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

	diags = diags.Append(typeCheckDiags)

	return typeCheckVal, diags
}

func performTupleTypeChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, allowUnknown bool, forEachVal cty.Value, excludableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if !forEachVal.IsKnown() && !allowUnknown {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      errInvalidUnknownDetailTuple + forEachCommandLineExcludeSuggestion(excludableAddr),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
			Extra:       DiagnosticCausedByUnknown(true),
		})
		return forEachVal, diags
	}

	return forEachVal, nil
}

func performDynamicTypeChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, allowUnknown bool, forEachVal cty.Value, excludableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if !forEachVal.IsKnown() && !allowUnknown {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      errInvalidUnknownDetailMap + forEachCommandLineExcludeSuggestion(excludableAddr),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
			Extra:       DiagnosticCausedByUnknown(true),
		})
		return forEachVal, diags
	}

	return forEachVal, nil
}

func performMapTypeChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, allowUnknown bool, forEachVal cty.Value, excludableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	if !forEachVal.IsKnown() && !allowUnknown {
		diags = diags.Append(&hcl.Diagnostic{
			Severity:    hcl.DiagError,
			Summary:     "Invalid for_each argument",
			Detail:      errInvalidUnknownDetailMap + forEachCommandLineExcludeSuggestion(excludableAddr),
			Subject:     expr.Range().Ptr(),
			Expression:  expr,
			EvalContext: hclCtx,
			Extra:       DiagnosticCausedByUnknown(true),
		})
		return forEachVal, diags
	}

	return forEachVal, nil
}

// performSetTypeChecks does checks when we have a Set type, as sets have some gotchas
func performSetTypeChecks(expr hcl.Expression, hclCtx *hcl.EvalContext, allowUnknown bool, forEachVal cty.Value, excludableAddr addrs.Targetable) (cty.Value, tfdiags.Diagnostics) {
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

	// since we can't use a set values that are unknown, we treat the
	// entire set as unknown
	if !forEachVal.IsWhollyKnown() {
		if !allowUnknown {
			diags = diags.Append(&hcl.Diagnostic{
				Severity:    hcl.DiagError,
				Summary:     "Invalid for_each argument",
				Detail:      errInvalidUnknownDetailSet + forEachCommandLineExcludeSuggestion(excludableAddr),
				Subject:     expr.Range().Ptr(),
				Expression:  expr,
				EvalContext: hclCtx,
				Extra:       DiagnosticCausedByUnknown(true),
			})
		}
		typeVal = cty.UnknownVal(ty)
	}

	// Since we're using a multi-error approach, we try to add as much of information as possible. The ElementIterator code below can't iterate on null or unknown values, that's why we are returning earlier.
	if typeVal.IsNull() || diags.HasErrors() || !typeVal.IsWhollyKnown() {
		return typeVal, diags
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
	return typeVal, diags
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
