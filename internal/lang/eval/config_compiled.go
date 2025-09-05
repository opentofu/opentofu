// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

import (
	"context"
	"iter"

	"github.com/opentofu/opentofu/internal/lang/eval/internal/configgraph"
)

// compiledModuleInstance is the interface implemented by the top-level object
// that our "module compiler" layer returns, which this package's exported
// functions use to access [configgraph] objects whose results we translate to
// the package's public API.
//
// A [compiledModuleInstance] represents a top-level module instance _and_ all
// of the module instances beneath it. This is what ties together all of the
// [configgraph] nodes representing a whole configuration tree to provide
// global information.
type compiledModuleInstance interface {
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
}
