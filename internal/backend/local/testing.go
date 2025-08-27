// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/providers"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statemgr"
	"github.com/opentofu/opentofu/internal/tofu"
)

// TestLocal returns a configured Local struct with temporary paths and
// in-memory ContextOpts.
//
// No operations will be called on the returned value, so you can still set
// public fields without any locks.
func TestLocal(t *testing.T) *Local {
	t.Helper()
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	local := New(encryption.StateEncryptionDisabled())
	local.StatePath = filepath.Join(tempDir, "state.tfstate")
	local.StateOutPath = filepath.Join(tempDir, "state.tfstate")
	local.StateBackupPath = filepath.Join(tempDir, "state.tfstate.bak")
	local.StateWorkspaceDir = filepath.Join(tempDir, "state.tfstate.d")
	local.ContextOpts = &tofu.ContextOpts{}

	t.Cleanup(func() {
		// Force garbage collection to help release any remaining
		// file handles. This avoids TempDir RemoveAll cleanup errors
		// on Windows.
		runtime.GC()
	})

	return local
}

// TestLocalProvider modifies the ContextOpts of the *Local parameter to
// have a provider with the given name.
func TestLocalProvider(t *testing.T, b *Local, name string, schema providers.ProviderSchema) *tofu.MockProvider {
	// Build a mock resource provider for in-memory operations
	p := new(tofu.MockProvider)

	p.GetProviderSchemaResponse = &schema

	p.PlanResourceChangeFn = func(req providers.PlanResourceChangeRequest) (resp providers.PlanResourceChangeResponse) {
		// this is a destroy plan,
		if req.ProposedNewState.IsNull() {
			resp.PlannedState = req.ProposedNewState
			resp.PlannedPrivate = req.PriorPrivate
			return resp
		}

		rSchema, _ := schema.SchemaForResourceType(addrs.ManagedResourceMode, req.TypeName)
		if rSchema == nil {
			rSchema = &configschema.Block{} // default schema is empty
		}
		plannedVals := map[string]cty.Value{}
		for name, attrS := range rSchema.Attributes {
			val := req.ProposedNewState.GetAttr(name)
			if attrS.Computed && val.IsNull() {
				val = cty.UnknownVal(attrS.Type)
			}
			plannedVals[name] = val
		}
		for name := range rSchema.BlockTypes {
			// For simplicity's sake we just copy the block attributes over
			// verbatim, since this package's mock providers are all relatively
			// simple -- we're testing the backend, not esoteric provider features.
			plannedVals[name] = req.ProposedNewState.GetAttr(name)
		}

		return providers.PlanResourceChangeResponse{
			PlannedState:   cty.ObjectVal(plannedVals),
			PlannedPrivate: req.PriorPrivate,
		}
	}
	p.ReadResourceFn = func(req providers.ReadResourceRequest) providers.ReadResourceResponse {
		return providers.ReadResourceResponse{NewState: req.PriorState}
	}
	p.ReadDataSourceFn = func(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
		return providers.ReadDataSourceResponse{State: req.Config}
	}

	// Initialize the opts
	if b.ContextOpts == nil {
		b.ContextOpts = &tofu.ContextOpts{}
	}

	// Set up our provider
	b.ContextOpts.Providers = map[addrs.Provider]providers.Factory{
		addrs.NewDefaultProvider(name): providers.FactoryFixed(p),
	}

	return p

}

// TestLocalSingleState is a backend implementation that wraps Local
// and modifies it to only support single states (returns
// ErrWorkspacesNotSupported for multi-state operations).
//
// This isn't an actual use case, this is exported just to provide a
// easy way to test that behavior.
type TestLocalSingleState struct {
	*Local
}

// TestNewLocalSingle is a factory for creating a TestLocalSingleState.
// This function matches the signature required for backend/init.
func TestNewLocalSingle(enc encryption.StateEncryption) backend.Backend {
	return &TestLocalSingleState{Local: New(encryption.StateEncryptionDisabled())}
}

func (b *TestLocalSingleState) Workspaces(context.Context) ([]string, error) {
	return nil, backend.ErrWorkspacesNotSupported
}

func (b *TestLocalSingleState) DeleteWorkspace(context.Context, string, bool) error {
	return backend.ErrWorkspacesNotSupported
}

func (b *TestLocalSingleState) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	if name != backend.DefaultStateName {
		return nil, backend.ErrWorkspacesNotSupported
	}

	return b.Local.StateMgr(ctx, name)
}

// TestLocalNoDefaultState is a backend implementation that wraps
// Local and modifies it to support named states, but not the
// default state. It returns ErrDefaultWorkspaceNotSupported when
// the DefaultStateName is used.
type TestLocalNoDefaultState struct {
	*Local
}

// TestNewLocalNoDefault is a factory for creating a TestLocalNoDefaultState.
// This function matches the signature required for backend/init.
func TestNewLocalNoDefault(enc encryption.StateEncryption) backend.Backend {
	return &TestLocalNoDefaultState{Local: New(encryption.StateEncryptionDisabled())}
}

func (b *TestLocalNoDefaultState) Workspaces(ctx context.Context) ([]string, error) {
	workspaces, err := b.Local.Workspaces(ctx)
	if err != nil {
		return nil, err
	}

	filtered := workspaces[:0]
	for _, name := range workspaces {
		if name != backend.DefaultStateName {
			filtered = append(filtered, name)
		}
	}

	return filtered, nil
}

func (b *TestLocalNoDefaultState) DeleteWorkspace(ctx context.Context, name string, force bool) error {
	if name == backend.DefaultStateName {
		return backend.ErrDefaultWorkspaceNotSupported
	}
	return b.Local.DeleteWorkspace(ctx, name, force)
}

func (b *TestLocalNoDefaultState) StateMgr(ctx context.Context, name string) (statemgr.Full, error) {
	if name == backend.DefaultStateName {
		return nil, backend.ErrDefaultWorkspaceNotSupported
	}
	return b.Local.StateMgr(ctx, name)
}

func testStateFile(t *testing.T, path string, s *states.State) {
	t.Helper()

	if err := statemgr.WriteAndPersist(t.Context(), statemgr.NewFilesystem(path, encryption.StateEncryptionDisabled()), s, nil); err != nil {
		t.Fatal(err)
	}
}

func mustProviderConfig(s string) addrs.AbsProviderConfig {
	p, diags := addrs.ParseAbsProviderConfigStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return p
}

func mustResourceInstanceAddr(s string) addrs.AbsResourceInstance {
	addr, diags := addrs.ParseAbsResourceInstanceStr(s)
	if diags.HasErrors() {
		panic(diags.Err())
	}
	return addr
}

// assertBackendStateUnlocked attempts to lock the backend state for a test.
// Failure indicates that the state was locked and false is returned.
// True is returned if a lock was obtained.
func assertBackendStateUnlocked(t *testing.T, b *Local) bool {
	t.Helper()
	stateMgr, _ := b.StateMgr(t.Context(), backend.DefaultStateName)
	if _, err := stateMgr.Lock(t.Context(), statemgr.NewLockInfo()); err != nil {
		t.Errorf("state is already locked: %s", err.Error())
		// lock was obtained
		return false
	}
	// lock was not obtained
	return true
}

// assertBackendStateLocked attempts to lock the backend state for a test.
// Failure indicates that the state was not locked and false is returned.
// True is returned if a lock was not obtained.
func assertBackendStateLocked(t *testing.T, b *Local) bool {
	t.Helper()
	stateMgr, _ := b.StateMgr(t.Context(), backend.DefaultStateName)
	if _, err := stateMgr.Lock(t.Context(), statemgr.NewLockInfo()); err != nil {
		// lock was not obtained
		return true
	}
	t.Error("unexpected success locking state")
	// lock was obtained
	return false
}
