// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func TestLocal_impl(t *testing.T) {
	var _ backend.Enhanced = New(encryption.StateEncryptionDisabled())
	var _ backend.Local = New(encryption.StateEncryptionDisabled())
	var _ backend.CLI = New(encryption.StateEncryptionDisabled())
}

func TestLocal_backend(t *testing.T) {
	testTmpDir(t)
	b := New(encryption.StateEncryptionDisabled())
	backend.TestBackendStates(t, b)
	backend.TestBackendStateLocks(t, b, b)

	// On Windows, the file open handles is not closed due to multiple
	// rounds of locking and reading the state.
	if runtime.GOOS == "windows" {
		runtime.GC()
	}
}

func checkState(t *testing.T, path, expected string) {
	t.Helper()
	// Read the state
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	state, err := statefile.Read(f, encryption.StateEncryptionDisabled())
	f.Close()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	actual := state.State.String()
	expected = strings.TrimSpace(expected)
	if actual != expected {
		t.Fatalf("state does not match! actual:\n%s\n\nexpected:\n%s", actual, expected)
	}
}

func TestLocal_StatePaths(t *testing.T) {
	b := New(encryption.StateEncryptionDisabled())

	// Test the defaults
	path, out, back := b.StatePaths("")

	if path != DefaultStateFilename {
		t.Fatalf("expected %q, got %q", DefaultStateFilename, path)
	}

	if out != DefaultStateFilename {
		t.Fatalf("expected %q, got %q", DefaultStateFilename, out)
	}

	dfltBackup := DefaultStateFilename + DefaultBackupExtension
	if back != dfltBackup {
		t.Fatalf("expected %q, got %q", dfltBackup, back)
	}

	// check with env
	testEnv := "test_env"
	path, out, back = b.StatePaths(testEnv)

	expectedPath := filepath.Join(DefaultWorkspaceDir, testEnv, DefaultStateFilename)
	expectedOut := expectedPath
	expectedBackup := expectedPath + DefaultBackupExtension

	if path != expectedPath {
		t.Fatalf("expected %q, got %q", expectedPath, path)
	}

	if out != expectedOut {
		t.Fatalf("expected %q, got %q", expectedOut, out)
	}

	if back != expectedBackup {
		t.Fatalf("expected %q, got %q", expectedBackup, back)
	}

}

func TestLocal_addAndRemoveStates(t *testing.T) {
	testTmpDir(t)
	dflt := backend.DefaultStateName
	expectedStates := []string{dflt}

	b := New(encryption.StateEncryptionDisabled())
	states, err := b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(states, expectedStates) {
		t.Fatalf("expected []string{%q}, got %q", dflt, states)
	}

	expectedA := "test_A"
	if _, err := b.StateMgr(t.Context(), expectedA); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedStates = append(expectedStates, expectedA)
	if !reflect.DeepEqual(states, expectedStates) {
		t.Fatalf("expected %q, got %q", expectedStates, states)
	}

	expectedB := "test_B"
	if _, err := b.StateMgr(t.Context(), expectedB); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedStates = append(expectedStates, expectedB)
	if !reflect.DeepEqual(states, expectedStates) {
		t.Fatalf("expected %q, got %q", expectedStates, states)
	}

	if err := b.DeleteWorkspace(t.Context(), expectedA, true); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedStates = []string{dflt, expectedB}
	if !reflect.DeepEqual(states, expectedStates) {
		t.Fatalf("expected %q, got %q", expectedStates, states)
	}

	if err := b.DeleteWorkspace(t.Context(), expectedB, true); err != nil {
		t.Fatal(err)
	}

	states, err = b.Workspaces(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	expectedStates = []string{dflt}
	if !reflect.DeepEqual(states, expectedStates) {
		t.Fatalf("expected %q, got %q", expectedStates, states)
	}

	if err := b.DeleteWorkspace(t.Context(), dflt, true); err == nil {
		t.Fatal("expected error deleting default state")
	}
}

// a local backend which returns sentinel errors for NamedState methods to
// verify it's being called.
type testDelegateBackend struct {
	*Local

	// return a sentinel error on these calls
	stateErr  bool
	statesErr bool
	deleteErr bool
}

var errTestDelegateState = errors.New("state called")
var errTestDelegateStates = errors.New("states called")
var errTestDelegateDeleteState = errors.New("delete called")

func (b *testDelegateBackend) StateMgr(_ context.Context, name string) (statemgr.Full, error) {
	if b.stateErr {
		return nil, errTestDelegateState
	}
	s := statemgr.NewFilesystem("terraform.tfstate", encryption.StateEncryptionDisabled())
	return s, nil
}

func (b *testDelegateBackend) Workspaces(context.Context) ([]string, error) {
	if b.statesErr {
		return nil, errTestDelegateStates
	}
	return []string{"default"}, nil
}

func (b *testDelegateBackend) DeleteWorkspace(_ context.Context, name string, force bool) error {
	if b.deleteErr {
		return errTestDelegateDeleteState
	}
	return nil
}

// verify that the MultiState methods are dispatched to the correct Backend.
func TestLocal_multiStateBackend(t *testing.T) {
	// assign a separate backend where we can read the state
	b := NewWithBackend(&testDelegateBackend{
		stateErr:  true,
		statesErr: true,
		deleteErr: true,
	}, nil)

	if _, err := b.StateMgr(t.Context(), "test"); err != errTestDelegateState {
		t.Fatal("expected errTestDelegateState, got:", err)
	}

	if _, err := b.Workspaces(t.Context()); err != errTestDelegateStates {
		t.Fatal("expected errTestDelegateStates, got:", err)
	}

	if err := b.DeleteWorkspace(t.Context(), "test", true); err != errTestDelegateDeleteState {
		t.Fatal("expected errTestDelegateDeleteState, got:", err)
	}
}

// testTmpDir changes into a tmp dir and change back automatically when the test
// and all its subtests complete.
func testTmpDir(t testing.TB) {
	t.Helper()
	tmp := t.TempDir()
	t.Chdir(tmp)
}
