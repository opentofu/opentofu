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
	"github.com/opentofu/opentofu/internal/states/statemgr"

	legacy "github.com/opentofu/opentofu/internal/legacy/tofu"
)

func TestWorkspace_createAndChange(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	t.Chdir(td)

	newCmd := &WorkspaceNewCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
		},
	}

	current, _ := newCmd.Workspace(t.Context())
	if current != backend.DefaultStateName {
		t.Fatal("current workspace should be 'default'")
	}

	args := []string{"test"}
	newCmdView, newCmdDone := testView(t)
	newCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       newCmdView,
	}
	code := newCmd.Run(args)
	newCmdOutput := newCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, newCmdOutput.Stderr())
	}

	current, _ = newCmd.Workspace(t.Context())
	if current != "test" {
		t.Fatalf("current workspace should be 'test', got %q", current)
	}

	selCmd := &WorkspaceSelectCommand{}
	args = []string{backend.DefaultStateName}
	selectCmdView, selectCmdDone := testView(t)
	selCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       selectCmdView,
	}
	code = selCmd.Run(args)
	selectCmdOutput := selectCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, selectCmdOutput.Stderr())
	}

	current, _ = newCmd.Workspace(t.Context())
	if current != backend.DefaultStateName {
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
		newCmd := &WorkspaceNewCommand{
			Meta: Meta{
				WorkingDir: workdir.NewDir("."),
				View:       newCmdView,
			},
		}
		code := newCmd.Run([]string{env})
		newCmdOutput := newCmdDone(t)
		if code != 0 {
			t.Fatalf("bad: %d\n\n%s", code, newCmdOutput.Stderr())
		}
	}

	listCmd := &WorkspaceListCommand{}
	listCmdView, listCmdDone := testView(t)
	listCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       listCmdView,
	}

	code := listCmd.Run(nil)
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
	showCmd := &WorkspaceShowCommand{}
	showCmdView, showCmdDone := testView(t)
	showCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}

	code := showCmd.Run(nil)
	showCmdOutput := showCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, showCmdOutput.Stderr())
	}

	actual := strings.TrimSpace(showCmdOutput.Stdout())
	expected := "default"

	if actual != expected {
		t.Fatalf("\nexpected: %q\nactual:  %q", expected, actual)
	}

	newCmd := &WorkspaceNewCommand{}

	env := []string{"test_a"}

	// create test_a workspace
	newCmdView, newCmdDone := testView(t)
	newCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       newCmdView,
	}
	code = newCmd.Run(env)
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
	code = selCmd.Run(env)
	selCmdOutput := selCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, selCmdOutput.Stderr())
	}

	showCmd = &WorkspaceShowCommand{}
	showCmdView, showCmdDone = testView(t)
	showCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       showCmdView,
	}

	code = showCmd.Run(nil)
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
		newCmd := &WorkspaceNewCommand{
			Meta: Meta{
				WorkingDir: workdir.NewDir("."),
				View:       view,
			},
		}
		code := newCmd.Run([]string{env})
		output := done(t)
		if code == 0 {
			t.Fatalf("expected failure: \n%s", output.All())
		}
	}

	// list workspaces to make sure none were created
	listCmd := &WorkspaceListCommand{}
	listCmdView, listCmdDone := testView(t)
	listCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       listCmdView,
	}

	code := listCmd.Run(nil)
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
	initCmd := &InitCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       initCmdView,
		},
	}
	code := initCmd.Run(nil)
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
	newCmd := &WorkspaceNewCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       newCmdView,
		},
	}
	code = newCmd.Run(args)
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
	delCmd := &WorkspaceDeleteCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       delCmdView,
		},
	}

	current, _ := delCmd.Workspace(t.Context())
	if current != "test" {
		t.Fatal("wrong workspace:", current)
	}

	// we can't delete our current workspace
	args := []string{"test"}
	code := delCmd.Run(args)
	delCmdOutput := delCmdDone(t)
	if code == 0 {
		t.Fatalf("expected error deleting current workspace: %s", delCmdOutput.All())
	}

	// change back to default
	if err := delCmd.SetWorkspace(backend.DefaultStateName); err != nil {
		t.Fatal(err)
	}

	// try the delete again
	delCmdView, delCmdDone = testView(t)
	delCmd.Meta.View = delCmdView
	code = delCmd.Run(args)
	delCmdOutput = delCmdDone(t)
	if code != 0 {
		t.Fatalf("error deleting workspace: %s", delCmdOutput.Stderr())
	}

	current, _ = delCmd.Workspace(t.Context())
	if current != backend.DefaultStateName {
		t.Fatalf("wrong workspace: %q", current)
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
	delCmd := &WorkspaceDeleteCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       delCmdView,
		},
	}

	// delete the workspace
	code := delCmd.Run([]string{workspace})
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

	// create a non-empty state
	originalState := &legacy.State{
		Modules: []*legacy.ModuleState{
			{
				Path: []string{"root"},
				Resources: map[string]*legacy.ResourceState{
					"test_instance.foo": {
						Type: "test_instance",
						Primary: &legacy.InstanceState{
							ID: "bar",
						},
					},
				},
			},
		},
	}

	f, err := os.Create(filepath.Join(local.DefaultWorkspaceDir, "test", "terraform.tfstate"))
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.WriteState(originalState, f); err != nil {
		t.Fatal(err)
	}
	f.Close()

	delCmdView, delCmdDone := testView(t)
	delCmd := &WorkspaceDeleteCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       delCmdView,
		},
	}
	args := []string{"test"}
	code := delCmd.Run(args)
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
	delCmd.Meta.View = delCmdView

	args = []string{"-force", "test"}
	code = delCmd.Run(args)
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

	selectCmd := &WorkspaceSelectCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
		},
	}

	current, _ := selectCmd.Workspace(t.Context())
	if current != backend.DefaultStateName {
		t.Fatal("current workspace should be 'default'")
	}

	args := []string{"-or-create", "test"}
	selectCmdView, selectCmdDone := testView(t)
	selectCmd.Meta = Meta{
		WorkingDir: workdir.NewDir("."),
		View:       selectCmdView,
	}
	code := selectCmd.Run(args)
	selectCmdOutput := selectCmdDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, selectCmdOutput.Stderr())
	}

	current, _ = selectCmd.Workspace(t.Context())
	if current != "test" {
		t.Fatalf("current workspace should be 'test', got %q", current)
	}

}
