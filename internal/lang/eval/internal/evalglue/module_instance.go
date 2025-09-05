// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"context"
	"iter"

	"github.com/apparentlymart/go-workgraph/workgraph"

	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
	"github.com/opentofu/opentofu/internal/lang/exprs"
	"github.com/opentofu/opentofu/internal/lang/grapheval"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

// CompiledModuleInstance is the interface implemented by the top-level object
// that our "module compiler" layer returns, which this package's exported
// functions use to access [configgraph] objects whose results we translate to
// the package's public API.
//
// A [CompiledModuleInstance] represents a top-level module instance _and_ all
// of the module instances beneath it. This is what ties together all of the
// [configgraph] nodes representing a whole configuration tree to provide
// global information. Logic in package eval typically interacts directly only
// with the [CompiledModuleInstance] object representing the root module of
// the configuration, but that object will internally use objects of this
// type to delegate to child module instances that might be using different
// implementations of this interface.
//
// This module instance boundary is therefore the layer that all language
// editions/experiments must agree on to some extent. Edition-based language
// evolution will need to carefully consider how to handle any feature that
// affects this API boundary between parent and child module instances, to
// ensure that modules of different editions are still able to interact
// successfully.
type CompiledModuleInstance interface {
	// CheckAll collects diagnostics for everything in this module instance
	// and its child module instances, recursively.
	//
	// All callers in package eval should call this method at some point on every
	// [CompiledModuleInstance] they construct, because this is the only
	// method that guarantees to visit everything declared in the configuration
	// and make sure it gets a chance to evaluate itself and return diagnostics.
	// (Other methods will typically only interact with parts of the config
	// that are relevant to the questions they are answering.)
	//
	// If the [Glue] implementation passed to this module instance has
	// operations that block on the completion of outside operations then this
	// function must run concurrently with those outside operations (i.e. in
	// a separate goroutine) because CheckAll will block until all of the
	// values in the configuration have been resolved, which means that e.g.
	// in the planning phase this won't return until all resources have been
	// planned and so their "planned new state" values have been decided.
	CheckAll(ctx context.Context) tfdiags.Diagnostics

	// ResultValuer returns the [exprs.Valuer] representing the module
	// instance's overall result value, which is what should be used to
	// represent this module instance when referred to in its parent module.
	//
	// In the current language this always has an object type whose attribute
	// names match the output values declared in the child module, but we
	// don't enforce that at this layer to allow the result to potentially
	// use special cty features like marks and unknown values when needed.
	// Callers require this to be a known object type should work defensively
	// to do _something_ reasonable -- even if just returning an error message
	// about the module instance returning an unsuitable value -- so that
	// the decisions here can potentially evolve in future without causing
	// panics or other misbehavior.
	ResultValuer(ctx context.Context) exprs.Valuer

	// ResourceInstancesDeep returns a sequence of all of the resource instances
	// declared throughout the configuration tree.
	//
	// Some of the enumerated objects will be placeholders for zero or more
	// instances where there isn't yet enough information to determine exactly
	// which dynamic instances are declared. The evaluator still makes a best
	// effort to provide approximate results for them so we can potentially
	// detect cases where something downstream is invalid regardless of the
	// final instance selections, but code from elsewhere that's driving the
	// planning phase will probably want to treat those in a special way, such
	// as returning an error saying that there isn't enough information to plan
	// or deferring everything that depends on the affected instances until a
	// later plan/apply round when hopefully more information will be available.
	ResourceInstancesDeep(ctx context.Context) iter.Seq[*configgraph.ResourceInstance]

	// AnnounceAllGraphevalRequests calls announce for each [grapheval.Once],
	// [OnceValuer], or other [workgraph.RequestID] anywhere in the tree under this
	// object.
	//
	// This is used only when [workgraph] detects a self-dependency or failure to
	// resolve and we want to find a nice human-friendly name and optional source
	// range to use to describe each of the requests that were involved in the
	// problem.
	AnnounceAllGraphevalRequests(announce func(workgraph.RequestID, grapheval.RequestInfo))
}
