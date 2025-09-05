// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"iter"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/ctymarks"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/lang/exprs"
)

// As an implementation detail within this package we use cty marks as the
// primary mechanism for tracking dependencies between resource instances:
// the object values representing resource instances are marked with the
// addrs.AbsResourceInstance they each belong to, so that any expressions
// deriving from those will automatically track that they are relying on
// a resource instance result.
//
// We don't expose these marks outside of the "internal/lang/eval" family
// of packages. The rest of the system only needs to know the relationships
// between resource instances as a whole, because operations on entire
// resource instances are our atomic unit of change as far as OpenTofu core
// is concerned.

// ResourceInstanceMark is a cty mark value used only internally within the
// evaluation system to track when a particular expression is derived from the
// result object for a resource instance.
type ResourceInstanceMark struct {
	// instance is a pointer to the resource instance this mark relates to.
	instance *ResourceInstance
}

// ContributingResourceInstances returns an iterable sequence of all of the
// resource instances whose result values may have contributed to the
// given value.
//
// The results are not guaranteed to be unique: if different nested parts
// of the same value were derived from the same resource instance then it
// may or may not appear twice in the sequence. Deduplicating, if needed, is
// the caller's responsibility.
//
// If sending the result somewhere outside of the evaluation system, which
// therefore shouldn't be aware of the [ResourceInstance] type, consider
// passing the result to [ResourceInstanceAddrs] to provide a sequence of
// absolute resource instance addresses instead.
func ContributingResourceInstances(v cty.Value) iter.Seq[*ResourceInstance] {
	// We search deeply for marks here because we want to know which resource
	// instances contributed to _any part_ of the given value, since that
	// means changes to the resource instance could potentially change the
	// value.
	// (This current implementation _does_ mean that we'll deduplicate
	// multiple references to the same resource instance as long as the
	// pointers are unique, but the doc comment above reserves the right
	// to change our approach here in future, if needed.)
	return func(yield func(*ResourceInstance) bool) {
		for mark := range cty.ValueMarksOfTypeDeep[ResourceInstanceMark](v) {
			if !yield(mark.instance) {
				break
			}
		}
	}
}

// PrepareOutgoingValue returns a modified version of the given value that
// has been stripped of all of the marks we use internally to the evaluation
// system, and so is ready to be sent to other parts of OpenTofu that aren't
// aware of these details.
func PrepareOutgoingValue(v cty.Value) cty.Value {
	// Currently our two kinds of special internal marks are this package's
	// own [ResourceInstanceMark], and the [exprs.EvalError] mark used to
	// represent when something has been derived from an unknown value
	// used as the placeholder for an evaluation error.
	return WithoutResourceInstanceMarks(exprs.WithoutEvalErrorMarks(v))
}

// prepareResourceInstanceResult returns a modified version of the given value
// that has had its marks adjusted so that derived values will track that
// they depend on this resource instance's result.
//
// The result is also marked as depending on all of the other resource instances
// that the configuration value depends on, which is important because a
// provider is allowed to decide results of other attributes based on anything
// in the configuration and so we must conservatively assume that all parts
// of the result could be derived from any part of the config.
func prepareResourceInstanceResult(resultVal cty.Value, thisRI *ResourceInstance, configVal cty.Value) cty.Value {
	v := resultVal.Mark(ResourceInstanceMark{thisRI})
	for ri := range ContributingResourceInstances(configVal) {
		v = v.Mark(ResourceInstanceMark{ri})
	}
	// TODO: We need to transfer certain other marks from the configVal too,
	// but others need to be transferred more surgically. For example,
	// any path that is "sensitive" in configVal should also be sensitive
	// in the result, but we mustn't hoist _that_ mark up to toplevel because
	// that would be too conservative and make our plan diffs useless.
	return v
}

// WithoutResourceInstanceMarks returns a copy of the given value with
// any [ResourceInstanceMark] marks removed from it, but with all other marks
// left intact.
//
// This MUST be used at any boundary between the eval system and the rest
// of the OpenTofu codebase, because [ResourceInstanceMark] is an implementation
// detail of our evaluation strategy that the rest of the system does not
// expect to encounter.
func WithoutResourceInstanceMarks(v cty.Value) cty.Value {
	v, _ = v.WrangleMarksDeep(func(mark any, path cty.Path) (ctymarks.WrangleAction, error) {
		if _, isOurMark := mark.(ResourceInstanceMark); isOurMark {
			return ctymarks.WrangleDrop, nil
		}
		return nil, nil // leave all other marks alone
	})
	return v
}

// ResourceInstanceAddrs maps a sequence of [ResourceInstance] pointers into
// a sequence of their [addrs.AbsResourceInstance] addresses.
func ResourceInstanceAddrs(insts iter.Seq[*ResourceInstance]) iter.Seq[addrs.AbsResourceInstance] {
	return func(yield func(addrs.AbsResourceInstance) bool) {
		for inst := range insts {
			if !yield(inst.Addr) {
				break
			}
		}
	}
}
