// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// An InstanceSelector defines a rule for choosing which dynamic child instances
// exist for a particular object.
//
// This package intentionally doesn't directly know directly about the "count",
// "for_each", and "enabled" meta arguments used in the current surface language
// because it's designed to be flexible for us to potentially change how these
// features work in later editions of the language, but the general idea here
// is that the "compiler" code for a specific edition of the language would
// have an implementation of this for each of the different repetition arguments
// and populate the appropriate one into the InstanceSelector field of each
// multi-instance container object.
type InstanceSelector interface {
	// InstanceKeyType returns the instance key type that all instances
	// produced by this selector will have.
	//
	// This is separated to allow for static validation of traversals
	// before actually selecting the instances. It also decides
	// between several different possible representations of an empty
	// set of instances.
	InstanceKeyType() addrs.InstanceKeyType

	// Instances returns a sequence of all of the instance keys, which
	// must must be of the same type returned from InstanceKeyType,
	// and the associated repetition data to use when compiling each
	// instance.
	//
	// If the decision about which instance keys to return was based
	// on evaluating expressions or otherwise interacting with cty values
	// then the second return value describes any marks that were
	// present on the value used to decide which instance keys exist.
	// If there were no marks at all, or the decision was not based on
	// evaluating expressions, then this returns a nil [cty.ValueMarks]
	//
	// If the returned diagnostics contains an error then the set of
	// instance keys is ignored but the returned marks will still be
	// retained and used for building a placeholder result.
	Instances(ctx context.Context) (Maybe[InstancesSeq], cty.ValueMarks, tfdiags.Diagnostics)

	// InstancesSourceRange optionally reports a source range for something in
	// the configuration that the author would consider as representing the
	// rule for deciding which instances exist.
	//
	// For example, this could be the source range of the expression
	// associated with a "for_each" argument, if that was what the
	// selector was based on.
	//
	// If there is no single obvious configuration construct to report
	// then prefer to return nil rather than returning something strange.
	InstancesSourceRange() *tfdiags.SourceRange
}

type InstancesSeq = func(yield func(addrs.InstanceKey, instances.RepetitionData) bool)

type compiledInstances[T any] struct {
	KeyType addrs.InstanceKeyType

	// Instances are the compiled objects representing each of the
	// currently-declared instances.
	Instances map[addrs.InstanceKey]T

	// ValueMarks are marks that must be applied to any value that is
	// created based on this decision about which resource instances
	// are available. (This will carry over any marks that were associated
	// with the value used to decide which instances exist.)
	ValueMarks cty.ValueMarks
}

func compileInstances[T any](
	ctx context.Context,
	selector InstanceSelector,
	compileInstance instanceCompiler[T],
) (*compiledInstances[T], tfdiags.Diagnostics) {
	keyType := selector.InstanceKeyType()
	maybeInsts, valueMarks, diags := selector.Instances(ctx)
	if diags.HasErrors() {
		return compilePlaceholderInstance(ctx, keyType, valueMarks, compileInstance), diags
	}
	insts, ok := GetKnown(maybeInsts)
	if !ok {
		return compilePlaceholderInstance(ctx, keyType, valueMarks, compileInstance), diags
	}
	ret := &compiledInstances[T]{
		KeyType:    keyType,
		Instances:  make(map[addrs.InstanceKey]T),
		ValueMarks: valueMarks,
	}
	for key, repData := range insts {
		// We transfer valueMarks to the each.key, each.value and count.index
		// values because valueMarks are the marks on whatever value we used to
		// decide which instances exist, and so any reference to those
		// should carry along mark information.
		// In particular this propagates resource instance dependency
		// information in case the count or for_each expression was derived
		// from resource instance attributes.
		if repData.EachKey != cty.NilVal {
			repData.EachKey = repData.EachKey.WithMarks(valueMarks)
		}
		if repData.EachValue != cty.NilVal {
			repData.EachValue = repData.EachValue.WithMarks(valueMarks)
		}
		if repData.CountIndex != cty.NilVal {
			repData.CountIndex = repData.CountIndex.WithMarks(valueMarks)
		}
		obj := compileInstance(ctx, key, repData)
		ret.Instances[key] = obj
	}
	return ret, diags
}

func compilePlaceholderInstance[T any](
	ctx context.Context,
	keyType addrs.InstanceKeyType,
	valueMarks cty.ValueMarks,
	compileInstance instanceCompiler[T],
) *compiledInstances[T] {
	// If we don't have enough information to decide the instances
	// then we produce a single placeholder instance which stands in
	// for zero or more instances. The placeholder's scope includes
	// unknown values of the appropriate types for local symbols like
	// each.key, so that the resulting resource instance is a
	// representation of what we know to be true for all instances.
	repData := instances.RepetitionData{}
	switch keyType {
	case addrs.StringKeyType:
		repData.EachKey = cty.UnknownVal(cty.String).WithMarks(valueMarks)
		repData.EachValue = cty.DynamicVal.WithMarks(valueMarks)
	case addrs.IntKeyType:
		repData.CountIndex = cty.UnknownVal(cty.Number).WithMarks(valueMarks)
	}
	key := addrs.WildcardKey{keyType}
	placeholder := compileInstance(ctx, key, repData)
	return &compiledInstances[T]{
		KeyType: keyType,
		Instances: map[addrs.InstanceKey]T{
			key: placeholder,
		},
		ValueMarks: valueMarks,
	}
}

type instanceCompiler[T any] func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) T

// valueForInstances turns a compiled set of instances of a type that implements
// [Valuer] into our conventional representation of a particular set of
// instances as a value for use elsewhere in the configuration.
func valueForInstances[T exprs.Valuer](ctx context.Context, insts *compiledInstances[T]) cty.Value {
	// All return paths from this function MUST include the
	// effect of .WithMarks(insts.ValueMarks), to make sure that
	// dynamic resource dependency tracking is effective when
	// resource instances contribute to the decision about which
	// instances exist.
	//
	// As a general rule this function should not pass on diagnostics
	// produced by calling Value on the given valuers, because we
	// assume that a validation visitor will call Value on each
	// of the child instances and so collect diagnostics directly.
	//
	// This function is conceptually like an HCL expression
	// producing a result derived from a reference to something else.
	switch insts.KeyType {
	case addrs.NoKeyType:
		// We should have either zero or one instances, because there is
		// only one key of this type. Our result is either the value of
		// that instance alone, or null to represent the absense of any object.
		if len(insts.Instances) > 1 {
			// This suggests a bug in the [InstanceSelector] that chose these instances.
			panic("more than one instance when there's no instance key type")
		}
		if placeholder, ok := insts.Instances[addrs.WildcardKey{addrs.NoKeyType}]; ok {
			v := diagsHandledElsewhere(placeholder.Value(ctx))
			// Since we don't know yet whether an object will exist here
			// at all, returning the placeholder directly here would be
			// overpromising, but we _can_ return an unknown value of
			// the same type to allow some typechecking for operations
			// that would definitely fail even if this turns out to be
			// non-null.
			return cty.UnknownVal(v.Type()).WithSameMarks(v).WithMarks(insts.ValueMarks)
		}
		inst, ok := insts.Instances[addrs.NoKey]
		if !ok {
			// TODO: We don't have enough information here to know what
			// type of null to return. Does that matter? HCL doesn't
			// typically do much with the types of null values anyway,
			// and cty can convert a result like this to another type of
			// null without complaint...
			return cty.NullVal(cty.DynamicPseudoType).WithMarks(insts.ValueMarks)
		}
		return diagsHandledElsewhere(inst.Value(ctx)).WithMarks(insts.ValueMarks)

	case addrs.StringKeyType:
		if _, ok := insts.Instances[addrs.WildcardKey{addrs.StringKeyType}]; ok {
			// In this case we cannot predict anything about the placeholder
			// result because if we don't know the instance keys then we
			// cannot even predict the object type.
			return cty.DynamicVal.WithMarks(insts.ValueMarks)
		}

		// We can have zero or more instances in this case, and each
		// instance key will become an attribute name in an object
		// we're returning.
		attrs := make(map[string]cty.Value, len(insts.Instances))
		for key, obj := range insts.Instances {
			attrName := string(key.(addrs.StringKey)) // panic here means buggy [InstanceSelector]
			// Diagnostics for this are collected directly from the instance
			// when the CheckAll tree walk visits it.
			attrs[attrName] = diagsHandledElsewhere(obj.Value(ctx))
		}
		return cty.ObjectVal(attrs).WithMarks(insts.ValueMarks)

	case addrs.IntKeyType:
		if _, ok := insts.Instances[addrs.WildcardKey{addrs.IntKeyType}]; ok {
			// In this case we cannot predict anything about the placeholder
			// result because if we don't know how many instances we have
			// then we cannot even predict the tuple type.
			return cty.DynamicVal.WithMarks(insts.ValueMarks)
		}
		elems := make([]cty.Value, len(insts.Instances))
		for i := range elems {
			inst, ok := insts.Instances[addrs.IntKey(i)]
			if !ok {
				// This should not be possible for a correct [InstanceSelector]
				// but this is not the place to deal with that.
				elems[i] = cty.DynamicVal
				continue
			}
			// Diagnostics for this are collected directly from the instance
			// when the CheckAll tree walk visits it.
			elems[i] = diagsHandledElsewhere(inst.Value(ctx))
		}
		return cty.TupleVal(elems).WithMarks(insts.ValueMarks)

	default:
		// Should not get here because [InstanceSelector] is not allowed to
		// return any other key type.
		panic("unsupported instance key type")
	}
}

func staticCheckTraversalForInstances(selector InstanceSelector, traversal hcl.Traversal) tfdiags.Diagnostics {
	if len(traversal) == 0 {
		return nil // empty traversal is always valid here
	}
	var diags tfdiags.Diagnostics
	switch keyType := selector.InstanceKeyType(); keyType {
	case addrs.IntKeyType, addrs.StringKeyType:
		// We disallow using attribute access in this case because
		// it's ambiguous with the common mistake of forgetting to
		// include the instance key at all, and we can return a better
		// error message for that mistake if we force always using
		// the index syntax for an instance key.
		var example string
		switch keyType {
		case addrs.IntKeyType:
			example = "[0]"
		case addrs.StringKeyType:
			example = "[\"key\"]"
		}
		if _, isAttr := traversal[0].(hcl.TraverseAttr); isAttr {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Missing instance key",
				Detail:   fmt.Sprintf("This value has multiple instances, so an instance key selection like %s must appear before accessing an attribute.", example),
				Subject:  traversal[0].SourceRange().Ptr(),
			})
		}
	}
	return diags
}
