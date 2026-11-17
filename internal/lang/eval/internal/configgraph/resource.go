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
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/lang/marks"
	"github.com/opentofu/opentofu/internal/resources"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type Resource struct {
	// Addr is the absolute address of this resource, used as the basis of
	// the addresses used to track its instances between plan/apply rounds
	// and between the plan and apply phases in a single round.
	//
	// Placeholder addresses (where the IsPlaceholder method returns true) are
	// allowed here, representing that the containing object is actually
	// itself a placeholder for zero or more resources whose existence
	// and addresses we cannot determine yet.
	Addr addrs.AbsResource

	// Provider is the provider that this resource's type belongs to. This
	// is the provider to use when asking for config validation, etc.
	//
	// Each instance of the resource is required to have the same provider,
	// but they are allowed to use different instances of that provider.
	Provider addrs.Provider

	// InstanceSelector represents a rule for deciding which instances of
	// this resource have been declared.
	InstanceSelector InstanceSelector

	// CompileResourceInstance is a callback function provided by whatever
	// compiled this [Resource] object that knows how to produce a compiled
	// [ResourceInstance] object once we know of the instance key and associated
	// repetition data for it.
	//
	// This indirection allows the caller to take into account the same
	// context it had available when it built this [Resource] object, while
	// incorporating the new information about this specific instance.
	CompileResourceInstance func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *ResourceInstance

	DestroyProvisioners func(ctx context.Context, addr addrs.ResourceInstance) []Provisioner

	// DeleteWhenUnwantedValuer returns the module author's decision about
	// whether unwanted objects belonging to this resource should be deleted
	// (cty.True) or just "forgotten" (cty.False).
	DeleteWhenUnwantedValuer *OnceValuer

	// PreventDestroyValuer is a valuer that returns the module author's
	// direction about if this resource instance can be destroyed.
	//
	// The valuer must return something that can be converted to [cty.Bool].
	PreventDestroyValuer *OnceValuer

	DeclRange tfdiags.SourceRange

	// instancesResult tracks the process of deciding which instances are
	// currently declared for this resource, and the result of that process.
	//
	// Only the decideInstances method accesses this directly. Use that
	// method to obtain the coalesced result for use elsewhere.
	instancesResult grapheval.Once[*compiledInstances[*ResourceInstance]]

	// valueResult ensures that we only compute the value of the resource once.
	// This is primarilly a performance optimization.
	valueResult grapheval.Once[cty.Value]
}

var _ exprs.Valuer = (*Resource)(nil)

// IsExpansionPlaceholder returns true if this object is acting as a placeholder
// for zero or more resources whose existence and addresses cannot be decided
// yet, because the expansion rule depends on information that isn't known yet.
//
// Note that at the Resource level this means that one of the modules this
// resource is nested within has an unknown set of instances, rather than
// that this resource's own expansion is not known. Unknown expansion of the
// resource itself is represented by producing a single [ResourceInstance]
// which is a placeholder itself, as reported by
// [ResourceInstance.IsExpansionPlaceholder].
func (r *Resource) IsExpansionPlaceholder() bool {
	return r.Addr.IsPlaceholder()
}

// Instances returns the instances that are selected for this resource in its
// configuration, without evaluating their configuration objects yet.
//
// Use this when performing a tree walk to discover resource instances to
// make sure that it's possible to tell whatever process is running alongside
// that it needs to produce a result value for a particular resource instance
// before we actually request that value.
func (r *Resource) Instances(ctx context.Context) map[addrs.InstanceKey]*ResourceInstance {
	ctx = withDebugAddr(ctx, r.Addr, "Instances")
	// We ignore the diagnostics here because they will be returned by
	// the Value method instead.
	result, diags := r.decideInstances(ctx)
	if diags.HasErrors() && result == nil {
		// If decideInstances fails for grapheval-related reasons, such as a
		// dependency cycle, then it won't produce any result at all. The
		// errors from that would be collected by a concurrent
		// [Resource.CheckAll] and so we just report no instances here to
		// allow things to unwind and report that error.
		// (If decideInstances returns nil without returning any errors then
		// that's a bug in decideInstances that should be fixed there.)
		return nil
	}
	return result.Instances
}

// StaticCheckTraversal implements exprs.Valuer.
func (r *Resource) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return staticCheckTraversalForInstances(r.InstanceSelector, traversal)
}

// Value implements exprs.Valuer.
func (r *Resource) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	return r.valueResult.Do(ctx, r.value, &r.DeclRange, "Value of %s", r.Addr)
}
func (r *Resource) value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	ctx = withDebugAddr(ctx, r.Addr, "Value")
	selection, diags := r.decideInstances(ctx)
	if diags.HasErrors() && selection == nil {
		// If decideInstances fails for grapheval-related reasons, such as a
		// dependency cycle, then it won't produce any result at all, but we
		// still want to let the diagnostics propagate upwards so that the
		// error gets reported.
		return exprs.AsEvalError(cty.DynamicVal), diags
	}
	return valueForInstances(ctx, selection), diags
}

// ValueSourceRange implements exprs.Valuer.
func (r *Resource) ValueSourceRange() *tfdiags.SourceRange {
	return &r.DeclRange
}

func (r *Resource) decideInstances(ctx context.Context) (*compiledInstances[*ResourceInstance], tfdiags.Diagnostics) {
	return r.instancesResult.Do(ctx, func(ctx context.Context) (*compiledInstances[*ResourceInstance], tfdiags.Diagnostics) {
		return compileInstances(ctx, r.InstanceSelector, r.CompileResourceInstance)
	}, &r.DeclRange, "Decide instances of %s", r.Addr)
}

// ActionIfUnwanted returns the author's decision (if any) about which action
// ought to be planned for any unwanted object belonging to this resource.
func (r *Resource) ActionIfUnwanted(ctx context.Context) (exprs.WithSourceRange[resources.ActionIfUnwanted], tfdiags.Diagnostics) {
	if r.DeleteWhenUnwantedValuer == nil {
		return exprs.WithSourceRange[resources.ActionIfUnwanted]{
			FromValue: exprs.Known(resources.IfUnwantedUndecided),
		}, nil
	}
	rng := r.DeleteWhenUnwantedValuer.ValueSourceRange()

	val, diags := r.DeleteWhenUnwantedValuer.Value(ctx)
	if val == cty.NilVal {
		if !diags.HasErrors() {
			panic("DeleteWhenUnwantedValuer returned cty.NilVal without errors")
		}
		val = exprs.AsEvalError(cty.DynamicVal) // just so the rest of this can run without crashing
	}

	ret, moreDiags := prepareDestroyIfUnwanted(fmt.Sprintf("Resource %s", r.Addr), val, rng)
	diags = diags.Append(moreDiags)
	return ret, diags
}

// PreventDestroy returns a value-based representation of the "prevent destroy"
// setting for this resource.
//   - true means that this resource instance MUST NOT be destroyed.
//   - false means that this resource instance MAY be destroyed.
func (r *Resource) PreventDestroy(ctx context.Context) (exprs.WithSourceRange[bool], tfdiags.Diagnostics) {
	if r.PreventDestroyValuer == nil {
		return exprs.WithSourceRange[bool]{FromValue: exprs.Known(false)}, nil
	}
	rng := r.PreventDestroyValuer.ValueSourceRange()

	preventDestroyVal, diags := r.PreventDestroyValuer.Value(ctx)
	if preventDestroyVal == cty.NilVal {
		if !diags.HasErrors() {
			panic("PreventDestroyValuer returned cty.NilVal without errors")
		}
		preventDestroyVal = exprs.AsEvalError(cty.DynamicVal) // just so the rest of this can run without crashing
	}

	const errSummary = "Invalid value for prevent_destroy"
	preventDestroyVal, err := convert.Convert(preventDestroyVal, cty.Bool)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  errSummary,
			Detail: fmt.Sprintf(
				"Resource instance %s has an invalid value for its prevent_destroy argument: %s.",
				r.Addr.String(), tfdiags.FormatError(err),
			),
			Subject: rng.ToHCL().Ptr(),
		})
		return exprs.WithSourceRange[bool]{FromValue: exprs.Unknown[bool](), SourceRange: rng}, diags
	}
	// TODO deprecated handling
	//preventDestroyVal, moreDiags := marks.ExtractDeprecatedDiagnosticsWithExpr(preventDestroyVal, preventDestroyExpr)
	//diags = diags.Append(moreDiags)
	preventDestroyVal = PrepareOutgoingValue(preventDestroyVal)
	preventDestroyVal, pdMarks := preventDestroyVal.Unmark()
	for mark := range pdMarks {
		switch mark {
		case marks.Sensitive:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"Resource instance %s has a sensitive value for its prevent_destroy argument, which is invalid because it would cause OpenTofu to disclose the sensitive value by whether deletion is blocked.\n\nIf you know this value is not sensitive in practice, consider using the nonsensitive function to declare that.",
					r.Addr.String(),
				),
				Subject: rng.ToHCL().Ptr(),
				Extra:   evalchecks.DiagnosticCausedByConfidentialValues(true),
			})
		case marks.Ephemeral:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"Resource instance %s has an ephemeral value for its prevent_destroy argument, which is invalid because the decision for whether it's okay to destroy instances of this resource instance must stay consistent between plan and apply.",
					r.Addr.String(),
				),
				Subject: rng.ToHCL().Ptr(),
			})
		default:
			// This is a generic error message just to make sure that we'll
			// fail if a new kind of mark gets added in future which we've
			// not yet considered whether to allow here. If we add a new mark
			// kind then we should add a new case for it above, even if the
			// behavior is to do absolutely nothing because that mark is
			// allowed in prevent_destroy.
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"Resource instance %s has a prevent_destroy value derived from something that isn't allowed for deciding whether a resource instance may be destroyed (has internal mark %#v). The fact that OpenTofu cannot give more details about this is a bug, so please report it!",
					r.Addr.String(), mark,
				),
				Subject: rng.ToHCL().Ptr(),
			})
		}
	}
	if preventDestroyVal.IsNull() {
		// We could potentially treat null as equivalent to false here, matching
		// how OpenTofu would behave if there were no expression present at all,
		// but "false" is just as easy to specify as "null" in a conditional
		// expression and doesn't require a reader to know what the default
		// is, so we'll require that to make life easier for a future maintainer
		// that isn't necessarily familiar with the prevent_destroy behavior yet.
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  errSummary,
			Detail: fmt.Sprintf(
				"Resource %s has prevent_destroy set to null. When making a dynamic decision to allow destroy, use false instead.",
				r.Addr,
			),
			Subject: rng.ToHCL().Ptr(),
		})
		// We'll still return a valid value so that downstream work can continue
		// despite the error.
		preventDestroyVal = cty.False.WithSameMarks(preventDestroyVal)
	}
	if diags.HasErrors() {
		preventDestroyVal = exprs.AsEvalError(preventDestroyVal)
	}
	ret, _ := exprs.DeriveFromValue(preventDestroyVal, func(v cty.Value) (bool, error) {
		return v.True(), nil
	})
	return exprs.WithSourceRange[bool]{FromValue: ret, SourceRange: rng}, diags
}

// CheckAll implements allChecker.
func (r *Resource) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	// Our InstanceSelector itself might block on expression evaluation,
	// so we'll run it async as part of the checkGroup.
	cg.Await(ctx, func(ctx context.Context) {
		for _, inst := range r.Instances(ctx) {
			cg.CheckValuer(ctx, inst)
		}
	})
	// We'll also check our final value for the overall resource, which
	// is where we report any problems with the resource's InstanceSelector.
	// (e.g. this is where an invalid for_each expression would be reported)
	cg.CheckValuer(ctx, r)
	if r.DeleteWhenUnwantedValuer != nil {
		cg.CheckDiagsFunc(ctx, func(ctx context.Context) tfdiags.Diagnostics {
			_, diags := r.ActionIfUnwanted(ctx)
			return diags
		})
	}
	if r.PreventDestroyValuer != nil {
		cg.CheckDiagsFunc(ctx, func(ctx context.Context) tfdiags.Diagnostics {
			_, diags := r.PreventDestroy(ctx)
			return diags
		})
	}
	return cg.Complete(ctx)
}

// prepareDestroyIfUnwanted translates the raw value from evaluating a "destroy"
// argument in either a "resource" or "removed" block into its internal
// representation, [resources.ActionIfUnwanted].
//
//   - cty.True corresponds to [resources.IfUnwantedDelete]
//   - cty.False corresponds to [resources.IfUnwantedForget]
//   - cty.NullVal(cty.Bool) corresponds to [resources.IfUnwantedUndecided],
//     leaving the decision to some other part of the configuration.
//
// This function automatically attempts to convert the given value to [cty.Bool]
// and handles any relevant marks on the value.
//
// "noun" should be set to something like "Resource aws_instance.example" or
// "Resource instance aws_instance.example[0]" describing what exactly this
// destroy argument was associated with, suitable for appearing at the start
// of a sentence as the subject of that sentence.
func prepareDestroyIfUnwanted(noun string, val cty.Value, rng *tfdiags.SourceRange) (exprs.WithSourceRange[resources.ActionIfUnwanted], tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	val = PrepareOutgoingValue(val)

	const errSummary = "Invalid value for \"destroy\" argument"
	val, err := convert.Convert(val, cty.Bool)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  errSummary,
			Detail: fmt.Sprintf(
				"%s has an invalid value for its \"destroy\" argument: %s.",
				noun, tfdiags.FormatError(err),
			),
			Subject: rng.ToHCL().Ptr(),
		})
		return exprs.WithSourceRange[resources.ActionIfUnwanted]{
			FromValue:   exprs.Unknown[resources.ActionIfUnwanted]().Mark(exprs.EvalError),
			SourceRange: rng,
		}, diags
	}

	val, valMarks := val.Unmark()
	// TODO: Handle any deprecation marks on "val"
	for mark := range valMarks {
		switch mark {
		case marks.Sensitive:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"%s has a sensitive value for its \"destroy\" argument, which is invalid because it would cause OpenTofu to disclose the sensitive value by whether it proposes deleting or forgetting.\n\nIf you know this value is not sensitive in practice, consider using the nonsensitive function to declare that.",
					noun,
				),
				Subject: rng.ToHCL().Ptr(),
				Extra:   evalchecks.DiagnosticCausedByConfidentialValues(true),
			})
		case marks.Ephemeral:
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"%s has an ephemeral value for its \"destroy\" argument, which is invalid because the decision for whether to delete or forget objects belonging to this resource must stay consistent between plan and apply.",
					noun,
				),
				Subject: rng.ToHCL().Ptr(),
			})
		default:
			// This is a generic error message just to make sure that we'll
			// fail if a new kind of mark gets added in future which we've
			// not yet considered whether to allow here. If we add a new mark
			// kind then we should add a new case for it above, even if the
			// behavior is to do absolutely nothing because that mark is
			// allowed in "destroy".
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  errSummary,
				Detail: fmt.Sprintf(
					"%s has a \"destroy\" value derived from something that isn't allowed for deciding whether to delete or forget (has internal mark %#v). The fact that OpenTofu cannot give more details about this is a bug, so please report it!",
					noun, mark,
				),
				Subject: rng.ToHCL().Ptr(),
			})
		}
	}
	ret, _ := exprs.DeriveFromValue(val, func(v cty.Value) (resources.ActionIfUnwanted, error) {
		if val.IsNull() {
			// We treat a null value the same as omitting the argument completely,
			// so that it's possible for a module to dynamically decide between
			// setting the argument or letting a child module make the decision
			// instead.
			return resources.IfUnwantedUndecided, nil
		}
		if val.True() {
			return resources.IfUnwantedDelete, nil
		}
		return resources.IfUnwantedForget, nil
	})
	if diags.HasErrors() {
		ret = ret.Mark(exprs.EvalError)
	}
	return exprs.WithSourceRange[resources.ActionIfUnwanted]{
		FromValue:   ret,
		SourceRange: rng,
	}, diags
}
