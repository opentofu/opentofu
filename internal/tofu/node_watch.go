// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// WatchRequest represents a request for notification of the final value
// associated with a given reference, once that value has been settled.
type WatchRequest struct {
	// Ref is the reference to notify about. This must be a valid result
	// from [addrs.ParseAbsRef], and not nil.
	Ref *addrs.AbsReference

	// Notify is the function to call once the value has been settled.
	// This must not be nil.
	Notify func(*addrs.AbsReference, cty.Value)
}

// nodeWatch is the graph node type that handles "-watch" options from the
// command line.
//
// Nodes of this type depend on whatever object their reference address
// refers to and then, when executed, evaluate the value associated with
// the reference address and report it via a hook so that the UI can
// display it.
type nodeWatch struct {
	req WatchRequest
}

var _ GraphNodeExecutable = (*nodeWatch)(nil)
var _ GraphNodeModulePath = (*nodeWatch)(nil)
var _ GraphNodeModuleInstance = (*nodeWatch)(nil)
var _ GraphNodeReferencer = (*nodeWatch)(nil)

func (n *nodeWatch) Name() string {
	return "watch " + n.req.Ref.DisplayString()
}

// ModulePath implements GraphNodeModulePath.
func (n *nodeWatch) ModulePath() addrs.Module {
	return n.req.Ref.Module.Module()
}

// Path implements GraphNodeModuleInstance.
func (n *nodeWatch) Path() addrs.ModuleInstance {
	return n.req.Ref.Module
}

// References implements GraphNodeReferencer.
func (n *nodeWatch) References() []*addrs.Reference {
	return []*addrs.Reference{n.req.Ref.LocalReference()}
}

// Execute implements GraphNodeExecutable.
func (n *nodeWatch) Execute(_ context.Context, evalCtx EvalContext, op walkOperation) tfdiags.Diagnostics {
	// Due to how our Path and References methods are implemented, we should
	// get in here only after other graph nodes have dealt with the decisions
	// and/or side-effects related to the subject of our reference, and
	// evalCtx should be associated with the module instance that our reference
	// relates to.
	localRef := n.req.Ref.LocalReference()
	path := localRef.Path()
	if moduleInstAddr := evalCtx.Path(); !moduleInstAddr.IsRoot() {
		log.Printf("[TRACE] nodeWatch: evaluating %s in %s", localRef.DisplayString(), moduleInstAddr)
	} else {
		log.Printf("[TRACE] nodeWatch: evaluating %s in the root module", localRef.DisplayString())
	}

	// The following is a little weird because apparently at some point we
	// broke Scope.EvalReference so it only supports references to resource
	// instances, despite claiming to accept all addrs.Referencable. :(

	scope := evalCtx.EvaluationScope(nil, nil, EvalDataForNoInstanceKey)
	hclCtx, diags := scope.EvalContext([]*addrs.Reference{localRef})
	if diags.HasErrors() {
		return diags
	}
	val := hclCtx.Variables[path[0].(cty.GetAttrStep).Name]
	// We'll step through the rest of the path using HCL's implementations
	// of GetAttr/Index, so we'll be consistent with how this would've been
	// evaluated in a normal HCL expression.
	remainStart := len(path) - len(localRef.Remaining)
	for i, step := range path {
		if i == 0 {
			continue // we already dealt with the first step above
		}
		sourceRange := localRef.SourceRange.ToHCL().Ptr()
		if i >= remainStart {
			// We only have detailed source information for the "remaining"
			// part of the reference, but that's an okay compromise because
			// that's the part most likely to cause dynamic errors: the leading
			// part got a bunch of extra checks applied to it during parsing.
			sourceRange = localRef.Remaining[i-remainStart].SourceRange().Ptr()
		}
		switch step := step.(type) {
		case cty.GetAttrStep:
			nextVal, moreDiags := hcl.GetAttr(val, step.Name, sourceRange)
			diags = diags.Append(moreDiags)
			val = nextVal
		case cty.IndexStep:
			nextVal, moreDiags := hcl.Index(val, step.Key, sourceRange)
			diags = diags.Append(moreDiags)
			val = nextVal
		default:
			// Should not get here because there aren't any other step types.
			diags = diags.Append(fmt.Errorf("unsupported cty step type %T", step))
			return diags
		}
	}
	if diags.HasErrors() {
		return diags
	}

	// If evaluation was successful then we'll notify the callback given
	// in the request.
	n.req.Notify(n.req.Ref, val)

	return diags
}

// watchTransformer is a [GraphTransformer] that adds one node for each
// of the given watch requests.
//
// This must run after all referenceable objects have been added to the graph
// but before the reference transformer, so that the reference transformer will
// arrange for it to depend on the node(s) that will decide the value that
// will ultimately be reported.
type watchTransformer struct {
	requests []WatchRequest
}

var _ GraphTransformer = (*watchTransformer)(nil)

// Transform implements GraphTransformer.
func (w *watchTransformer) Transform(g *Graph) error {
	for _, req := range w.requests {
		if req.Ref == nil || req.Notify == nil {
			// This is a bug in the caller outside of this package, since
			// WatchRequest objects should always be constructed properly.
			// We'll catch this here to ensure it gets caught even if the
			// corresponding graph node isn't reached during the walk.
			return fmt.Errorf("malformed WatchRequest")
		}

		g.Add(&nodeWatch{
			req: req,
		})
	}
	return nil
}
