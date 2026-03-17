// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"bytes"
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/backend"
	"github.com/opentofu/opentofu/internal/backend/remote-state/inmem"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/encryption"
	"github.com/opentofu/opentofu/internal/states"
)

func TestStatePush_empty(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-push-good"), td)
	t.Chdir(td)

	expected := testStateRead(t, "replace.tfstate")

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{"replace.tfstate"}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	actual := testStateRead(t, "local-state.tfstate")
	if !actual.Equal(expected) {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestStatePush_lockedState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-push-good"), td)
	t.Chdir(td)

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	unlock, err := testLockState(t, testDataDir, "local-state.tfstate")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	args := []string{"replace.tfstate"}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("bad code: %d, expected 1\n:%s", code, output.Stdout())
	}
	stderr := output.Stderr()
	if !strings.Contains(stderr, "Error acquiring the state lock") {
		t.Fatalf("expected a lock error, got: %s", stderr)
	}
}

func TestStatePush_replaceMatch(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-push-replace-match"), td)
	t.Chdir(td)

	expected := testStateRead(t, "replace.tfstate")

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{"replace.tfstate"}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	actual := testStateRead(t, "local-state.tfstate")
	if !actual.Equal(expected) {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestStatePush_replaceMatchStdin(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-push-replace-match"), td)
	t.Chdir(td)

	expected := testStateRead(t, "replace.tfstate")

	// Set up the replacement to come from stdin
	var buf bytes.Buffer
	if err := writeStateForTesting(expected, &buf); err != nil {
		t.Fatalf("err: %s", err)
	}
	defer testStdinPipe(t, &buf)()

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{"-force", "-"}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	actual := testStateRead(t, "local-state.tfstate")
	if !actual.Equal(expected) {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestStatePush_lineageMismatch(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-push-bad-lineage"), td)
	t.Chdir(td)

	expected := testStateRead(t, "local-state.tfstate")

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{"replace.tfstate"}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	actual := testStateRead(t, "local-state.tfstate")
	if !actual.Equal(expected) {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestStatePush_serialNewer(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-push-serial-newer"), td)
	t.Chdir(td)

	expected := testStateRead(t, "local-state.tfstate")

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{"replace.tfstate"}
	code := c.Run(args)
	_ = done(t)
	if code != 1 {
		t.Fatalf("bad: %d", code)
	}

	actual := testStateRead(t, "local-state.tfstate")
	if !actual.Equal(expected) {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestStatePush_serialOlder(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-push-serial-older"), td)
	t.Chdir(td)

	expected := testStateRead(t, "replace.tfstate")

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{"replace.tfstate"}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	actual := testStateRead(t, "local-state.tfstate")
	if !actual.Equal(expected) {
		t.Fatalf("bad: %#v", actual)
	}
}

func TestStatePush_forceRemoteState(t *testing.T) {
	td := t.TempDir()
	testCopyDir(t, testFixturePath("inmem-backend"), td)
	t.Chdir(td)
	defer inmem.Reset()

	s := states.NewState()
	statePath := testStateFile(t, s)

	// init the backend
	initView, initDone := testView(t)
	initCmd := &InitCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       initView,
		},
	}
	code := initCmd.Run([]string{})
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("bad exit code: %d\n output:\n%s", code, initOutput.All())
	}

	// create a new workspace
	wsView, wsDone := testView(t)
	newCmd := &WorkspaceNewCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       wsView,
		},
	}
	wsCode := newCmd.Run([]string{"test"})
	wsOutput := wsDone(t)
	if wsCode != 0 {
		t.Fatalf("bad exit code: %d\n\n%s", wsCode, wsOutput.All())
	}

	// put a dummy state in place, so we have something to force
	b := backend.TestBackendConfig(t, inmem.New(encryption.StateEncryptionDisabled()), nil)
	sMgr, err := b.StateMgr(t.Context(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := sMgr.WriteState(states.NewState()); err != nil {
		t.Fatal(err)
	}
	if err := sMgr.PersistState(t.Context(), nil); err != nil {
		t.Fatal(err)
	}

	// push our local state to that new workspace
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{"-force", statePath}
	pushCode := c.Run(args)
	output := done(t)
	if pushCode != 0 {
		t.Fatalf("bad code: %d\noutput:\n%s", pushCode, output.All())
	}
}

func TestStatePush_checkRequiredVersion(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("command-check-required-version"), td)
	t.Chdir(td)

	p := testProvider()
	view, done := testView(t)
	c := &StatePushCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{"replace.tfstate"}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("got exit status %d; want 1\nstderr:\n%s\n\nstdout:\n%s", code, output.Stderr(), output.Stdout())
	}

	// Required version diags are correct
	errStr := output.Stderr()
	if !strings.Contains(errStr, `required_version = "~> 0.9.0"`) {
		t.Fatalf("output should point to unmet version constraint, but is:\n\n%s", errStr)
	}
	if strings.Contains(errStr, `required_version = ">= 0.13.0"`) {
		t.Fatalf("output should not point to met version constraint, but is:\n\n%s", errStr)
	}
}
