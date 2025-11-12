// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lint

import (
	"iter"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/tfdiags"
)

// DiscardedObjectConstructorAttr is the result type for
// [DiscardedObjectConstructorAttrs].
type DiscardedObjectConstructorAttr struct {
	// Path is a description of the path from the top-level expression passed
	// to [DiscardedObjectConstructorAttrs] to the attribute that was defined
	// but would be discarded under type conversion.
	//
	// By definition the last step of this path will always be an attribute
	// step referring to an attribute that is not included in the target type.
	//
	// The steps before the final element of this path, if any, describe where
	// in a nested data structure the problematic definition appeared. This
	// is important to include somehow in a resulting error message to help
	// the reader understand which part of the data structure was affected.
	// These path steps can potentially include index steps with unknown key
	// values when a problem is being reported for all elements of a collection
	// at once. [tfdiags.FormatCtyPath] has support for index
	// steps with unknown values, so it's always valid to pass the value
	// of this field to that function.
	Path cty.Path

	// NameRange is the source range of the name part of the attribute
	// definition that is being reported.
	NameRange tfdiags.SourceRange

	// ContextRange is the source range of relevant context surrounding the
	// problematic attribute defition, intended to be used for the "Context"
	// field of a generated diagnostic so that its included source code
	// snippet will also include some surrounding lines that should indicate
	// what else was set in the relevant object constructor.
	ContextRange tfdiags.SourceRange

	// TargetType is the leaf object type that the affected object constructor
	// was building a value for. This is the type that the last element of
	// Path would traverse into, and which (by definition) does not have an
	// attribute corresponding to that final traversal step.
	TargetType cty.Type
}

// DiscardedObjectConstructorAttrs compares the given HCL expression with the
// given target type that the result of the expression would be converted to,
// and returns a sequence of attributes defined within object constructor
// expressions that would be immediately discarded during type conversion, and
// so are therefore effectively useless.
//
// A common cause for this to arise is someone making a typo of the attribute
// name when the attribute they were intending to define is optional and
// therefore this doesn't cause an error from the failure to define a required
// attribute.
//
// This function makes a best effort to work recursively through certain other
// HCL expression types to find problems even in nested object constructor
// expressions, but it's primarily focused on the most common case where a
// nested structure is written out literally using nested object and tuple
// constructor expressions, because other expression types are harder to
// analyze reliably with only static analysis.
func DiscardedObjectConstructorAttrs(expr hcl.Expression, targetTy cty.Type) iter.Seq[DiscardedObjectConstructorAttr] {
	return func(yield func(DiscardedObjectConstructorAttr) bool) {
		if targetTy.IsPrimitiveType() || targetTy == cty.DynamicPseudoType {
			// There's nothing useful we could do with these types, so we'll
			// just return early to reduce overhead.
			return
		}
		// We'll preallocate some capacity for extending path a few steps,
		// so we can avoid reallocating this unless the given structure is
		// particularly deep. We share the backing array of this buffer
		// across calls, so whenever a path is reported as part of a result
		// we must copy it first.
		path := make(cty.Path, 0, 4)
		yieldDiscardedObjectConstructorAttrs(expr, targetTy, path, yield)
	}
}

// yieldDiscardedObjectConstructorAttrs is the main recursive body of
// [DiscardedObjectConstructorAttrs], called for each relevant nested expression
// inside a given expression, starting with the topmost expression.
//
// Returns false if the caller should cease calling yield or passing yield to
// other functions that might call yield.
func yieldDiscardedObjectConstructorAttrs(expr hcl.Expression, targetTy cty.Type, path cty.Path, yield func(DiscardedObjectConstructorAttr) bool) bool {
	// What expression types are interesting depends on what type the caller
	// is intending to convert the expression result to.
	switch {
	case targetTy.IsObjectType():
		// This is our main case and the only one where we'll actually
		// potentially generate results directly, rather than just finding
		// more nested expressions to visit recursively.
		return yieldDiscardedObjectConstructorAttrsObject(expr, targetTy, path, yield)
	case targetTy.IsMapType():
		return yieldDiscardedObjectConstructorAttrsMap(expr, targetTy, path, yield)
	case targetTy.IsListType() || targetTy.IsSetType():
		// In both of cases we can potentially find nested expressions
		// to visit through either a tuple constructor or tuple-for expression.
		return yieldDiscardedObjectConstructorAttrsListLike(expr, targetTy, path, yield)
	case targetTy.IsTupleType():
		return yieldDiscardedObjectConstructorAttrsTuple(expr, targetTy, path, yield)
	default:
		// No other target types relate to an expression type we know how
		// to analyze for nested object constructor expressions.
		return true
	}
}

func yieldDiscardedObjectConstructorAttrsObject(expr hcl.Expression, targetTy cty.Type, path cty.Path, yield func(DiscardedObjectConstructorAttr) bool) bool {
	switch expr := expr.(type) {
	case *hclsyntax.ObjectConsExpr:
		// This is the main part of this overall analysis: we are checking for
		// any object constructor item which has a constant attribute name
		// that doesn't appear in the target type.
		for _, item := range expr.Items {
			keyStr, ok := attributeNameFromObjectConsKey(item.KeyExpr)
			if !ok {
				continue
			}
			if !targetTy.HasAttribute(keyStr) {
				// Because this particular path is going to be included in
				// our result, we need to make sure it has its own private
				// backing array separate from "path".
				retPath := make(cty.Path, len(path), len(path)+1)
				copy(retPath, path)
				retPath = append(retPath, cty.GetAttrStep{Name: keyStr})
				result := DiscardedObjectConstructorAttr{
					Path:         retPath,
					NameRange:    tfdiags.SourceRangeFromHCL(item.KeyExpr.Range()),
					ContextRange: tfdiags.SourceRangeFromHCL(expr.SrcRange), // the entire containing object literal
					TargetType:   targetTy,
				}
				if !yield(result) {
					return false
				}
			} else {
				// If the item _does_ correlate with an expected attribute
				// then we might be able to find more nested problems.
				elemTy := targetTy.AttributeType(keyStr)
				childPath := append(path, cty.GetAttrStep{Name: keyStr})
				if !yieldDiscardedObjectConstructorAttrs(item.ValueExpr, elemTy, childPath, yield) {
					return false
				}
			}
		}
		return true

	default:
		// We don't have anything useful to do with any other expression types.
		return true
	}
}

func yieldDiscardedObjectConstructorAttrsMap(expr hcl.Expression, targetTy cty.Type, path cty.Path, yield func(DiscardedObjectConstructorAttr) bool) bool {
	switch expr := expr.(type) {
	case *hclsyntax.ObjectConsExpr:
		// When a map is defined by conversion of an object constructor result,
		// we can recursively visit the definitions of any items that have
		// a known and constant key, using the map element type for all
		// attribute values because that's how the object attributes would
		// eventually be converted.
		elemTy := targetTy.ElementType()
		for _, item := range expr.Items {
			keyStr, ok := attributeNameFromObjectConsKey(item.KeyExpr)
			if !ok {
				continue
			}
			// Temporary extension of the path for our recursive call
			childPath := append(path, cty.IndexStep{Key: cty.StringVal(keyStr)})
			if !yieldDiscardedObjectConstructorAttrs(item.ValueExpr, elemTy, childPath, yield) {
				return false
			}
		}
		return true

	case *hclsyntax.ForExpr:
		if expr.KeyExpr == nil {
			// This is a tuple-for expression, which is not a valid way
			// to define a map and so we'll ignore it and let this fail
			// type conversion when the caller tries.
			return true
		}
		// In this case we're testing the result expression of the object-for
		// imagining that it will become a value for each element of the
		// resulting map, and therefore we can assume that any value that
		// is produced will get converted to the map's element type.
		elemTy := targetTy.ElementType()
		// Temporary extension of the path for our recursive call. We
		// are effectively testing all elements of the resulting map at
		// once, so the placeholder key is an unknown string.
		childPath := append(path, cty.IndexStep{Key: cty.UnknownVal(cty.String)})
		return yieldDiscardedObjectConstructorAttrs(expr.ValExpr, elemTy, childPath, yield)

	default:
		// We don't have anything useful to do with any other expression types.
		return true
	}
}

func yieldDiscardedObjectConstructorAttrsListLike(expr hcl.Expression, targetTy cty.Type, path cty.Path, yield func(DiscardedObjectConstructorAttr) bool) bool {
	switch expr := expr.(type) {
	case *hclsyntax.TupleConsExpr:
		// When a list or set is defined by conversion of a tuple constructor
		// result, we can recursively visit the definitions of any items, using
		// the collection element type for all attribute values because that's
		// how the tuple elements would eventually be converted.
		elemTy := targetTy.ElementType()
		for idx, childExpr := range expr.Exprs {
			// Temporary extension of the path for our recursive call
			childPath := append(path, cty.IndexStep{Key: cty.NumberIntVal(int64(idx))})
			if !yieldDiscardedObjectConstructorAttrs(childExpr, elemTy, childPath, yield) {
				return false
			}
		}
		return true
	case *hclsyntax.ForExpr:
		if expr.KeyExpr != nil {
			// This is an object-for expression, which is not a valid way
			// to define a list or set and so we'll ignore it and let this
			// fail type conversion when the caller tries.
			return true
		}
		// In this case we're testing the result expression of the tuple-for
		// imagining that it will become a value for each element of the
		// resulting list/set, and therefore we can assume that any value that
		// is produced will get converted to the map's element type.
		elemTy := targetTy.ElementType()
		// Temporary extension of the path for our recursive call. We
		// are effectively testing all elements of the resulting map at
		// once, so the placeholder key is an unknown number.
		childPath := append(path, cty.IndexStep{Key: cty.UnknownVal(cty.Number)})
		return yieldDiscardedObjectConstructorAttrs(expr.ValExpr, elemTy, childPath, yield)
	default:
		// We don't have anything useful to do with any other expression types.
		return true
	}
}

func yieldDiscardedObjectConstructorAttrsTuple(expr hcl.Expression, targetTy cty.Type, path cty.Path, yield func(DiscardedObjectConstructorAttr) bool) bool {
	switch expr := expr.(type) {
	case *hclsyntax.TupleConsExpr:
		// For a tuple type each element has its own type, so we'll visit
		// pairs of child attribute type and child expression.
		// We only do this when the tuple constructor has the same number of
		// elements as the tuple type, because otherwise type conversion will
		// ultimately fail anyway.
		etys := targetTy.TupleElementTypes()
		if len(expr.Exprs) != len(etys) {
			return true
		}
		for idx, childExpr := range expr.Exprs {
			elemTy := etys[idx]
			// Temporary extension of the path for our recursive call
			childPath := append(path, cty.IndexStep{Key: cty.NumberIntVal(int64(idx))})
			if !yieldDiscardedObjectConstructorAttrs(childExpr, elemTy, childPath, yield) {
				return false
			}
		}
		return true
	default:
		// We don't have anything useful to do with any other expression types.
		// (We don't try to analyze tuple-for expressions when the target
		// type is tuple because in that case we can't make any reliable
		// correlation between the dynamically-decided result values and the
		// statically-decided tuple element types.)
		return true
	}
}

// attributeNameFromObjectConsKey tries to find a known, constant attribute
// name given the expression from the "key" part of an item in an object
// constructor expression.
//
// If the second result is false then the key expression is invalid or too
// complicated to analyze.
func attributeNameFromObjectConsKey(keyExpr hcl.Expression) (string, bool) {
	// The following is a cut-down version of the logic that
	// HCL itself uses to evaluate keys in an object constructor
	// expression during evaluation, from
	// [hclsyntax.ObjectConsExpr.Value].
	//
	// This is a best-effort thing which only works for
	// valid and literally-defined keys, since that's the common
	// case we're trying to lint-check. We'll ignore anything that
	// we can't trivially evaluate.
	key, keyDiags := keyExpr.Value(nil)
	if keyDiags.HasErrors() || !key.IsKnown() || key.IsNull() {
		return "", false
	}
	key, _ = key.Unmark()
	var err error
	key, err = convert.Convert(key, cty.String)
	if err != nil {
		return "", false
	}
	return key.AsString(), true
}
