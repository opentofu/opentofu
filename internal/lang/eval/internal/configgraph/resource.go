// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"fmt"

	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/evalchecks"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/lang/marks"
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
	})
}

// PreventDestroy returns a value-based representation of the "prevent destroy"
// setting for this resource.
//
// The result is guaranteed to be a [cty.Bool] value, but it could potentially
// be unknown or marked and it's the caller's responsibility to handle those
// situations.
//
// The different possible known boolean results have the following meaning:
//   - [cty.True] means that this resource instance MUST not be destroyed.
//   - [cty.False] means that this resource instance MAY be destroyed.
//   - A null value is not accepted, see the comment below.
//
// This was copied and modified from NodeAbstractResourceInstance.checkPreventDestroy
func (r *Resource) PreventDestroy(ctx context.Context) (cty.Value, *tfdiags.SourceRange, tfdiags.Diagnostics) {
	if r.PreventDestroyValuer == nil {
		return cty.False, nil, nil
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
	if diags.HasErrors() {
		preventDestroyVal = exprs.AsEvalError(preventDestroyVal)
	}
	return preventDestroyVal, rng, diags
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
	if r.PreventDestroyValuer != nil {
		cg.CheckValuer(ctx, r.PreventDestroyValuer)
	}
	return cg.Complete(ctx)
}

func (r *Resource) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	// There might be other grapheval requests in our dynamic instances, but
	// they are hidden behind another request themselves so we'll try to
	// report them only if that request was already started.
	instancesReqId := r.instancesResult.RequestID()
	if instancesReqId == workgraph.NoRequest {
		return
	}
	announce(instancesReqId, grapheval.RequestInfo{
		Name:        fmt.Sprintf("decide instances for %s", r.Addr),
		SourceRange: r.InstanceSelector.InstancesSourceRange(),
	})
	if r.PreventDestroyValuer != nil {
		announce(r.PreventDestroyValuer.RequestID(), grapheval.RequestInfo{
			Name:        fmt.Sprintf("prevent_destroy argument for %s", r.Addr),
			SourceRange: r.PreventDestroyValuer.ValueSourceRange(),
		})
	}
	// The Instances method potentially starts a new request, but we already
	// confirmed above that this request was already started and so we
	// can safely just await its result here.
	for _, inst := range r.Instances(grapheval.ContextWithNewWorker(context.Background())) {
		inst.AnnounceAllGraphevalRequests(announce)
	}
}
