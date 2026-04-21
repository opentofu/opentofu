// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package objchange

import (
	"fmt"
	"iter"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
)

// PendingChange is used to "mark" cty values which are derived from the result
// of a planned change that hasn't yet been applied to the remote object it
// belongs to.
//
// [MarkPendingChanges] produces values marked in this way. We can use this in
// situations where a pending change upstream must cause something downstream
// to be delayed until that change has been applied, such as when the
// configuration for a data resource instance refers to a value that is expected
// to change during the apply phase and therefore the data resource instance
// must not be read until the apply phase.
type PendingChange struct {
	// instAddr is a string representation of the address of the resource
	// instance whose changes must be applied before the value can be assumed
	// to match the remote system state.
	//
	// This is an unexported string internally just because cty marks are
	// required to be comparable. In the public API we use
	// [addrs.AbsResourceInstance] instead, for better typechecking in callers.
	instAddr string
}

// ValuePendingChange returns the given value marked as being derived from the
// result of a pending change to the resource instance given in instAddr.
func ValuePendingChange(v cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	return v.Mark(PendingChange{
		instAddr: instAddr.String(),
	})
}

// PrereqChangesForValue takes a value derived from a result of
// [MarkPendingChanges]  and returns a sequence of all of the resource instance
// addresses that have pending changes that must be applied before the remote
// system should agree with the given value.
//
// This is intended for deciding whether it's okay to read a data resource
// instance during the planning phase or if it must wait until some other
// changes have been applied during the apply phase. If this returns one or more
// addresses then the action of reading the data resource instance should
// happen during the apply phase and should happen only after the changes for
// the returned resource instances have been successfully applied.
//
// The same resource instance could potentially appear more than once in the
// sequence if its changed values were used in more than one place in the
// given value. Callers should ignore duplicate reports, such as by adding
// each of the results to an [addrs.Set].
//
// It's okay but useless to call this function with a value that wasn't
// derived from a [MarkPendingChanges] result: in that case it'll just return
// an empty sequence.
func PrereqChangesForValue(v cty.Value) iter.Seq[addrs.AbsResourceInstance] {
	return func(yield func(addrs.AbsResourceInstance) bool) {
		for m := range cty.ValueMarksOfTypeDeep[PendingChange](v) {
			// We assume that the address string is always valid because only
			// [ValuePendingChange] should be constructing marks of this type.
			addr, diags := addrs.ParseAbsResourceInstanceStr(m.instAddr)
			if diags.HasErrors() {
				panic(fmt.Sprintf("invalid PendingChange mark with address string %q", m.instAddr))
			}
			if !yield(addr) {
				return
			}
		}
	}
}

// MarkUnknownValuesAsPending returns a value that is equivalent to the given
// value except that any unknown values within it are marked as having pending
// changes associated with the given resource instance address.
//
// This is intended for when preparing the approximate planned value for a
// data resource instance whose read is being delayed until the apply phase to
// await some upstream changes. In that case, we want to treat the unknown parts
// of that placeholder as also being pending changes so that downstream uses
// of those values in other data resource instances can get attributed to the
// data resource instance in question.
//
// Ideally we'd merge [PlannedUnknownObject] and this function together to do
// both jobs at once instead of doing this in two passes, but that'll wait until
// we're ready to start changing code that the older OpenTofu runtime depends
// on, rather than having new-runtime-specific needs intentionally separated.
func MarkUnknownValuesAsPending(v cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	// This transform cannot fail because the given callback never returns an error.
	ret, _ := cty.Transform(v, func(p cty.Path, v cty.Value) (cty.Value, error) {
		if !v.IsKnown() {
			return ValuePendingChange(v, instAddr), nil
		}
		return v, nil
	})
	return ret
}

// MarkPendingChanges returns a value with the same content as "planned" except
// that it has additional [PendingChange] marks on any part of the value that
// differs from "prior".
//
// Use this with the prior and planned values for a resource instance to
// produce a value that can be used for downstream analysis of what else in the
// configuration is derived from changes that have not been applied yet.
//
// The given values must both conform to the given schema and must be consistent
// with the rules enforced by [AssertPlanValid], or the behavior is unspecified.
func MarkPendingChanges(schema *configschema.Block, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	return markPendingChangesSchemaObj(schema.Attributes, schema.BlockTypes, prior, planned, instAddr)
}

// markPendingChangesNestedBlockType does the work of [MarkPendingChanges] for a
// nested block type.
func markPendingChangesNestedBlockType(schema *configschema.NestedBlock, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	return markPendingChangesNestedSchemaObj(
		schema.Nesting, schema.Attributes, schema.BlockTypes,
		prior, planned,
		instAddr,
	)
}

// markPendingChangesComplexAttr does the work of [MarkPendingChanges] for an
// attribute that uses "structural typing", where it has a full nested schema
// rather than just a required type.
func markPendingChangesComplexAttr(schema *configschema.Object, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	return markPendingChangesNestedSchemaObj(
		schema.Nesting, schema.Attributes, nil,
		prior, planned,
		instAddr,
	)
}

// markPendingChangesSchemaObj deals with the common aspects of top-level
// blocks, instances of nested blocks, and structural attributes, all of which
// involve cty object types.
func markPendingChangesSchemaObj(
	attrs map[string]*configschema.Attribute,
	nestedBlocks map[string]*configschema.NestedBlock,
	prior, planned cty.Value,
	instAddr addrs.AbsResourceInstance,
) cty.Value {
	if prior.IsNull() || planned.IsNull() || !planned.IsKnown() {
		// For null and unknown values we just use the "leaf" implementation
		// because deep analysis is not possible or needed or possible.
		return markPendingChangesLeaf(prior, planned, instAddr)
	}
	retAttrs := make(map[string]cty.Value, len(attrs)+len(nestedBlocks))
	for name, attrS := range attrs {
		nestedPrior := prior.GetAttr(name)
		nestedPlanned := planned.GetAttr(name)
		if attrS.NestedType == nil {
			// This is a "simple" attribute which is just a regular value
			// required to conform to some type constraint.
			retAttrs[name] = markPendingChangesAttr(attrS.Type, nestedPrior, nestedPlanned, instAddr)
		} else {
			retAttrs[name] = markPendingChangesComplexAttr(attrS.NestedType, nestedPrior, nestedPlanned, instAddr)
		}
	}
	for name, blockS := range nestedBlocks {
		nestedPrior := prior.GetAttr(name)
		nestedPlanned := planned.GetAttr(name)
		retAttrs[name] = markPendingChangesNestedBlockType(blockS, nestedPrior, nestedPlanned, instAddr)
	}
	return cty.ObjectVal(retAttrs)
}

func markPendingChangesNestedSchemaObj(
	nesting configschema.NestingMode,
	attrs map[string]*configschema.Attribute,
	nestedBlocks map[string]*configschema.NestedBlock,
	prior, planned cty.Value,
	instAddr addrs.AbsResourceInstance,
) cty.Value {
	if prior.IsNull() || planned.IsNull() || !planned.IsKnown() {
		// For null and unknown values we just use the "leaf" implementation
		// because deep analysis is not possible or needed or possible.
		return markPendingChangesLeaf(prior, planned, instAddr)
	}
	switch nesting {
	case configschema.NestingSingle, configschema.NestingGroup:
		return markPendingChangesSchemaObj(
			attrs, nestedBlocks,
			prior, planned,
			instAddr,
		)
	case configschema.NestingList:
		if lengthEq, _ := planned.Length().Equals(prior.Length()).Unmark(); !lengthEq.IsKnown() || lengthEq.False() {
			// If the length has changed then there's no need for any deeper
			// analysis, since we'll be marking the top-level value anyway.
			return ValuePendingChange(planned, instAddr)
		}
		var retElems []cty.Value
		for k := range planned.Elements() {
			step := cty.IndexStep{Key: k}
			// These lookups should not be able to fail because we checked that
			// both lists have the same length above.
			nestedPrior, _ := step.Apply(prior)
			nestedPlanned, _ := step.Apply(planned)
			newVal := markPendingChangesSchemaObj(
				attrs, nestedBlocks,
				nestedPrior, nestedPlanned,
				instAddr,
			)
			retElems = append(retElems, newVal)
		}
		// NestingList can represent either a list or a tuple type depending
		// on whether the nested schema has dynamically-typed items, so we'll
		// respect whatever kind "planned" had.
		if ty := planned.Type(); ty.IsTupleType() {
			return cty.TupleVal(retElems)
		} else {
			if len(retElems) == 0 {
				return cty.ListValEmpty(ty.ElementType())
			}
			return cty.ListVal(retElems)
		}
	case configschema.NestingMap:
		if lengthEq, _ := planned.Length().Equals(prior.Length()).Unmark(); !lengthEq.IsKnown() || lengthEq.False() {
			// If the length has changed then there's no need for any deeper
			// analysis, since we'll be marking the top-level value anyway.
			return ValuePendingChange(planned, instAddr)
		}
		// If there are any elements of prior that are not also in planned
		// then we must treat the whole map has having pending changes, because
		// its set of keys is changing. We'll check this first since there's
		// no need to construct a new result if we return here.
		for k := range prior.Elements() {
			step := cty.IndexStep{Key: k}
			_, err := step.Apply(planned)
			if err != nil {
				return ValuePendingChange(planned, instAddr)
			}
		}
		// If we get here then we know that the keys in the planned map are a
		// superset of those in prior, and so we can produce a more precise
		// result that describes only individual elements as having changed.
		retElems := make(map[string]cty.Value)
		for k := range planned.Elements() {
			step := cty.IndexStep{Key: k}
			nestedPrior, err := step.Apply(prior)
			if err != nil {
				// If the same key can't be used in prior then that means the
				// set of keys has changed, so for the sake of our analysis here
				// we'll treat the prior value as a null value.
				nestedPrior = cty.NullVal(prior.Type())
			}
			// This should not fail because we're iterating over the planned keys
			nestedPlanned, _ := step.Apply(planned)
			retElems[k.AsString()] = markPendingChangesSchemaObj(
				attrs, nestedBlocks,
				nestedPrior, nestedPlanned,
				instAddr,
			)
		}
		// NestingMap can represent either a map or an object type depending
		// on whether the nested schema has dynamically-typed items, so we'll
		// respect whatever kind "planned" had.
		if ty := planned.Type(); ty.IsObjectType() {
			return cty.ObjectVal(retElems)
		} else {
			if len(retElems) == 0 {
				return cty.MapValEmpty(ty.ElementType())
			}
			return cty.MapVal(retElems)
		}
	case configschema.NestingSet:
		// cty doesn't support nested marks inside a set anyway, so there's
		// no point in doing deep analysis here: we'll just treat this as a leaf.
		return markPendingChangesLeaf(prior, planned, instAddr)
	default:
		// The above should be exhaustive for all possible nesting modes.
		panic(fmt.Sprintf("unsupported nesting mode %#v", nesting))
	}
}

// markPendingChangesAttr does the work of [MarkPendingChanges] for a
// leaf non-structural attribute, or a nested part of one recursively.
//
// The second return value is true if a [PendingChange] mark was added anywhere
// inside the returned value, so that the caller doesn't need to do a deep
// mark search to discover that.
func markPendingChangesAttr(ty cty.Type, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	if prior.IsNull() || planned.IsNull() || !planned.IsKnown() {
		// For null and unknown values we just use the "leaf" implementation
		// because deep analysis is not possible or needed or possible.
		return markPendingChangesLeaf(prior, planned, instAddr)
	}
	if ty.HasDynamicTypes() {
		// If the schema allows various types but the two values we have are of
		// the same dynamic type then we'll compare them by their dynamic type
		// so we can get a more precise result. Values of different types are
		// never equal though, so it's okay not to do this when they differ.
		if plannedTy := planned.Type(); plannedTy.Equals(prior.Type()) {
			ty = plannedTy
		}
	}
	if ty.IsObjectType() {
		return markPendingChangesObject(ty.AttributeTypes(), prior, planned, instAddr)
	}
	if ty.IsTupleType() {
		return markPendingChangesTuple(ty.TupleElementTypes(), prior, planned, instAddr)
	}
	if ty.IsListType() {
		return markPendingChangesList(ty.ElementType(), prior, planned, instAddr)
	}
	if ty.IsMapType() {
		return markPendingChangesMap(ty.ElementType(), prior, planned, instAddr)
	}
	// NOTE: We don't have recursive treatment for sets because cty doesn't
	// allow marking individual nested items inside a set anyway, so there's
	// no point in trying to compute it precisely.
	return markPendingChangesLeaf(prior, planned, instAddr)
}

func markPendingChangesObject(atys map[string]cty.Type, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	if len(atys) == 0 {
		// There is only one known, non-null value of the empty object type,
		// so there can't possibly be any changes and so we can skip
		// constructing a new object.
		return cty.EmptyObjectVal
	}
	retAttrs := make(map[string]cty.Value, len(atys))
	for name, aty := range atys {
		nestedPrior := prior.GetAttr(name)
		nestedPlanned := planned.GetAttr(name)
		retAttrs[name] = markPendingChangesAttr(aty, nestedPrior, nestedPlanned, instAddr)
	}
	// Because the attributes of an object are fixed as part of the type we
	// can assume that the overall object is not a pending change, so any
	// changes we return here will be nested inside the attributes.
	return cty.ObjectVal(retAttrs)
}

func markPendingChangesTuple(etys []cty.Type, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	if len(etys) == 0 {
		// There is only one known, non-null value of the empty tuple type,
		// so there can't possibly be any changes and so we can skip
		// constructing a new tuple.
		return cty.EmptyTupleVal
	}
	retElems := make([]cty.Value, len(etys))
	for i, ety := range etys {
		step := cty.IndexStep{Key: cty.NumberIntVal(int64(i))}
		// These lookups can't fail because the length of a tuple is chosen
		// by its type rather than its value, and the caller is required to
		// make sure both values have the same type.
		nestedPrior, _ := step.Apply(prior)
		nestedPlanned, _ := step.Apply(planned)
		retElems[i] = markPendingChangesAttr(ety, nestedPrior, nestedPlanned, instAddr)
	}
	// Because the element types of a a tuple are fixed as part of the type we
	// can assume that the overall tuple is not a pending change, so any
	// changes we return here will be nested inside the elements.
	return cty.TupleVal(retElems)
}

func markPendingChangesList(ety cty.Type, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	// For lists the length is part of the value rather than part of the type,
	// so we'll compare across both lists for any index they both have in common
	// but we will also need to add a broad mark if the lengths of the lists are
	// different. We expect to have been called by [markPendingChangesAttr] so
	// that both values are guaranteed to be known and not null, but they could
	// still be marked.
	priorUnmarked, _ := prior.Unmark()
	plannedUnmarked, plannedMarks := planned.Unmark()
	priorLen := priorUnmarked.LengthInt()
	plannedLen := plannedUnmarked.LengthInt()
	commonLen := min(priorLen, plannedLen)
	retElems := make([]cty.Value, plannedLen)
	// We'll start with the indices that both lists have, where we'll compare
	// the element values directly.
	for i := range commonLen {
		step := cty.IndexStep{Key: cty.NumberIntVal(int64(i))}
		// These lookups can't fail because we know that there are at least
		// commonLen elements in both lists.
		nestedPrior, _ := step.Apply(priorUnmarked)
		nestedPlanned, _ := step.Apply(plannedUnmarked)
		retElems[i] = markPendingChangesAttr(ety, nestedPrior, nestedPlanned, instAddr)
	}
	// Now we'll deal with elements that are only present in "planned", which
	// therefore have no nested changes.
	for i := commonLen; i < plannedLen; i++ {
		step := cty.IndexStep{Key: cty.NumberIntVal(int64(i))}
		retElems[i], _ = step.Apply(plannedUnmarked)
	}
	var ret cty.Value
	if len(retElems) != 0 {
		ret = cty.ListVal(retElems)
	} else {
		ret = cty.ListValEmpty(ety)
	}
	// If the length has changed then our entire result value must be marked,
	// because e.g. length(list) itself would produce a different result after
	// the change has been applied.
	if priorLen != plannedLen {
		ret = ValuePendingChange(ret, instAddr)
	}
	return ret.WithMarks(plannedMarks)
}

func markPendingChangesMap(ety cty.Type, prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	// For maps the set of keys is part of the value rather than part of the
	// type, so we'll compare the values of any keys that both values have in
	// common but we'll also need to add a broad mark if there are any unshared
	// keys. We expect to have been called by [markPendingChangesAttr] so
	// that both values are guaranteed to be known and not null, but they could
	// still be marked.
	priorUnmarked, _ := prior.Unmark()
	plannedUnmarked, plannedMarks := planned.Unmark()
	differentKeys := false
	retElems := make(map[string]cty.Value)
	for k, nestedPlanned := range plannedUnmarked.Elements() {
		nestedPrior, err := cty.IndexStep{Key: k}.Apply(priorUnmarked)
		if err != nil {
			differentKeys = true
			retElems[k.AsString()] = nestedPlanned
			continue
		}
		retElems[k.AsString()] = markPendingChangesAttr(ety, nestedPrior, nestedPlanned, instAddr)
	}
	if !differentKeys {
		// There might also be some keys present in prior that are not present
		// in planned, which we've not checked for yet.
		for k := range priorUnmarked.Elements() {
			_, err := cty.IndexStep{Key: k}.Apply(plannedUnmarked)
			if err != nil {
				differentKeys = true
				continue
			}
		}
	}
	var ret cty.Value
	if len(retElems) != 0 {
		ret = cty.MapVal(retElems)
	} else {
		ret = cty.MapValEmpty(ety)
	}
	if differentKeys {
		ret = ValuePendingChange(ret, instAddr)
	}
	return ret.WithMarks(plannedMarks)
}

func markPendingChangesLeaf(prior, planned cty.Value, instAddr addrs.AbsResourceInstance) cty.Value {
	// We use RawEquals here because changes to marks and refinements are also
	// potentially relevant. This would detect, for example, if the sensitivity
	// of a value has changed even if the value itself did not change.
	if !planned.RawEquals(prior) {
		return ValuePendingChange(planned, instAddr)
	}
	return planned
}
