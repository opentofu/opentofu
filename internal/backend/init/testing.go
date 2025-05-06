// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package init

import (
	"context"
	"fmt"
	"strings"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// RegisterTemp adds a new entry to the table of backends and returns a
// function that will deregister it once called.
//
// This is essentially a workaround for the fact that the OpenTofu CLI
// layer expects backends to always come from a centrally-maintained table
// and doesn't have any way to directly pass an anonymous backend
// implementation.
//
// The given name MUST start with an underscore, to ensure that it cannot
// collide with any "real" backend. If the given name does not start with
// an underscore then this function will panic. If we introduce plugin-based
// backends in future then we might consider reserving part of the plugin
// address namespace to represent temporary backends for test purposes only,
// which would then replace this special underscore prefix as the differentiator.
//
// This is intended for unit tests that use [MockBackend], or a derivative
// thereof. A typical usage pattern from a unit test would be:
//
//	// ("backendInit" represents _this_ package)
//	t.Cleanup(backendInit.RegisterTemp("_test", func (enc encryption.StateEncryption) Backend {
//		return &backendInit.MockBackend{
//			// (and then whichever mock settings your test case needs)
//		}
//	}))
//
// Because this function modifies global state observable throughout the
// program, any test using this function MUST NOT use t.Parallel. If a
// single test needs to register multiple temporary backends for some reason
// then it must select a different name for each one.
func RegisterTemp(name string, f backend.InitFn) func() {
	// FIXME: It would be better to add a map of backends to command.Meta's existing
	// "testingOverrides" field, but at the time of writing direct calls to this
	// package's func Backend are made from too many different places in the
	// codebase to retrofit "testing overrides" without considerable risk to
	// already-working code, so for now we compromise and just offer this helper
	// function to hopefully help tests properly manage their temporary addition
	// to the global backend table.

	if !strings.HasPrefix(name, "_") {
		panic("temporary backend name must begin with underscore")
	}
	backendsLock.Lock()
	defer backendsLock.Unlock()

	if _, exists := backends[name]; exists {
		// If we get in here then it suggests one of the following mistakes in
		// the calling code:
		// - Using RegisterTemp in a test case that uses t.Parallel.
		// - Forgetting to call the cleanup function in some other earlier test that
		//   happened to choose the same temporary name.
		// - Registering more than one temporary backend in a single test without
		//   assigning each one a unique name.
		panic(fmt.Sprintf("there is already a temporary backend named %q", name))
	}

	// The given init function is temporarily added to the global table, so that
	// the CLI package can find it using the given (underscore-prefixed) name.
	backends[name] = f

	return func() {
		backendsLock.Lock()
		delete(backends, name)
		backendsLock.Unlock()
	}
}

// MockBackend is an implementation of [Backend] that largely just routes incoming
// calls to a set of closures provided by a caller.
//
// This is included for testing purposes only. Use [RegisterTemp] to
// temporarily add a MockBackend instance to the table of available backends
// from a unit test function. Do not include MockBackend instances in the
// initial static backend table.
//
// The mock automatically tracks the most recent call to each method for ease
// of writing assertions in simple cases. If you need more complex tracking such
// as a log of all calls then you can implement that inside your provided callback
// functions.
//
// This implementation intentionally covers only the basic [Backend] interface,
// and not any extension interfaces like [CLI] and [Enhanced]. Consider embedding
// this into another type if you need to mock extension interfaces too, since
// OpenTofu backend init uses type assertions to check for extension interfaces
// and so having this type implement them would prevent its use in testing
// situations that occur with non-extended backend implementations.
type MockBackend struct {
	// If you add support for new methods here in future, please preserve the
	// alphabetical order by function name and the other naming suffix conventions
	// for each field.

	ConfigSchemaFn     func() *configschema.Block
	ConfigSchemaCalled bool

	ConfigureFn        func(configObj cty.Value) tfdiags.Diagnostics
	ConfigureCalled    bool
	ConfigureConfigObj cty.Value

	DeleteWorkspaceFn     func(name string, force bool) error
	DeleteWorkspaceCalled bool
	DeleteWorkspaceName   string
	DeleteWorkspaceForce  bool

	PrepareConfigFn        func(configObj cty.Value) (cty.Value, tfdiags.Diagnostics)
	PrepareConfigCalled    bool
	PrepareConfigConfigObj cty.Value

	StateMgrFn        func(workspace string) (statemgr.Full, error)
	StateMgrCalled    bool
	StateMgrWorkspace string

	WorkspacesFn     func() ([]string, error)
	WorkspacesCalled bool
}

var _ backend.Backend = (*MockBackend)(nil)

// ConfigSchema implements Backend.
func (m *MockBackend) ConfigSchema() *configschema.Block {
	m.ConfigSchemaCalled = true

	if m.ConfigSchemaFn == nil {
		// Default behavior: return an empty schema
		return &configschema.Block{}
	}
	return m.ConfigSchemaFn()
}

// Configure implements Backend.
func (m *MockBackend) Configure(ctx context.Context, configObj cty.Value) tfdiags.Diagnostics {
	m.ConfigureCalled = true
	m.ConfigureConfigObj = configObj

	if m.ConfigureFn == nil {
		// Default behavior: do nothing at all, and report success
		return nil
	}
	return m.ConfigureFn(configObj)
}

// DeleteWorkspace implements Backend.
func (m *MockBackend) DeleteWorkspace(_ context.Context, name string, force bool) error {
	m.DeleteWorkspaceCalled = true
	m.DeleteWorkspaceName = name
	m.DeleteWorkspaceForce = force

	if m.DeleteWorkspaceFn == nil {
		// Default behavior: do nothing at all, and report success
		return nil
	}
	return m.DeleteWorkspaceFn(name, force)
}

// PrepareConfig implements Backend.
func (m *MockBackend) PrepareConfig(configObj cty.Value) (cty.Value, tfdiags.Diagnostics) {
	m.PrepareConfigCalled = true
	m.PrepareConfigConfigObj = configObj

	if m.PrepareConfigFn == nil {
		// Default behavior: just echo back the given config object and indicate success
		return configObj, nil
	}
	return m.PrepareConfigFn(configObj)
}

// StateMgr implements Backend.
func (m *MockBackend) StateMgr(_ context.Context, workspace string) (statemgr.Full, error) {
	m.StateMgrCalled = true
	m.StateMgrWorkspace = workspace

	if m.StateMgrFn == nil {
		// Default behavior: fail as if there is no workspace of the given name
		return nil, fmt.Errorf("no workspace named %q", workspace)
	}
	return m.StateMgrFn(workspace)
}

// Workspaces implements Backend.
func (m *MockBackend) Workspaces(context.Context) ([]string, error) {
	m.WorkspacesCalled = true

	if m.WorkspacesFn == nil {
		// Default behavior: report no workspaces at all.
		return nil, nil
	}
	return m.WorkspacesFn()
}
