// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hcltest"
	"github.com/mitchellh/cli"
	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/configs"
	"github.com/opentofu/opentofu/internal/configs/configschema"
	"github.com/opentofu/opentofu/internal/copy"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/plans"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"

	backendInit "github.com/opentofu/opentofu/internal/backend/init"
	backendLocal "github.com/opentofu/opentofu/internal/backend/local"
	backendInmem "github.com/opentofu/opentofu/internal/backend/remote-state/inmem"
)

// Test empty directory with no config/state creates a local state.
func TestMetaBackend_emptyDir(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	os.MkdirAll(td, 0755)
	t.Chdir(td)

	// Get the backend
	m := testMetaBackend(t, nil)
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Write some state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	s.WriteState(testState())
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify it exists where we expect it to
	if isEmptyState(DefaultStateFilename) {
		t.Fatalf("no state was written")
	}

	// Verify no backup since it was empty to start
	if !isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup state should be empty")
	}

	// Verify no backend state was made
	if !isEmptyState(filepath.Join(m.DataDir(), DefaultStateFilename)) {
		t.Fatal("backend state should be empty")
	}
}

// check for no state. Either the file doesn't exist, or is empty
func isEmptyState(path string) bool {
	fi, err := os.Stat(path)
	if os.IsNotExist(err) {
		return true
	}

	if fi.Size() == 0 {
		return true
	}

	return false
}

// Test a directory with a legacy state and no config continues to
// use the legacy state.
func TestMetaBackend_emptyWithDefaultState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	os.MkdirAll(td, 0755)
	t.Chdir(td)

	// Write the legacy state
	statePath := DefaultStateFilename
	{
		f, err := os.Create(statePath)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		err = writeStateForTesting(testState(), f)
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	// Get the backend
	m := testMetaBackend(t, nil)
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("err: %s", err)
	}
	if actual := s.State().String(); actual != testState().String() {
		t.Fatalf("bad: %s", actual)
	}

	// Verify it exists where we expect it to
	if _, err := os.Stat(DefaultStateFilename); err != nil {
		t.Fatalf("err: %s", err)
	}

	stateName := filepath.Join(m.DataDir(), DefaultStateFilename)
	if !isEmptyState(stateName) {
		t.Fatal("expected no state at", stateName)
	}

	// Write some state
	next := testState()
	next.RootModule().SetOutputValue("foo", cty.StringVal("bar"), false, "")
	s.WriteState(next)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify a backup was made since we're modifying a pre-existing state
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup state should not be empty")
	}
}

// Test an empty directory with an explicit state path (outside the dir)
func TestMetaBackend_emptyWithExplicitState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	os.MkdirAll(td, 0755)
	t.Chdir(td)

	// Create another directory to store our state
	stateDir := t.TempDir()
	os.MkdirAll(stateDir, 0755)

	// Write the legacy state
	statePath := filepath.Join(stateDir, "foo")
	{
		f, err := os.Create(statePath)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		err = writeStateForTesting(testState(), f)
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	// Setup the meta
	m := testMetaBackend(t, nil)
	m.statePath = statePath

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("err: %s", err)
	}
	if actual := s.State().String(); actual != testState().String() {
		t.Fatalf("bad: %s", actual)
	}

	// Verify neither defaults exist
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	stateName := filepath.Join(m.DataDir(), DefaultStateFilename)
	if !isEmptyState(stateName) {
		t.Fatal("expected no state at", stateName)
	}

	// Write some state
	next := testState()
	markStateForMatching(next, "bar") // just any change so it shows as different than before
	s.WriteState(next)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify a backup was made since we're modifying a pre-existing state
	if isEmptyState(statePath + DefaultBackupExtension) {
		t.Fatal("backup state should not be empty")
	}
}

// Verify that interpolations are allowed
func TestMetaBackend_configureInterpolation(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-new-interp"), td)
	t.Chdir(td)

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	_, err := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if err != nil {
		t.Fatal("should not error")
	}
}

// Newly configured backend
func TestMetaBackend_configureNew(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-new"), td)
	t.Chdir(td)

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state != nil {
		t.Fatal("state should be nil")
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")

	s.WriteState(state)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open("local-state.tfstate")
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify the default paths don't exist
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify a backup doesn't exist
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}
}

// Newly configured backend with prior local state and no remote state
func TestMetaBackend_configureNewWithState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-new-migrate"), td)
	t.Chdir(td)

	// Ask input
	defer testInteractiveInput(t, []string{"yes"})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// This combination should not require the extra -migrate-state flag, since
	// there is no existing backend config
	m.migrateState = false

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state, err := statemgr.RefreshAndRead(s)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if state == nil {
		t.Fatal("state is nil")
	}

	if got, want := testStateMgrCurrentLineage(s), "backend-new-migrate"; got != want {
		t.Fatalf("lineage changed during migration\nnow: %s\nwas: %s", got, want)
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")

	if err := statemgr.WriteAndPersist(s, state, nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open("local-state.tfstate")
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify the default paths don't exist
	if !isEmptyState(DefaultStateFilename) {
		data, _ := os.ReadFile(DefaultStateFilename)

		t.Fatal("state should not exist, but contains:\n", string(data))
	}

	// Verify a backup does exist
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup state is empty or missing")
	}
}

// Newly configured backend with matching local and remote state doesn't prompt
// for copy.
func TestMetaBackend_configureNewWithoutCopy(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-new-migrate"), td)
	t.Chdir(td)

	if err := copy.CopyFile(DefaultStateFilename, "local-state.tfstate"); err != nil {
		t.Fatal(err)
	}

	// Setup the meta
	m := testMetaBackend(t, nil)
	m.input = false

	// init the backend
	_, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Verify the state is where we expect
	f, err := os.Open("local-state.tfstate")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
	f.Close()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if actual.Lineage != "backend-new-migrate" {
		t.Fatalf("incorrect state lineage: %q", actual.Lineage)
	}

	// Verify the default paths don't exist
	if !isEmptyState(DefaultStateFilename) {
		data, _ := os.ReadFile(DefaultStateFilename)

		t.Fatal("state should not exist, but contains:\n", string(data))
	}

	// Verify a backup does exist
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup state is empty or missing")
	}
}

// Newly configured backend with prior local state and no remote state,
// but opting to not migrate.
func TestMetaBackend_configureNewWithStateNoMigrate(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-new-migrate"), td)
	t.Chdir(td)

	// Ask input
	defer testInteractiveInput(t, []string{"no"})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if state := s.State(); state != nil {
		t.Fatal("state is not nil")
	}

	// Verify the default paths don't exist
	if !isEmptyState(DefaultStateFilename) {
		data, _ := os.ReadFile(DefaultStateFilename)

		t.Fatal("state should not exist, but contains:\n", string(data))
	}

	// Verify a backup does exist
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup state is empty or missing")
	}
}

// Newly configured backend with prior local state and remote state
func TestMetaBackend_configureNewWithStateExisting(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-new-migrate-existing"), td)
	t.Chdir(td)

	// Setup the meta
	m := testMetaBackend(t, nil)
	// suppress input
	m.forceInitCopy = true

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state is nil")
	}
	if got, want := testStateMgrCurrentLineage(s), "local"; got != want {
		t.Fatalf("wrong lineage %q; want %q", got, want)
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")

	s.WriteState(state)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open("local-state.tfstate")
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify the default paths don't exist
	if !isEmptyState(DefaultStateFilename) {
		data, _ := os.ReadFile(DefaultStateFilename)

		t.Fatal("state should not exist, but contains:\n", string(data))
	}

	// Verify a backup does exist
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup state is empty or missing")
	}
}

// Newly configured backend with prior local state and remote state
func TestMetaBackend_configureNewWithStateExistingNoMigrate(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-new-migrate-existing"), td)
	t.Chdir(td)

	// Ask input
	defer testInteractiveInput(t, []string{"no"})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state is nil")
	}
	if testStateMgrCurrentLineage(s) != "remote" {
		t.Fatalf("bad: %#v", state)
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")
	s.WriteState(state)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open("local-state.tfstate")
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify the default paths don't exist
	if !isEmptyState(DefaultStateFilename) {
		data, _ := os.ReadFile(DefaultStateFilename)

		t.Fatal("state should not exist, but contains:\n", string(data))
	}

	// Verify a backup does exist
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup state is empty or missing")
	}
}

// Saved backend state matching config
func TestMetaBackend_configuredUnchanged(t *testing.T) {
	t.Chdir(testFixturePath("backend-unchanged"))

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("nil state")
	}
	if testStateMgrCurrentLineage(s) != "configuredUnchanged" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify the default paths don't exist
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify a backup doesn't exist
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}
}

// Saved backend state matching config when the configuration uses static eval references
// and there's an argument overridden on the commandl ine.
func TestMetaBackend_configuredUnchangedWithStaticEvalVars(t *testing.T) {
	// This test is covering the fix for the following issue:
	// https://github.com/opentofu/opentofu/issues/2024
	//
	// To match that issue's reproduction case the following must both be true:
	// - The configuration written in the fixture's .tf file must include either a
	//   reference to a named value or a function call. Currently we use a reference
	//   to a variable.
	// - There must be at least one -backend-config argument on the command line,
	//   which causes us to go into the trickier comparison codepath that has to
	//   re-evaluate _just_ the configuration to distinguish from the combined
	//   configuration plus command-line overrides. Without this the configuration
	//   doesn't get re-evaluated and so the expressions used to construct it are
	//   irrelevant.
	//
	// Although not strictly required for reproduction at the time of writing this
	// test, the local-state.tfstate file in the fixture also includes an output
	// value to ensure that it can't be classified as an "empty state" and thus
	// have migration skipped, even if the rules for activating that fast path
	// change in future.

	t.Chdir(testFixturePath("backend-unchanged-vars"))

	// We'll use a mock backend here because we need to control the schema to
	// make sure that we always have a required field for the ConfigOverride
	// argument to populate. This is covering the regression caused by the first
	// fix to the original bug, discussed here:
	//    https://github.com/opentofu/opentofu/issues/2118
	t.Cleanup(
		backendInit.RegisterTemp("_test_local", func(enc encryption.StateEncryption) backend.Backend {
			return &backendInit.MockBackend{
				ConfigSchemaFn: func() *configschema.Block {
					// The following is based on a subset of the normal "local"
					// backend at the time of writing this test, but subsetted
					// to only what we need and with all of the arguments
					// marked as required (even though the real backend doesn't)
					// so we can make sure that we handle required arguments
					// properly.
					return &configschema.Block{
						Attributes: map[string]*configschema.Attribute{
							"path": {
								Type:     cty.String,
								Required: true,
								// We'll set this one in the root module, using early eval.
							},
							"workspace_dir": {
								Type:     cty.String,
								Required: true,
								// We'll treat this one as if it were set with the -backend-config option to "tofu init"
							},
						},
					}
				},
				WorkspacesFn: func() ([]string, error) {
					return []string{"default"}, nil
				},
				StateMgrFn: func(workspace string) (statemgr.Full, error) {
					// The migration-detection code actually fetches the state to
					// decide if it's "empty" so it can avoid proposing to migrate
					// an empty state, and so unfortunately we do need to have
					// a relatively-realistic implementation of this. We'll
					// just use the same filesystem-based implementation that
					// the real local backend would use, but fixed to use our
					// local-state.tfstate file from the test fixture.
					return statemgr.NewFilesystem("local-state.tfstate", enc), nil
				},
			}
		}),
	)

	// Setup the meta
	m := testMetaBackend(t, nil)
	// testMetaBackend normally sets migrateState on, because most of the tests
	// _want_ to perform migration, but for this one we're behaving as if the
	// user hasn't set the -migrate-state option and thus it should be an error
	// if state migration is required.
	m.migrateState = false

	// Get the backend
	b, diags := m.Backend(
		t.Context(),
		&BackendOpts{
			Init: true,

			// ConfigOverride is the internal representation of the -backend-config
			// command line options. In the normal codepath this gets built into
			// a synthetic hcl.Body so it can be merged with the real hcl.Body
			// for evaluation. For testing purposes here we're constructing the
			// synthetic body using the hcltest package instead, but the effect
			// is the same.
			ConfigOverride: hcltest.MockBody(&hcl.BodyContent{
				Attributes: hcl.Attributes{
					"workspace_dir": {
						Name: "workspace_dir",
						// We're using the "default" workspace in this test and so the workspace_dir
						// isn't actually significant -- we're setting it only to enter the full-evaluation
						// codepath. The only thing that matters is that the value here matches the
						// argument value stored in the .terraform/terraform.tfstate file in the
						// test fixture, meaning that state migration is not required because the
						// configuration is unchanged.
						Expr: hcltest.MockExprLiteral(cty.StringVal("doesnt-actually-matter-what-this-is")),
					},
				},
			}),
		},
		encryption.StateEncryptionDisabled(),
	)
	if diags.HasErrors() {
		// The original problem reported in https://github.com/opentofu/opentofu/issues/2024
		// would return an error here: "Backend configuration has changed".
		t.Fatal(diags.Err())
	}

	// The remaining checks are not directly related to the issue that this test
	// is covering, but are included for completeness to check that this situation
	// also follows the usual invariants for a failed backend init.

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("nil state")
	}
	if testStateMgrCurrentLineage(s) != "configuredUnchanged" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify the default paths don't exist
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify a backup doesn't exist
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}
}

// Changing a configured backend
func TestMetaBackend_configuredChange(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change"), td)
	t.Chdir(td)

	// Ask input
	defer testInteractiveInput(t, []string{"no"})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state != nil {
		t.Fatal("state should be nil")
	}

	// Verify the default paths don't exist
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify a backup doesn't exist
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")

	s.WriteState(state)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open("local-state-2.tfstate")
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify no local state
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify no local backup
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}
}

// Reconfiguring with an already configured backend.
// This should ignore the existing backend config, and configure the new
// backend is if this is the first time.
func TestMetaBackend_reconfigureChange(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-single-to-single"), td)
	t.Chdir(td)

	// Register the single-state backend
	backendInit.Set("local-single", backendLocal.TestNewLocalSingle)
	defer backendInit.Set("local-single", nil)

	// Setup the meta
	m := testMetaBackend(t, nil)

	// this should not ask for input
	m.input = false

	// cli flag -reconfigure
	m.reconfigure = true

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	newState := s.State()
	if newState != nil || !newState.Empty() {
		t.Fatal("state should be nil/empty after forced reconfiguration")
	}

	// verify that the old state is still there
	s = statemgr.NewFilesystem("local-state.tfstate", encryption.StateEncryptionDisabled())
	if err := s.RefreshState(); err != nil {
		t.Fatal(err)
	}
	oldState := s.State()
	if oldState == nil || oldState.Empty() {
		t.Fatal("original state should be untouched")
	}
}

// Initializing a backend which supports workspaces and does *not* have
// the currently selected workspace should prompt the user with a list of
// workspaces to choose from to select a valid one, if more than one workspace
// is available.
func TestMetaBackend_initSelectedWorkspaceDoesNotExist(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-backend-selected-workspace-doesnt-exist-multi"), td)
	t.Chdir(td)

	// Setup the meta
	m := testMetaBackend(t, nil)

	defer testInputMap(t, map[string]string{
		"select-workspace": "2",
	})()

	// Get the backend
	_, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	expected := "foo"
	actual, err := m.Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if actual != expected {
		t.Fatalf("expected selected workspace to be %q, but was %q", expected, actual)
	}
}

// Initializing a backend which supports workspaces and does *not* have the
// currently selected workspace - and which only has a single workspace - should
// automatically select that single workspace.
func TestMetaBackend_initSelectedWorkspaceDoesNotExistAutoSelect(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-backend-selected-workspace-doesnt-exist-single"), td)
	t.Chdir(td)

	// Setup the meta
	m := testMetaBackend(t, nil)

	// this should not ask for input
	m.input = false

	// Assert test precondition: The current selected workspace is "bar"
	previousName, err := m.Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if previousName != "bar" {
		t.Fatalf("expected test fixture to start with 'bar' as the current selected workspace")
	}

	// Get the backend
	_, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	expected := "default"
	actual, err := m.Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if actual != expected {
		t.Fatalf("expected selected workspace to be %q, but was %q", expected, actual)
	}
}

// Initializing a backend which supports workspaces and does *not* have
// the currently selected workspace with input=false should fail.
func TestMetaBackend_initSelectedWorkspaceDoesNotExistInputFalse(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-backend-selected-workspace-doesnt-exist-multi"), td)
	t.Chdir(td)

	// Setup the meta
	m := testMetaBackend(t, nil)
	m.input = false

	// Get the backend
	_, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())

	// Should fail immediately
	if got, want := diags.ErrWithWarnings().Error(), `Currently selected workspace "bar" does not exist`; !strings.Contains(got, want) {
		t.Fatalf("wrong error\ngot:  %s\nwant: %s", got, want)
	}
}

// Changing a configured backend, copying state
func TestMetaBackend_configuredChangeCopy(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change"), td)
	t.Chdir(td)

	// Ask input
	defer testInteractiveInput(t, []string{"yes", "yes"})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if testStateMgrCurrentLineage(s) != "backend-change" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify no local state
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify no local backup
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}
}

// Changing a configured backend that supports only single states to another
// backend that only supports single states.
func TestMetaBackend_configuredChangeCopy_singleState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-single-to-single"), td)
	t.Chdir(td)

	// Register the single-state backend
	backendInit.Set("local-single", backendLocal.TestNewLocalSingle)
	defer backendInit.Set("local-single", nil)

	// Ask input
	defer testInputMap(t, map[string]string{
		"backend-migrate-copy-to-empty": "yes",
	})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if testStateMgrCurrentLineage(s) != "backend-change" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify no local state
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify no local backup
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}
}

// Changing a configured backend that supports multi-state to a
// backend that only supports single states. The multi-state only has
// a default state.
func TestMetaBackend_configuredChangeCopy_multiToSingleDefault(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-multi-default-to-single"), td)
	t.Chdir(td)

	// Register the single-state backend
	backendInit.Set("local-single", backendLocal.TestNewLocalSingle)
	defer backendInit.Set("local-single", nil)

	// Ask input
	defer testInputMap(t, map[string]string{
		"backend-migrate-copy-to-empty": "yes",
	})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if testStateMgrCurrentLineage(s) != "backend-change" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify no local state
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify no local backup
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}
}

// Changing a configured backend that supports multi-state to a
// backend that only supports single states.
func TestMetaBackend_configuredChangeCopy_multiToSingle(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-multi-to-single"), td)
	t.Chdir(td)

	// Register the single-state backend
	backendInit.Set("local-single", backendLocal.TestNewLocalSingle)
	defer backendInit.Set("local-single", nil)

	// Ask input
	defer testInputMap(t, map[string]string{
		"backend-migrate-multistate-to-single": "yes",
		"backend-migrate-copy-to-empty":        "yes",
	})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if testStateMgrCurrentLineage(s) != "backend-change" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify no local state
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify no local backup
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify existing workspaces exist
	envPath := filepath.Join(backendLocal.DefaultWorkspaceDir, "env2", backendLocal.DefaultStateFilename)
	if _, err := os.Stat(envPath); err != nil {
		t.Fatal("env should exist")
	}

	// Verify we are now in the default env, or we may not be able to access the new backend
	env, err := m.Workspace(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if env != backend.DefaultStateName {
		t.Fatal("using non-default env with single-env backend")
	}
}

// Changing a configured backend that supports multi-state to a
// backend that only supports single states.
func TestMetaBackend_configuredChangeCopy_multiToSingleCurrentEnv(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-multi-to-single"), td)
	t.Chdir(td)

	// Register the single-state backend
	backendInit.Set("local-single", backendLocal.TestNewLocalSingle)
	defer backendInit.Set("local-single", nil)

	// Ask input
	defer testInputMap(t, map[string]string{
		"backend-migrate-multistate-to-single": "yes",
		"backend-migrate-copy-to-empty":        "yes",
	})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Change env
	if err := m.SetWorkspace("env2"); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state should not be nil")
	}
	if testStateMgrCurrentLineage(s) != "backend-change-env2" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify no local state
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify no local backup
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify existing workspaces exist
	envPath := filepath.Join(backendLocal.DefaultWorkspaceDir, "env2", backendLocal.DefaultStateFilename)
	if _, err := os.Stat(envPath); err != nil {
		t.Fatal("env should exist")
	}
}

// Changing a configured backend that supports multi-state to a
// backend that also supports multi-state.
func TestMetaBackend_configuredChangeCopy_multiToMulti(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-multi-to-multi"), td)
	t.Chdir(td)

	// Ask input
	defer testInputMap(t, map[string]string{
		"backend-migrate-multistate-to-multistate": "yes",
	})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check resulting states
	workspaces, err := b.Workspaces(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	sort.Strings(workspaces)
	expected := []string{"default", "env2"}
	if !reflect.DeepEqual(workspaces, expected) {
		t.Fatalf("bad: %#v", workspaces)
	}

	{
		// Check the default state
		s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := s.RefreshState(); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		state := s.State()
		if state == nil {
			t.Fatal("state should not be nil")
		}
		if testStateMgrCurrentLineage(s) != "backend-change" {
			t.Fatalf("bad: %#v", state)
		}
	}

	{
		// Check the other state
		s, err := b.StateMgr(t.Context(), "env2")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := s.RefreshState(); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		state := s.State()
		if state == nil {
			t.Fatal("state should not be nil")
		}
		if testStateMgrCurrentLineage(s) != "backend-change-env2" {
			t.Fatalf("bad: %#v", state)
		}
	}

	// Verify no local backup
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}

	{
		// Verify existing workspaces exist
		envPath := filepath.Join(backendLocal.DefaultWorkspaceDir, "env2", backendLocal.DefaultStateFilename)
		if _, err := os.Stat(envPath); err != nil {
			t.Fatalf("%s should exist, but does not", envPath)
		}
	}

	{
		// Verify new workspaces exist
		envPath := filepath.Join("envdir-new", "env2", backendLocal.DefaultStateFilename)
		if _, err := os.Stat(envPath); err != nil {
			t.Fatalf("%s should exist, but does not", envPath)
		}
	}
}

// Changing a configured backend that supports multi-state to a
// backend that also supports multi-state, but doesn't allow a
// default state while the default state is non-empty.
func TestMetaBackend_configuredChangeCopy_multiToNoDefaultWithDefault(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-multi-to-no-default-with-default"), td)
	t.Chdir(td)

	// Register the single-state backend
	backendInit.Set("local-no-default", backendLocal.TestNewLocalNoDefault)
	defer backendInit.Set("local-no-default", nil)

	// Ask input
	defer testInputMap(t, map[string]string{
		"backend-migrate-multistate-to-multistate": "yes",
		"new-state-name": "env1",
	})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check resulting states
	workspaces, err := b.Workspaces(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	sort.Strings(workspaces)
	expected := []string{"env1", "env2"}
	if !reflect.DeepEqual(workspaces, expected) {
		t.Fatalf("bad: %#v", workspaces)
	}

	{
		// Check the renamed default state
		s, err := b.StateMgr(t.Context(), "env1")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := s.RefreshState(); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		state := s.State()
		if state == nil {
			t.Fatal("state should not be nil")
		}
		if testStateMgrCurrentLineage(s) != "backend-change-env1" {
			t.Fatalf("bad: %#v", state)
		}
	}

	{
		// Verify existing workspaces exist
		envPath := filepath.Join(backendLocal.DefaultWorkspaceDir, "env2", backendLocal.DefaultStateFilename)
		if _, err := os.Stat(envPath); err != nil {
			t.Fatal("env should exist")
		}
	}

	{
		// Verify new workspaces exist
		envPath := filepath.Join("envdir-new", "env2", backendLocal.DefaultStateFilename)
		if _, err := os.Stat(envPath); err != nil {
			t.Fatal("env should exist")
		}
	}
}

// Changing a configured backend that supports multi-state to a
// backend that also supports multi-state, but doesn't allow a
// default state while the default state is empty.
func TestMetaBackend_configuredChangeCopy_multiToNoDefaultWithoutDefault(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-change-multi-to-no-default-without-default"), td)
	t.Chdir(td)

	// Register the single-state backend
	backendInit.Set("local-no-default", backendLocal.TestNewLocalNoDefault)
	defer backendInit.Set("local-no-default", nil)

	// Ask input
	defer testInputMap(t, map[string]string{
		"backend-migrate-multistate-to-multistate": "yes",
	})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check resulting states
	workspaces, err := b.Workspaces(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	sort.Strings(workspaces)
	expected := []string{"env2"} // default is skipped because it is absent in the source backend
	if !reflect.DeepEqual(workspaces, expected) {
		t.Fatalf("wrong workspaces\ngot:  %#v\nwant: %#v", workspaces, expected)
	}

	{
		// Check the named state
		s, err := b.StateMgr(t.Context(), "env2")
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if err := s.RefreshState(); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		state := s.State()
		if state == nil {
			t.Fatal("state should not be nil")
		}
		if testStateMgrCurrentLineage(s) != "backend-change-env2" {
			t.Fatalf("bad: %#v", state)
		}
	}

	{
		// Verify existing workspaces exist
		envPath := filepath.Join(backendLocal.DefaultWorkspaceDir, "env2", backendLocal.DefaultStateFilename)
		if _, err := os.Stat(envPath); err != nil {
			t.Fatalf("%s should exist, but does not", envPath)
		}
	}

	{
		// Verify new workspaces exist
		envPath := filepath.Join("envdir-new", "env2", backendLocal.DefaultStateFilename)
		if _, err := os.Stat(envPath); err != nil {
			t.Fatalf("%s should exist, but does not", envPath)
		}
	}
}

// Unsetting a saved backend
func TestMetaBackend_configuredUnset(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-unset"), td)
	t.Chdir(td)

	// Ask input
	defer testInteractiveInput(t, []string{"no"})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state != nil {
		t.Fatal("state should be nil")
	}

	// Verify the default paths don't exist
	if !isEmptyState(DefaultStateFilename) {
		data, _ := os.ReadFile(DefaultStateFilename)
		t.Fatal("state should not exist, but contains:\n", string(data))
	}

	// Verify a backup doesn't exist
	if !isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		data, _ := os.ReadFile(DefaultStateFilename + DefaultBackupExtension)
		t.Fatal("backup should not exist, but contains:\n", string(data))
	}

	// Write some state
	s.WriteState(testState())
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify it exists where we expect it to
	if isEmptyState(DefaultStateFilename) {
		t.Fatal(DefaultStateFilename, "is empty")
	}

	// Verify no backup since it was empty to start
	if !isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		data, _ := os.ReadFile(DefaultStateFilename + DefaultBackupExtension)
		t.Fatal("backup state should be empty, but contains:\n", string(data))
	}
}

// Unsetting a saved backend and copying the remote state
func TestMetaBackend_configuredUnsetCopy(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-unset"), td)
	t.Chdir(td)

	// Ask input
	defer testInteractiveInput(t, []string{"yes", "yes"})()

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("state is nil")
	}
	if got, want := testStateMgrCurrentLineage(s), "configuredUnset"; got != want {
		t.Fatalf("wrong state lineage %q; want %q", got, want)
	}

	// Verify a backup doesn't exist
	if !isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatalf("backup state should be empty")
	}

	// Write some state
	s.WriteState(testState())
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify it exists where we expect it to
	if _, err := os.Stat(DefaultStateFilename); err != nil {
		t.Fatalf("err: %s", err)
	}

	// Verify a backup since it wasn't empty to start
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup is empty")
	}
}

// A plan that has uses the local backend
func TestMetaBackend_planLocal(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-plan-local"), td)
	t.Chdir(td)

	backendConfigBlock := cty.ObjectVal(map[string]cty.Value{
		"path":          cty.NullVal(cty.String),
		"workspace_dir": cty.NullVal(cty.String),
	})
	backendConfigRaw, err := plans.NewDynamicValue(backendConfigBlock, backendConfigBlock.Type())
	if err != nil {
		t.Fatal(err)
	}
	backendConfig := plans.Backend{
		Type:      "local",
		Config:    backendConfigRaw,
		Workspace: "default",
	}

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.BackendForLocalPlan(t.Context(), backendConfig, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state != nil {
		t.Fatalf("state should be nil: %#v", state)
	}

	// The default state file should not exist yet
	if !isEmptyState(DefaultStateFilename) {
		t.Fatal("expected empty state")
	}

	// A backup file shouldn't exist yet either.
	if !isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("expected empty backup")
	}

	// Verify we have no configured backend
	path := filepath.Join(m.DataDir(), DefaultStateFilename)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("should not have backend configured")
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")

	s.WriteState(state)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open(DefaultStateFilename)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify no local backup
	if !isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatalf("backup state should be empty")
	}
}

// A plan with a custom state save path
func TestMetaBackend_planLocalStatePath(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-plan-local"), td)
	t.Chdir(td)

	original := testState()
	markStateForMatching(original, "hello")

	backendConfigBlock := cty.ObjectVal(map[string]cty.Value{
		"path":          cty.NullVal(cty.String),
		"workspace_dir": cty.NullVal(cty.String),
	})
	backendConfigRaw, err := plans.NewDynamicValue(backendConfigBlock, backendConfigBlock.Type())
	if err != nil {
		t.Fatal(err)
	}
	plannedBackend := plans.Backend{
		Type:      "local",
		Config:    backendConfigRaw,
		Workspace: "default",
	}

	// Create an alternate output path
	statePath := "foo.tfstate"

	// put an initial state there that needs to be backed up
	err = statemgr.WriteAndPersist(statemgr.NewFilesystem(statePath, encryption.StateEncryptionDisabled()), original, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Setup the meta
	m := testMetaBackend(t, nil)
	m.stateOutPath = statePath

	// Get the backend
	b, diags := m.BackendForLocalPlan(t.Context(), plannedBackend, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state != nil {
		t.Fatal("default workspace state is not nil, but should be because we've not put anything there")
	}

	// Verify the default path doesn't exist
	if _, err := os.Stat(DefaultStateFilename); err == nil {
		t.Fatalf("err: %s", err)
	}

	// Verify a backup doesn't exists
	if _, err := os.Stat(DefaultStateFilename + DefaultBackupExtension); err == nil {
		t.Fatal("file should not exist")
	}

	// Verify we have no configured backend/legacy
	path := filepath.Join(m.DataDir(), DefaultStateFilename)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("should not have backend configured")
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")

	s.WriteState(state)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open(statePath)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify we have a backup
	if isEmptyState(statePath + DefaultBackupExtension) {
		t.Fatal("backup is empty")
	}
}

// A plan that has no backend config, matching local state
func TestMetaBackend_planLocalMatch(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-plan-local-match"), td)
	t.Chdir(td)

	backendConfigBlock := cty.ObjectVal(map[string]cty.Value{
		"path":          cty.NullVal(cty.String),
		"workspace_dir": cty.NullVal(cty.String),
	})
	backendConfigRaw, err := plans.NewDynamicValue(backendConfigBlock, backendConfigBlock.Type())
	if err != nil {
		t.Fatal(err)
	}
	backendConfig := plans.Backend{
		Type:      "local",
		Config:    backendConfigRaw,
		Workspace: "default",
	}

	// Setup the meta
	m := testMetaBackend(t, nil)

	// Get the backend
	b, diags := m.BackendForLocalPlan(t.Context(), backendConfig, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s, err := b.StateMgr(t.Context(), backend.DefaultStateName)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if err := s.RefreshState(); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	state := s.State()
	if state == nil {
		t.Fatal("should is nil")
	}
	if testStateMgrCurrentLineage(s) != "hello" {
		t.Fatalf("bad: %#v", state)
	}

	// Verify the default path
	if isEmptyState(DefaultStateFilename) {
		t.Fatal("state is empty")
	}

	// Verify we have no configured backend/legacy
	path := filepath.Join(m.DataDir(), DefaultStateFilename)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("should not have backend configured")
	}

	// Write some state
	state = states.NewState()
	mark := markStateForMatching(state, "changing")

	s.WriteState(state)
	if err := s.PersistState(nil); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Verify the state is where we expect
	{
		f, err := os.Open(DefaultStateFilename)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		actual, err := statefile.Read(f, encryption.StateEncryptionDisabled())
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}

		assertStateHasMarker(t, actual.State, mark)
	}

	// Verify local backup
	if isEmptyState(DefaultStateFilename + DefaultBackupExtension) {
		t.Fatal("backup is empty")
	}
}

// init a backend using -backend-config options multiple times
func TestMetaBackend_configureWithExtra(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-backend-empty"), td)
	t.Chdir(td)

	extras := map[string]cty.Value{"path": cty.StringVal("hello")}
	m := testMetaBackend(t, nil)
	opts := &BackendOpts{
		ConfigOverride: configs.SynthBody("synth", extras),
		Init:           true,
	}

	_, cHash, err := m.backendConfig(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}

	// init the backend
	_, diags := m.Backend(t.Context(), &BackendOpts{
		ConfigOverride: configs.SynthBody("synth", extras),
		Init:           true,
	}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// Check the state
	s := testDataStateRead(t, filepath.Join(DefaultDataDir, backendLocal.DefaultStateFilename))
	if s.Backend.Hash != uint64(cHash) {
		t.Fatal("mismatched state and config backend hashes")
	}

	// init the backend again with the same options
	m = testMetaBackend(t, nil)
	_, err = m.Backend(t.Context(), &BackendOpts{
		ConfigOverride: configs.SynthBody("synth", extras),
		Init:           true,
	}, encryption.StateEncryptionDisabled())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Check the state
	s = testDataStateRead(t, filepath.Join(DefaultDataDir, backendLocal.DefaultStateFilename))
	if s.Backend.Hash != uint64(cHash) {
		t.Fatal("mismatched state and config backend hashes")
	}
}

// when configuring a default local state, don't delete local state
func TestMetaBackend_localDoesNotDeleteLocal(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-backend-empty"), td)
	t.Chdir(td)

	//// create our local state
	orig := states.NewState()
	orig.Module(addrs.RootModuleInstance).SetOutputValue("foo", cty.StringVal("bar"), false, "")
	testStateFileDefault(t, orig)

	m := testMetaBackend(t, nil)
	m.forceInitCopy = true
	// init the backend
	_, diags := m.Backend(t.Context(), &BackendOpts{Init: true}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	// check that we can read the state
	s := testStateRead(t, DefaultStateFilename)
	if s.Empty() {
		t.Fatal("our state was deleted")
	}
}

// move options from config to -backend-config
func TestMetaBackend_configToExtra(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("init-backend"), td)
	t.Chdir(td)

	// init the backend
	m := testMetaBackend(t, nil)
	_, err := m.Backend(t.Context(), &BackendOpts{
		Init: true,
	}, encryption.StateEncryptionDisabled())
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// Check the state
	s := testDataStateRead(t, filepath.Join(DefaultDataDir, backendLocal.DefaultStateFilename))
	backendHash := s.Backend.Hash

	// init again but remove the path option from the config
	cfg := "terraform {\n  backend \"local\" {}\n}\n"
	if err := os.WriteFile("main.tf", []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	// init the backend again with the  options
	extras := map[string]cty.Value{"path": cty.StringVal("hello")}
	m = testMetaBackend(t, nil)
	m.forceInitCopy = true
	_, diags := m.Backend(t.Context(), &BackendOpts{
		ConfigOverride: configs.SynthBody("synth", extras),
		Init:           true,
	}, encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	s = testDataStateRead(t, filepath.Join(DefaultDataDir, backendLocal.DefaultStateFilename))

	if s.Backend.Hash == backendHash {
		t.Fatal("state.Backend.Hash was not updated")
	}
}

// no config; return inmem backend stored in state
func TestBackendFromState(t *testing.T) {
	wd := tempWorkingDirFixture(t, "backend-from-state")
	t.Chdir(wd.RootModuleDir())

	// Setup the meta
	m := testMetaBackend(t, nil)
	m.WorkingDir = wd
	// tofu caches a small "state" file that stores the backend config.
	// This test must override m.dataDir so it loads the "terraform.tfstate" file in the
	// test directory as the backend config cache. This fixture is really a
	// fixture for the data dir rather than the module dir, so we'll override
	// them to match just for this test.
	wd.OverrideDataDir(".")

	stateBackend, diags := m.backendFromState(context.Background(), encryption.StateEncryptionDisabled())
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	if _, ok := stateBackend.(*backendInmem.Backend); !ok {
		t.Fatal("did not get expected inmem backend")
	}
}

func testMetaBackend(t *testing.T, args []string) *Meta {
	var m Meta
	m.Ui = new(cli.MockUi)
	view, _ := testView(t)
	m.View = view
	m.process(args)
	f := m.extendedFlagSet("test")
	if err := f.Parse(args); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	// metaBackend tests are verifying migrate actions
	m.migrateState = true

	return &m
}
