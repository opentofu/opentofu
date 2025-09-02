// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

// evaluationGlue is an interface used internally by this package to deal
// with situations where evaluation relies on the results of side-effects
// managed outside of this package, so that our generalized evaluation
// logic can get the information it needs in a generic way.
//
// We export higher-level interfaces like [PlanGlue] and [ApplyGlue] that
// are more tailored for specific operations, and then we use implementations
// of [evaluationGlue] to adapt that into the minimal set of operations
// that are needed regardless of what overall operation we're currently driving.
type evaluationGlue interface {
	// ...
}
