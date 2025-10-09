// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package execgraph

// operationDesc is a low-level description of an operation that can be saved in
// serialized form for reloading later and can be "compiled" into a form
// suitable for execution once the full graph execution graph has been built.
type operationDesc struct {
	opCode   opCode
	operands []AnyResultRef
}

// opCode is an enumeration of all of the different operation types that
// can appear in an execution graph.
//
// This does not represent the actual implementation of each opCode. The
// descriptions of operations are "compiled" into executable functions as a
// separate step after assembling the execution graph piecemeal during the
// planning process.
type opCode int

const (
	_ = opCode(iota) // the zero value is not a valid operation
	// opManagedFinalPlan uses the configuration value and initial planned state
	// for a resource instance to produce its final plan, which can then
	// be applied by [opApplyChanges].
	opManagedFinalPlan
	// opManagedApplyChanges applies a plan created by [opFinalPlan].
	opManagedApplyChanges
	// opDataRead reads a data resource.
	opDataRead
	// opOpenProvider takes a provider address and a configuration value
	// and produces a configured client for the specified provider.
	opOpenProvider
	// opCloseProvider takes a client previously created by [opOpenProvider],
	// along with a "waiter" node that resolves only once all uses of the
	// provider client are done, and closes the client once the waiter node
	// has resolved.
	opCloseProvider
)

//go:generate go run golang.org/x/tools/cmd/stringer -type=opCode
