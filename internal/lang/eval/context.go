// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package eval

// EvalContext is a collection of contextual information provided by an
// external caller of this package to help it to interact with the surrounding
// environment.
type EvalContext struct {
	// This type should only contain broad stuff that'd typically be set up
	// only once for a particular OpenTofu CLI command, and NOT
	// operation-specific things like the input variables provided for a
	// given module, or where state is supposed to be stored, etc.

	// Modules gives access to all of the modules available for use in
	// this context.
	Modules ExternalModules

	// Providers gives access to all of the providers available for use
	// in this context.
	Providers Providers

	// Provisioners gives access to all of the provisioners available for
	// use in this context.
	Provisioners Provisioners
}

// init must be called early on entry to any exported function that accepts
// an [EvalContext] as an argument to prepare it for use, before accessing
// any of its fields or calling any of its other methods.
func (c *EvalContext) init() {
	// If any of the external dependency fields were left nil (likely in
	// unit tests which aren't intending to use a particular kind of dependency)
	// we'll replace it with a non-nil implementation that just returns an
	// error immediately on call, so that accidental reliance on these will
	// return an error instead of panicking.
	//
	// "Real" callers (performing operations on behalf of end-users) should
	// avoid relying on this because it returns low-quality error messages.
	c.Modules = ensureExternalModules(c.Modules)
	c.Providers = ensureProviders(c.Providers)
	c.Provisioners = ensureProvisioners(c.Provisioners)
}
