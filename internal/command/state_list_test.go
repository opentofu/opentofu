// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/workdir"
)

func TestStateList(t *testing.T) {
	state := testState()
	statePath := testStateFile(t, state)

	p := testProvider()
	view, done := testView(t)
	c := &StateListCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-state", statePath,
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that outputs were displayed
	expected := strings.TrimSpace(testStateListOutput) + "\n"
	actual := output.Stdout()
	if actual != expected {
		t.Fatalf("Expected:\n%q\n\nTo equal: %q", actual, expected)
	}
}

func TestStateListWithID(t *testing.T) {
	state := testState()
	statePath := testStateFile(t, state)

	p := testProvider()
	view, done := testView(t)
	c := &StateListCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-state", statePath,
		"-id", "bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that outputs were displayed
	expected := strings.TrimSpace(testStateListOutput) + "\n"
	actual := output.Stdout()
	if actual != expected {
		t.Fatalf("Expected:\n%q\n\nTo equal: %q", actual, expected)
	}
}

func TestStateListWithNonExistentID(t *testing.T) {
	state := testState()
	statePath := testStateFile(t, state)

	p := testProvider()
	view, done := testView(t)
	c := &StateListCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{
		"-state", statePath,
		"-id", "baz",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that output is empty
	actual := output.Stdout()
	if actual != "" {
		t.Fatalf("Expected an empty output but got: %q", actual)
	}
}

func TestStateList_backendDefaultState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-list-backend-default"), td)
	t.Chdir(td)

	p := testProvider()
	view, done := testView(t)
	c := &StateListCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that outputs were displayed
	expected := "null_resource.a\n"
	actual := output.Stdout()
	if actual != expected {
		t.Fatalf("Expected:\n%q\n\nTo equal: %q", actual, expected)
	}
}

func TestStateList_backendCustomState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-list-backend-custom"), td)
	t.Chdir(td)

	p := testProvider()
	view, done := testView(t)
	c := &StateListCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	args := []string{}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Test that outputs were displayed
	expected := "null_resource.a\n"
	actual := output.Stdout()
	if actual != expected {
		t.Fatalf("Expected:\n%q\n\nTo equal: %q", actual, expected)
	}
}

func TestStateList_backendOverrideState(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-list-backend-custom"), td)
	t.Chdir(td)

	p := testProvider()
	view, done := testView(t)
	c := &StateListCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	// This test is configured to use a local backend that has
	// a custom path defined. So we test if we can still pass
	// is a user defined state file that will then override the
	// one configured in the backend. As this file does not exist
	// it should exit with a no state found error.
	args := []string{"-state=" + arguments.DefaultStateFilename}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("bad: %d", code)
	}
	if !strings.Contains(output.Stderr(), "No state file was found") {
		t.Fatalf("expected a no state file error, got: %s", output.Stderr())
	}
}

func TestStateList_noState(t *testing.T) {
	testCwdTemp(t)

	p := testProvider()
	view, done := testView(t)
	c := &StateListCommand{
		Meta: Meta{
			WorkingDir:       workdir.NewDir("."),
			testingOverrides: metaOverridesForProvider(p),
			View:             view,
		},
	}

	code := c.Run(nil)
	output := done(t)
	if code != 1 {
		t.Fatalf("bad exit code: %d. output: %s", code, output.All())
	}
}

func TestStateList_modules(t *testing.T) {
	// Create a temporary working directory that is empty
	td := t.TempDir()
	testCopyDir(t, testFixturePath("state-list-nested-modules"), td)
	t.Chdir(td)

	p := testProvider()

	t.Run("list resources in module and submodules", func(t *testing.T) {
		view, done := testView(t)
		c := &StateListCommand{
			Meta: Meta{
				WorkingDir:       workdir.NewDir("."),
				testingOverrides: metaOverridesForProvider(p),
				View:             view,
			},
		}
		args := []string{"module.nest"}
		code := c.Run(args)
		output := done(t)
		if code != 0 {
			t.Fatalf("bad: %d", code)
		}

		// resources in the module and any submodules should be included in the outputs
		expected := "module.nest.test_instance.nest\nmodule.nest.module.subnest.test_instance.subnest\n"
		actual := output.Stdout()
		if actual != expected {
			t.Fatalf("Expected:\n%q\n\nTo equal: %q", actual, expected)
		}
	})

	t.Run("submodule has resources only", func(t *testing.T) {
		// now get the state for a module that has no resources, only another nested module
		view, done := testView(t)
		c := &StateListCommand{
			Meta: Meta{
				WorkingDir:       workdir.NewDir("."),
				testingOverrides: metaOverridesForProvider(p),
				View:             view,
			},
		}
		args := []string{"module.nonexist"}
		code := c.Run(args)
		output := done(t)
		if code != 0 {
			t.Fatalf("bad: %d", code)
		}
		expected := "module.nonexist.module.child.test_instance.child\n"
		actual := output.Stdout()
		if actual != expected {
			t.Fatalf("Expected:\n%q\n\nTo equal: %q", actual, expected)
		}
	})

	t.Run("expanded module", func(t *testing.T) {
		// finally get the state for a module with an index
		view, done := testView(t)
		c := &StateListCommand{
			Meta: Meta{
				WorkingDir:       workdir.NewDir("."),
				testingOverrides: metaOverridesForProvider(p),
				View:             view,
			},
		}
		args := []string{"module.count"}
		code := c.Run(args)
		output := done(t)
		if code != 0 {
			t.Fatalf("bad: %d", code)
		}
		expected := "module.count[0].test_instance.count\nmodule.count[1].test_instance.count\n"
		actual := output.Stdout()
		if actual != expected {
			t.Fatalf("Expected:\n%q\n\nTo equal: %q", actual, expected)
		}
	})

	t.Run("completely nonexistent module", func(t *testing.T) {
		// finally get the state for a module with an index
		view, done := testView(t)
		c := &StateListCommand{
			Meta: Meta{
				WorkingDir:       workdir.NewDir("."),
				testingOverrides: metaOverridesForProvider(p),
				View:             view,
			},
		}
		args := []string{"module.notevenalittlebit"}
		code := c.Run(args)
		output := done(t)
		if code != 1 {
			t.Fatalf("bad exit code: %d. output: %s", code, output.All())
		}
	})
}

const testStateListOutput = `
test_instance.foo
`
