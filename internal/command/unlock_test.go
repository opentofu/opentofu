// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/backend/remote-state/inmem"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/workdir"

	legacy "github.com/opentofu/opentofu/internal/legacy/tofu"
)

// Since we can't unlock a local state file, just test that calling unlock
// doesn't fail.
func TestUnlock(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	// Write the legacy state
	statePath := arguments.DefaultStateFilename
	{
		f, err := os.Create(statePath)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		err = legacy.WriteState(legacy.NewState(), f)
		f.Close()
		if err != nil {
			t.Fatalf("err: %s", err)
		}
	}

	p := testProvider()
	view, done := testView(t)
	c := &UnlockCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-force",
		"LOCK_ID",
	}

	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("bad: %d\n%s", code, output.All())
	}

	// make sure we don't crash with arguments in the wrong order
	args = []string{
		"LOCK_ID",
		"-force",
	}
	view, done = testView(t)
	c.Meta.View = view
	code = c.Run(args)
	output = done(t)
	if code != cli.RunResultHelp {
		t.Fatalf("bad: %d\n%s", code, output.All())
	}
}

// Newly configured backend
func TestUnlock_inmemBackend(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("backend-inmem-locked"), td)
	t.Chdir(td)
	defer inmem.Reset()

	// init backend
	initView, initDone := testView(t)
	ci := &InitCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       initView,
		},
	}
	code := ci.Run(nil)
	initOutput := initDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n%s", code, initOutput.Stderr())
	}

	unlockView, unlockDone := testView(t)
	c := &UnlockCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       unlockView,
		},
	}

	// run with the incorrect lock ID
	args := []string{
		"-force",
		"LOCK_ID",
	}

	code = c.Run(args)
	unlockOutput := unlockDone(t)
	if code == 0 {
		t.Fatalf("bad: %d\n%s", code, unlockOutput.All())
	}

	unlockView, unlockDone = testView(t)
	c = &UnlockCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       unlockView,
		},
	}

	// lockID set in the test fixture
	args[1] = "2b6a6738-5dd5-50d6-c0ae-f6352977666b"
	code = c.Run(args)
	unlockOutput = unlockDone(t)
	if code != 0 {
		t.Fatalf("bad: %d\n%s", code, unlockOutput.All())
	}
}
