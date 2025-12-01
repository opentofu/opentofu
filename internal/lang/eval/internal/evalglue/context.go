// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package evalglue

import (
	"testing"
)

// EvalContext is a collection of contextual information provided by an
// external caller of this package to help it to interact with the surrounding
// environment.
//
// This type should be used only for settings that would typically remain
// equivalent throughout an entire validate/plan/apply round, and NOT for
// phase-specific settings. Specifically, it should be possible for a caller
// like the OpenTofu CLI layer to construct a single [EvalContext] object
// based on whole-process concerns like command line arguments and the current
// CLI configuration, and then reuse it without modification across multiple
// calls into different execution phases.
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
	Providers ProvidersSchema

	// Provisioners gives access to all of the provisioners available for
	// use in this context.
	Provisioners ProvisionersSchema

	// RootModuleDir and OriginalWorkingDir both represent local filesystem
	// directories whose paths are exposed in various ways to expressions
	// in modules.
	//
	// RootModuleDir is the local directory containing the root module, to
	// be used for "path.root" and for the base directory of
	// filesystem-interacting core functions, while OriginalWorkingDir
	// is used for "path.cwd". These tend to differ only when someone uses
	// the "-chdir" command line option, which causes RootModuleDir to change
	// but leaves OriginalWorkingDir unchanged.
	RootModuleDir, OriginalWorkingDir string
}

// AssertValid must be called early on entry to any exported function that
// accepts an [EvalContext] from outside of the lang/eval family of packages,
// to fail quickly if the caller has not populated the fields in a valid
// way.
func (c *EvalContext) AssertValid() {
	// ----------------------------------------------------------------------
	// NOTE: If you're looking here because you tried to write a test that
	// panicked on one of the following, consider whether
	// [EvalContextForTesting] would be a good compromise for your test so
	// that we can add new requirements here in future without having to
	// immediatately update all of the tests to conform to them.
	// ----------------------------------------------------------------------

	if c.Modules == nil {
		panic("EvalContext.Modules must be set")
	}
	if c.Providers == nil {
		panic("EvalContext.Providers must be set")
	}
	if c.Provisioners == nil {
		panic("EvalContext.Provisioners must be set")
	}
	if c.RootModuleDir == "" {
		panic("EvalContext.RootModuleDir must be set")
	}
	if c.OriginalWorkingDir == "" {
		panic("EvalContext.OriginalWorkingDir must be set")
	}
}

// EvalContextForTesting is a test-only helper which takes a
// possibly-partially-initialized [EvalContext] and substitutes some reasonable
// inert test implementations for anything that isn't populated.
//
// Passing a nil pointer is allowed and means that the calling test doesn't
// have any specific requirements for any part of the EvalContext and so
// _everything_ should be set to the inert defaults.
//
// This is intended to reduce the maintenence overhead for our tests as we
// grow EvalContext over time while still forcing "normal" code to be updated
// to set whatever new fields are required in future. Do not use this outside
// of test code, but also avoid changing this in ways that are overly-tailored
// for the needs of individual tests; add more specific test helpers if a family
// of tests have a common need but that need does not generalize across tests
// of many different parts of the system.
//
// If any changes need to be made then the given EvalContext may be modified
// in-place, but callers should not rely on this and should instead use
// the return value as their final EvalContext.
func EvalContextForTesting(t testing.TB, initial *EvalContext) *EvalContext {
	t.Helper()

	ret := initial
	if ret == nil {
		ret = &EvalContext{}
	}

	ret.Modules = ensureExternalModules(ret.Modules)
	ret.Providers = ensureProviders(ret.Providers)
	ret.Provisioners = ensureProvisioners(ret.Provisioners)

	if ret.RootModuleDir == "" {
		ret.RootModuleDir = "."
	}
	if ret.OriginalWorkingDir == "" {
		ret.OriginalWorkingDir = "."
	}

	return ret
}
