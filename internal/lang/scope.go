// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package lang

import (
	"context"
	"sync"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/experiments"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

type ParseRef func(traversal hcl.Traversal) (*addrs.Reference, tfdiags.Diagnostics)

// Scope is the main type in this package, allowing dynamic evaluation of
// blocks and expressions based on some contextual information that informs
// which variables and functions will be available.
type Scope struct {
	// Data is used to resolve references in expressions.
	Data Data

	// ParseRef is a function that the scope uses to extract references from
	// a hcl.Traversal. This controls the type of references the scope currently
	// supports. As an example, the testing scope can reference outputs directly
	// while the main OpenTofu context scope can not. This means that this
	// function for the testing scope will happily return outputs, while the
	// main context scope would fail if a user attempts to reference an output.
	ParseRef ParseRef

	// SelfAddr is the address that the "self" object should be an alias of,
	// or nil if the "self" object should not be available at all.
	SelfAddr addrs.Referenceable

	// SourceAddr is the address of the source item for the scope. This will
	// affect any scoped resources that can be accessed from within this scope.
	//
	// If nil, access is assumed to be at the module level. So, in practice this
	// only needs to be set for items that should be able to access something
	// hidden in their own scope.
	SourceAddr addrs.Referenceable

	// BaseDir is the base directory used by any interpolation functions that
	// accept filesystem paths as arguments.
	BaseDir string

	// PureOnly can be set to true to request that any non-pure functions
	// produce unknown value results rather than actually executing. This is
	// important during a plan phase to avoid generating results that could
	// then differ during apply.
	PureOnly bool

	funcsLock sync.Mutex
	funcs     map[string]function.Function

	// activeExperiments is an optional set of experiments that should be
	// considered as active in the module that this scope will be used for.
	// Callers can populate it by calling the SetActiveExperiments method.
	activeExperiments experiments.Set

	// ConsoleMode can be set to true to request any console-only functions are
	// included in this scope.
	ConsoleMode bool

	// PlanTimestamp is a timestamp representing when the plan was made. It will
	// either have been generated during this operation or read from the plan.
	PlanTimestamp time.Time

	ProviderFunctions ProviderFunction
}

type ProviderFunction func(context.Context, addrs.ProviderFunction, tfdiags.SourceRange) (*function.Function, tfdiags.Diagnostics)

// SetActiveExperiments allows a caller to declare that a set of experiments
// is active for the module that the receiving Scope belongs to, which might
// then cause the scope to activate some additional experimental behaviors.
func (s *Scope) SetActiveExperiments(active experiments.Set) {
	s.activeExperiments = active
}
