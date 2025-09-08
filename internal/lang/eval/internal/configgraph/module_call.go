// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package configgraph

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/apparentlymart/go-workgraph/workgraph"
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/instances"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ModuleCall struct {
	Addr      addrs.AbsModuleCall
	DeclRange tfdiags.SourceRange

	// ParentSourceAddr is the source address of the module that contained
	// this module call, which is then used as the base for resolving
	// any relative addresses returned from SourceAddrValuer.
	ParentSourceAddr addrs.ModuleSource

	// InstanceSelector represents a rule for deciding which instances of
	// this resource have been declared.
	InstanceSelector InstanceSelector

	// SourceAddrValuer and VersionConstraintValuer together describe how
	// to select the module to be called.
	//
	// We currently require these to be equal for all instances of the
	// module call because although in principle this new evaluation model
	// could support entirely different declarations in each module, the
	// surface syntax of HCL would make that very hard to use (can't easily
	// set drastically different arguments for each instance) and this
	// also allows us to echo the design for resource instances where we've
	// effectively already baked in what schema we ought to be validating
	// against before we try to evaluate the config body inside
	// [ModuleCallInstance].
	SourceAddrValuer        *OnceValuer
	VersionConstraintValuer *OnceValuer

	// ValidateSourceArguments is a callback function provided by whatever
	// compiled this [ModuleCall] object that checks whether the source
	// arguments are resolvable in the current execution context, so that we
	// can report any problems just once at the call level rather than
	// re-reporting the same problems once for each instance.
	//
	// Depending on what phase we're in this could either try to find a
	// suitable module in a local cache directory or could even try to actually
	// fetch a remote module over the network, and so this function may take
	// a long time to return.
	ValidateSourceArguments func(ctx context.Context, sourceArgs ModuleSourceArguments) tfdiags.Diagnostics

	// CompileCallInstance is a callback function provided by whatever
	// compiled this [ModuleCall] object that knows how to produce a compiled
	// [ModuleCallInstance] object once we know of the instance key and
	// associated repetition data for it.
	//
	// This indirection allows the caller to take into account the same
	// context it had available when it built this [ModuleCall] object, while
	// incorporating the new information about this specific instance.
	//
	// This should only be called with a [ModuleSourceArguments] that was
	// accepted by [ModuleCall.ValidateSourceArguments] without returning any
	// errors.
	CompileCallInstance func(ctx context.Context, sourceArgs ModuleSourceArguments, key addrs.InstanceKey, repData instances.RepetitionData) *ModuleCallInstance

	// instancesResult tracks the process of deciding which instances are
	// currently declared for this provider config, and the result of that process.
	//
	// Only the decideInstances method accesses this directly. Use that
	// method to obtain the coalesced result for use elsewhere.
	instancesResult grapheval.Once[*compiledInstances[*ModuleCallInstance]]
}

var _ exprs.Valuer = (*ModuleCall)(nil)

// Instances returns the instances that are selected for this module call in
// its configuration, without evaluating their configuration objects yet.
func (c *ModuleCall) Instances(ctx context.Context) map[addrs.InstanceKey]*ModuleCallInstance {
	// We ignore the diagnostics here because they will be returned by
	// the Value method instead.
	result, _ := c.decideInstances(ctx)
	return result.Instances
}

func (c *ModuleCall) SourceArguments(ctx context.Context) (Maybe[ModuleSourceArguments], cty.ValueMarks, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	sourceVal, moreDiags := c.SourceAddrValuer.Value(ctx)
	diags = diags.Append(moreDiags)
	versionVal, moreDiags := c.VersionConstraintValuer.Value(ctx)
	diags = diags.Append(moreDiags)
	if diags.HasErrors() {
		// Not even valid enough to try anything else.
		return nil, nil, diags
	}

	// We'll decode both source address and version together so that we can
	// always return their combined marks even if one is invalid.
	const sourceDiagSummary = "Invalid module source address"
	const versionDiagSummary = "Invalid module version constraints"
	sourceStr, sourceMarks, sourceErr := decodeModuleCallSourceArgumentString(sourceVal, false)
	versionStr, versionMarks, versionErr := decodeModuleCallSourceArgumentString(versionVal, true)
	allMarks := make(cty.ValueMarks)
	maps.Copy(allMarks, sourceMarks)
	maps.Copy(allMarks, versionMarks)

	if sourceErr != nil {
		var detail string
		switch err := sourceErr.(type) {
		case moduleSourceDependsOnResourcesError:
			var buf strings.Builder
			fmt.Fprintln(&buf, "The module source address value is derived from the results of the following resource instances:")
			for _, addr := range err.instAddrs {
				fmt.Fprintf(&buf, "  - %s\n", addr)
			}
			fmt.Fprint(&buf, "\n\nModule source selections cannot be based on resource results because they must remain consistent throughout each plan/apply round.")
			detail = buf.String()
		case moduleSourceDependsOnUpstreamEvalError:
			// We intentionally don't generate any more diagnostics in this
			// case because we assume that something upstream will already
			// have reported error diagnostics for whatever caused this
			// error.
		default:
			detail = fmt.Sprintf("Unsuitable value for module source address: %s.", tfdiags.FormatError(err))
		}
		if detail != "" {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  sourceDiagSummary,
				Detail:   detail,
				Subject:  MaybeHCLSourceRange(c.SourceAddrValuer.ValueSourceRange()),
			})
		}
	}
	if versionErr != nil {
		var detail string
		switch err := sourceErr.(type) {
		case moduleSourceDependsOnResourcesError:
			var buf strings.Builder
			fmt.Fprintln(&buf, "The module version constraints value is derived from the results of the following resource instances:")
			for _, addr := range err.instAddrs {
				fmt.Fprintf(&buf, "  - %s\n", addr)
			}
			fmt.Fprint(&buf, "\n\nModule source selections cannot be based on resource results because they must remain consistent throughout each plan/apply round.")
			detail = buf.String()
		case moduleSourceDependsOnUpstreamEvalError:
			// We intentionally don't generate any more diagnostics in this
			// case because we assume that something upstream will already
			// have reported error diagnostics for whatever caused this
			// error.
		default:
			detail = fmt.Sprintf("Unsuitable value for module version constraints: %s.", tfdiags.FormatError(err))
		}
		if detail != "" {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  versionDiagSummary,
				Detail:   detail,
				Subject:  MaybeHCLSourceRange(c.SourceAddrValuer.ValueSourceRange()),
			})
		}
	}
	if diags.HasErrors() {
		return nil, allMarks, diags
	}

	sourceAddr, err := addrs.ParseModuleSource(sourceStr)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  sourceDiagSummary,
			Detail:   fmt.Sprintf("Cannot use %q as module source address: %s.", sourceStr, tfdiags.FormatError(err)),
			Subject:  MaybeHCLSourceRange(c.SourceAddrValuer.ValueSourceRange()),
		})
		return nil, allMarks, diags
	}
	// If the specified source address is a relative path then we need to
	// resolve it to absolute based on the source address where this
	// module call appeared.
	sourceAddr, err = addrs.ResolveRelativeModuleSource(c.ParentSourceAddr, sourceAddr)
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  sourceDiagSummary,
			Detail:   fmt.Sprintf("Cannot use %q as module source address: %s.", sourceStr, tfdiags.FormatError(err)),
			Subject:  MaybeHCLSourceRange(c.SourceAddrValuer.ValueSourceRange()),
		})
		return nil, allMarks, diags
	}

	// FIXME: It would be better if the rule for what source address types
	// are allowed to have version constraints would live somewhere else,
	// such as in "package addrs".
	var allowedVersions versions.Set
	if _, isRegistry := sourceAddr.(addrs.ModuleSourceRegistry); isRegistry {
		if versionStr != "" {
			vs, err := versions.MeetingConstraintsStringRuby(versionStr)
			if err != nil {
				diags = diags.Append(&hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  versionDiagSummary,
					Detail:   fmt.Sprintf("Cannot use %q as module version constraints: %s.", versionStr, tfdiags.FormatError(err)),
					Subject:  MaybeHCLSourceRange(c.VersionConstraintValuer.ValueSourceRange()),
				})
				return nil, allMarks, diags
			}
			allowedVersions = vs
		} else {
			allowedVersions = versions.All
		}
	} else {
		if versionStr != "" {
			diags = diags.Append(&hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  versionDiagSummary,
				Detail:   fmt.Sprintf("Module source address %q does not support version constraints.", sourceAddr),
				Subject:  MaybeHCLSourceRange(c.VersionConstraintValuer.ValueSourceRange()),
			})
			return nil, allMarks, diags
		}
	}

	return Known(ModuleSourceArguments{
		Source:          sourceAddr,
		AllowedVersions: allowedVersions,
	}), allMarks, diags
}

func (c *ModuleCall) decideInstances(ctx context.Context) (*compiledInstances[*ModuleCallInstance], tfdiags.Diagnostics) {
	return c.instancesResult.Do(ctx, func(ctx context.Context) (*compiledInstances[*ModuleCallInstance], tfdiags.Diagnostics) {
		// We intentionally ignore diagnostics and marks here because Value
		// deals with those and skips calling this function at all when
		// the arguments are too invalid.
		maybeSourceArgs, _, _ := c.SourceArguments(ctx)
		sourceArgs, ok := GetKnown(maybeSourceArgs)
		if !ok {
			// For our purposes here we just use this as a signal that we
			// should not even try to select instances. [ModuleCall.Value]
			// also checks this and skips even making use of the instances
			// when the source arguments are invalid, and so we're just
			// returning something valid enough for other callers to unwind
			// successfully and move on here.
			return &compiledInstances[*ModuleCallInstance]{
				KeyType:    addrs.UnknownKeyType,
				Instances:  nil,
				ValueMarks: nil,
			}, nil
		}
		return compileInstances(ctx, c.InstanceSelector, func(ctx context.Context, key addrs.InstanceKey, repData instances.RepetitionData) *ModuleCallInstance {
			return c.CompileCallInstance(ctx, sourceArgs, key, repData)
		})
	})
}

// StaticCheckTraversal implements exprs.Valuer.
func (c *ModuleCall) StaticCheckTraversal(traversal hcl.Traversal) tfdiags.Diagnostics {
	return staticCheckTraversalForInstances(c.InstanceSelector, traversal)
}

// Value implements exprs.Valuer.
func (c *ModuleCall) Value(ctx context.Context) (cty.Value, tfdiags.Diagnostics) {
	// We'll first check whether the arguments specifying which module to
	// call are valid, because we can't really do anything else if not.
	maybeSourceArgs, sourceMarks, diags := c.SourceArguments(ctx)
	sourceArgs, ok := GetKnown(maybeSourceArgs)
	if !ok {
		// Either the source information was invalid or was based on something
		// that failed evaluation upstream, so we'll just bail out here.
		// NOTE: It's possible for sourceArgs to be nil while diags does not
		// have errors: we react in this way when the source information is
		// unknown because it was derived from something that failed upstream,
		// with the assumption that the upstream error generated its own
		// diagnostics that'll come via another return path.
		return cty.DynamicVal.WithMarks(sourceMarks), diags
	}

	// We'll validate that the source arguments are acceptable before we
	// try to instantiate any instances, because otherwise we'd likely
	// detect and report the same problems separately for each instance.
	moreDiags := c.ValidateSourceArguments(ctx, sourceArgs)
	diags = diags.Append(moreDiags)
	if moreDiags.HasErrors() {
		return cty.DynamicVal.WithMarks(sourceMarks), diags
	}

	// If the source args were acceptable then we'll decide what instances
	// we have and then collect their individual results into our overall
	// return value.
	selection, diags := c.decideInstances(ctx)
	return valueForInstances(ctx, selection).WithMarks(sourceMarks), diags
}

// ValueSourceRange implements exprs.Valuer.
func (c *ModuleCall) ValueSourceRange() *tfdiags.SourceRange {
	return &c.DeclRange
}

// CheckAll implements allChecker.
func (c *ModuleCall) CheckAll(ctx context.Context) tfdiags.Diagnostics {
	var cg CheckGroup
	// Our InstanceSelector itself might block on expression evaluation,
	// so we'll run it async as part of the checkGroup.
	cg.Await(ctx, func(ctx context.Context) {
		for _, inst := range c.Instances(ctx) {
			cg.CheckValuer(ctx, inst)
		}
	})
	// This is where an invalid for_each expression would be reported.
	cg.CheckValuer(ctx, c)
	return cg.Complete(ctx)
}

func (c *ModuleCall) AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo)) {
	announce(c.SourceAddrValuer.RequestID(), grapheval.RequestInfo{
		Name:        c.Addr.String() + " source address",
		SourceRange: c.SourceAddrValuer.ValueSourceRange(),
	})
	announce(c.VersionConstraintValuer.RequestID(), grapheval.RequestInfo{
		Name:        c.Addr.String() + " version constraint",
		SourceRange: c.VersionConstraintValuer.ValueSourceRange(),
	})

	// There might be other grapheval requests in our dynamic instances, but
	// they are hidden behind another request themselves so we'll try to
	// report them only if that request was already started.
	instancesReqId := c.instancesResult.RequestID()
	if instancesReqId == workgraph.NoRequest {
		return
	}
	announce(instancesReqId, grapheval.RequestInfo{
		Name:        fmt.Sprintf("decide instances for %s", c.Addr),
		SourceRange: c.InstanceSelector.InstancesSourceRange(),
	})
	// The Instances method potentially starts a new request, but we already
	// confirmed above that this request was already started and so we
	// can safely just await its result here.
	for _, inst := range c.Instances(grapheval.ContextWithNewWorker(context.Background())) {
		inst.AnnounceAllGraphevalRequests(announce)
	}
}

// decodeModuleCallSourceArgumentString deals with the various requirements
// that apply to both the source address _and_ version constraints strings
// in a module call, producing an error that can be used as part of an
// error diagnostic saying that the argument is invalid.
//
// If nullAllowed is true then a null value is represented by returning an
// empty string. An _actual_ empty string is always rejected as invalid input,
// so a successful result with an empty string always means that the given
// value was null.
func decodeModuleCallSourceArgumentString(v cty.Value, nullAllowed bool) (string, cty.ValueMarks, error) {
	v, err := convert.Convert(v, cty.String)
	retV, retMarks := v.Unmark()
	if err != nil {
		return "", nil, err
	}
	if v.IsNull() {
		if nullAllowed {
			// Note that this is the only case where the result can be ""
			// without returning an error, because we reject an _actual_
			// empty string with an error later in this function.
			return "", retMarks, nil
		}
		return "", retMarks, errors.New("value must not be null")
	}
	if ris := slices.Collect(ResourceInstanceAddrs(ContributingResourceInstances(v))); len(ris) != 0 {
		// A module source argument is never allowed to depend on a resource
		// instance result, even if its value happens to be known in the
		// current evaluation context.
		return "", retMarks, moduleSourceDependsOnResourcesError{ris}
	}
	if !v.IsKnown() {
		// Although resource instance references are the main reason for
		// it to be unknown, it could also be unknown if it were derived
		// from an impure function during the planning phase, or from an
		// input variable that was set to an unknown value.
		//
		// We use a special error type when the EvalError mark is present
		// because in that case we want to be quiet and avoid distracting
		// the user with an error message about unknown values when the
		// unknown values were likely generated by OpenTofu itself as
		// an implementation detail, rather than by a module authoring mistake.
		if exprs.IsEvalError(v) {
			return "", nil, moduleSourceDependsOnUpstreamEvalError{}
		}
		return "", retMarks, errors.New("depends on a value that won't be known until the apply phase")
	}
	ret := retV.AsString()
	if ret == "" {
		return "", retMarks, errors.New("must be a non-empty string")
	}
	return ret, retMarks, nil
}

type moduleSourceDependsOnResourcesError struct {
	instAddrs []addrs.AbsResourceInstance
}

// Error implements error, though it returns only a generic error message
// that should be substituted for something better when building a user-facing
// diagnostic,
func (m moduleSourceDependsOnResourcesError) Error() string {
	return "must not be derived from resource instance attributes"
}

type moduleSourceDependsOnUpstreamEvalError struct{}

// Error implements error.
func (m moduleSourceDependsOnUpstreamEvalError) Error() string {
	return "derived from upstream expression that was invalid"
}
