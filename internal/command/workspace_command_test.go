// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/workdir"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/local"
	"github.com/opentofu/opentofu/internal/backend/remote-state/inmem"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states"
	"github.com/opentofu/opentofu/internal/states/statefile"
	"github.com/opentofu/opentofu/internal/states/statemgr"
)

func TestWorkspace_createAndChange(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	t.Chdir(td)

	showCmdView, showCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}
	code := RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput := showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}
	if strings.TrimSpace(showCmdOutput.Stdout()) != backend.DefaultStateName {
		t.Fatal("current workspace should be 'default'")
	}

	args := []string{"test"}
	newCmdView, newCmdDone := testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       newCmdView,
	}
	code = RunCommander(t, WorkspaceNewCommander(false), meta, args)
	newCmdOutput := newCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, newCmdOutput.Stderr())
	}

	showCmdView, showCmdDone = testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}
	code = RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput = showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}
	if strings.TrimSpace(showCmdOutput.Stdout()) != "test" {
		t.Fatal("current workspace should be 'test'")
	}

	args = []string{backend.DefaultStateName}
	selectCmdView, selectCmdDone := testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       selectCmdView,
	}
	code = RunCommander(t, WorkspaceSelectCommander(false), meta, args)
	selectCmdOutput := selectCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, selectCmdOutput.Stderr())
	}

	showCmdView, showCmdDone = testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}
	code = RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput = showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}
	if strings.TrimSpace(showCmdOutput.Stdout()) != backend.DefaultStateName {
		t.Fatal("current workspace should be 'default'")
	}
}

// Create some workspaces and test the list output.
// This also ensures we switch to the correct env after each call
func TestWorkspace_createAndList(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	t.Chdir(td)

	// make sure a vars file doesn't interfere
	err := os.WriteFile(
		DefaultVarsFilename,
		[]byte(`foo = "bar"`),
		0644,
	)
	if err != nil {
		t.Fatal(err)
	}

	envs := []string{"test_a", "test_b", "test_c"}

	// create multiple workspaces
	for _, env := range envs {
		newCmdView, newCmdDone := testView(t)
		meta := Meta{
			WorkingDir: workdir.NewDir("."),
			View:       newCmdView,
		}
		code := RunCommander(t, WorkspaceNewCommander(false), meta, []string{env})
		newCmdOutput := newCmdDone(t)
		if code != 0 {
			t.Fatalf("bad: %d\n\n%s", code, newCmdOutput.Stderr())
		}
	}

	listCmdView, listCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       listCmdView,
	}

	code := RunCommander(t, WorkspaceListCommander(false), meta, nil)
	listCmdOutput := listCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, listCmdOutput.Stderr())
	}

	actual := strings.TrimSpace(listCmdOutput.Stdout())
	expected := "default\n  test_a\n  test_b\n* test_c"

	if actual != expected {
		t.Fatalf("\nexpected: %q\nactual:  %q", expected, actual)
	}
}

// Create some workspaces and test the show output.
func TestWorkspace_createAndShow(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	t.Chdir(td)

	// make sure a vars file doesn't interfere
	err := os.WriteFile(
		DefaultVarsFilename,
		[]byte(`foo = "bar"`),
		0644,
	)
	if err != nil {
		t.Fatal(err)
	}

	// make sure current workspace show outputs "default"
	showCmdView, showCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}

	code := RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput := showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}

	actual := strings.TrimSpace(showCmdOutput.Stdout())
	expected := "default"

	if actual != expected {
		t.Fatalf("\nexpected: %q\nactual:  %q", expected, actual)
	}

	env := []string{"test_a"}

	// create test_a workspace
	newCmdView, newCmdDone := testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       newCmdView,
	}
	code = RunCommander(t, WorkspaceNewCommander(false), meta, env)
	newCmdOutput := newCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, newCmdOutput.Stderr())
	}

	selCmd := &WorkspaceSelectCommand{}
	selCmdView, selCmdDone := testView(t)
	selCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       selCmdView,
	}
	code = RunCommander(t, WorkspaceSelectCommander(false), meta, env)
	selCmdOutput := selCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, selCmdOutput.Stderr())
	}

	showCmdView, showCmdDone = testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}

	code = RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput = showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}

	actual = strings.TrimSpace(showCmdOutput.Stdout())
	expected = "test_a"

	if actual != expected {
		t.Fatalf("\nexpected: %q\nactual:  %q", expected, actual)
	}
}

// Don't allow names that aren't URL safe
func TestWorkspace_createInvalid(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	t.Chdir(td)

	envs := []string{"test_a*", "test_b/foo", "../../../test_c", "好_d"}

	// create multiple workspaces
	for _, env := range envs {
		view, done := testView(t)
		meta := Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		}
		code := RunCommander(t, WorkspaceNewCommander(false), meta, []string{env})
		output := done(t)
		if code == 0 {
			t.Fatalf("expected failure: \n%s", output.All())
		}
	}

	// list workspaces to make sure none were created
	listCmdView, listCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       listCmdView,
	}

	code := RunCommander(t, WorkspaceListCommander(false), meta, nil)
	listCmdOutput := listCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, listCmdOutput.Stderr())
	}

	actual := strings.TrimSpace(listCmdOutput.Stdout())
	expected := "* default"

	if actual != expected {
		t.Fatalf("\nexpected: %q\nactual:  %q", expected, actual)
	}
}

func TestWorkspace_createWithState(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("inmem-backend"), td)
	t.Chdir(td)
	defer inmem.Reset()

	// init the backend
	initCmdView, initCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       initCmdView,
	}
	code := RunCommander(t, InitCommander(), meta, nil)
	initCmdOutput := initCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: \n%s", initCmdOutput.Stderr())
	}

	originalState := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	err := statemgr.WriteAndPersist(t.Context(), statemgr.NewFilesystem("test.tfstate", encryption.StateEncryptionDisabled()), originalState, nil)
	if err != nil {
		t.Fatal(err)
	}

	workspace := "test_workspace"

	args := []string{"-state", "test.tfstate", workspace}
	newCmdView, newCmdDone := testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       newCmdView,
	}
	code = RunCommander(t, WorkspaceNewCommander(false), meta, args)
	newCmdOutput := newCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, newCmdOutput.Stderr())
	}

	newPath := filepath.Join(local.DefaultWorkspaceDir, "test", arguments.DefaultStateFilename)
	envState := statemgr.NewFilesystem(newPath, encryption.StateEncryptionDisabled())
	err = envState.RefreshState(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	b := backend.TestBackendConfig(t, inmem.New(encryption.StateEncryptionDisabled()), nil)
	sMgr, err := b.StateMgr(t.Context(), workspace)
	if err != nil {
		t.Fatal(err)
	}

	newState := sMgr.State()

	if got, want := newState.String(), originalState.String(); got != want {
		t.Fatalf("states not equal\ngot: %s\nwant: %s", got, want)
	}
}

func TestWorkspace_delete(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	// create the workspace directories
	if err := os.MkdirAll(filepath.Join(local.DefaultWorkspaceDir, "test"), 0755); err != nil {
		t.Fatal(err)
	}

	// create the workspace file
	if err := os.MkdirAll(workdir.DefaultDataDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workdir.DefaultDataDir, local.DefaultWorkspaceFile), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	delCmdView, delCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       delCmdView,
	}

	// we can't delete our current workspace
	args := []string{"test"}
	code := RunCommander(t, WorkspaceDeleteCommander(false), meta, args)
	delCmdOutput := delCmdDone(t)
	if code == 0 {
		t.Fatalf("expected error deleting current workspace: %s", delCmdOutput.All())
	}

	selectCmdView, selectCmdDone := testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       selectCmdView,
	}
	code = RunCommander(t, WorkspaceSelectCommander(false), meta, []string{backend.DefaultStateName})
	selectCmdOutput := selectCmdDone(t)
	if code != 0 {
		t.Fatalf("error selecting workspace: %s", selectCmdOutput.All())
	}

	// try the delete again
	delCmdView, delCmdDone = testView(t)
	meta.View = delCmdView
	code = RunCommander(t, WorkspaceDeleteCommander(false), meta, args)
	delCmdOutput = delCmdDone(t)
	if code != 0 {
		t.Fatalf("error deleting workspace: %s", delCmdOutput.Stderr())
	}

	showCmdView, showCmdDone := testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}
	code = RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput := showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}
	if strings.TrimSpace(showCmdOutput.Stdout()) != backend.DefaultStateName {
		t.Fatal("current workspace should be 'default'")
	}
}

func TestWorkspace_deleteInvalid(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	// choose an invalid workspace name
	workspace := "test workspace"
	path := filepath.Join(local.DefaultWorkspaceDir, workspace)

	// create the workspace directories
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}

	delCmdView, delCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       delCmdView,
	}

	// delete the workspace
	code := RunCommander(t, WorkspaceDeleteCommander(false), meta, []string{workspace})
	delCmdOutput := delCmdDone(t)
	if code != 0 {
		t.Fatalf("error deleting workspace: %s", delCmdOutput.Stderr())
	}

	if _, err := os.Stat(path); err == nil {
		t.Fatalf("should have deleted workspace, but %s still exists", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error for workspace path: %s", err)
	}
}

func TestWorkspace_deleteWithState(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	// create the workspace directories
	if err := os.MkdirAll(filepath.Join(local.DefaultWorkspaceDir, "test"), 0755); err != nil {
		t.Fatal(err)
	}

	originalState := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectReady,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})

	f, err := os.Create(filepath.Join(local.DefaultWorkspaceDir, "test", "terraform.tfstate"))
	if err != nil {
		t.Fatal(err)
	}
	err = statefile.Write(&statefile.File{
		Serial:  0,
		Lineage: "test-lineage",
		State:   originalState,
	}, f, encryption.StateEncryptionDisabled())
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	delCmdView, delCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       delCmdView,
	}
	args := []string{"test"}
	code := RunCommander(t, WorkspaceDeleteCommander(false), meta, args)
	delCmdOutput := delCmdDone(t)
	if code == 0 {
		t.Fatalf("expected failure without -force.\noutput: %s", delCmdOutput.All())
	}
	gotStderr := delCmdOutput.Stderr()
	if want, got := `Workspace "test" is currently tracking the following resource instances`, gotStderr; !strings.Contains(got, want) {
		t.Errorf("missing expected error message\nwant substring: %s\ngot:\n%s", want, got)
	}
	if want, got := `- test_instance.foo`, gotStderr; !strings.Contains(got, want) {
		t.Errorf("error message doesn't mention the remaining instance\nwant substring: %s\ngot:\n%s", want, got)
	}

	delCmdView, delCmdDone = testView(t)
	meta.View = delCmdView

	args = []string{"-force", "test"}
	code = RunCommander(t, WorkspaceDeleteCommander(false), meta, args)
	delCmdOutput = delCmdDone(t)
	if code != 0 {
		t.Fatalf("failure: %s", delCmdOutput.Stderr())
	}

	if _, err := os.Stat(filepath.Join(local.DefaultWorkspaceDir, "test")); !os.IsNotExist(err) {
		t.Fatal("env 'test' still exists!")
	}
}

func TestWorkspace_selectWithOrCreate(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	t.Chdir(td)

	showCmdView, showCmdDone := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}
	code := RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput := showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}
	if strings.TrimSpace(showCmdOutput.Stdout()) != backend.DefaultStateName {
		t.Fatal("current workspace should be 'default'")
	}

	args := []string{"-or-create", "test"}
	selectCmdView, selectCmdDone := testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       selectCmdView,
	}
	code = RunCommander(t, WorkspaceSelectCommander(false), meta, args)
	selectCmdOutput := selectCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, selectCmdOutput.Stderr())
	}

	showCmdView, showCmdDone = testView(t)
	meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}
	code = RunCommander(t, WorkspaceShowCommander(), meta, nil)
	showCmdOutput = showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}
	if strings.TrimSpace(showCmdOutput.Stdout()) != "test" {
		t.Fatal("current workspace should be 'test'")
	}
}
