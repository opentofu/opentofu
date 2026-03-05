// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/command/arguments"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/states"
)

func TestUntaint(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	expected := strings.TrimSpace(`
test_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`)
	testStateOutput(t, statePath, expected)
}

func TestUntaint_lockedState(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)
	unlock, err := testLockState(t, testDataDir, statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code == 0 {
		t.Fatal("expected error")
	}

	stderr := output.Stderr()
	if !strings.Contains(stderr, "lock") {
		t.Fatalf("command output does not look like a lock error: %s", stderr)
	}
}

func TestUntaint_backup(t *testing.T) {
	// Get a temp cwd
	testCwdTemp(t)

	// Write the temp state
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	testStateFileDefault(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Backup is still tainted
	testStateOutput(t, arguments.DefaultStateFilename+".backup", strings.TrimSpace(`
test_instance.foo: (tainted)
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`))

	// State is untainted
	testStateOutput(t, arguments.DefaultStateFilename, strings.TrimSpace(`
test_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`))
}

func TestUntaint_backupDisable(t *testing.T) {
	// Get a temp cwd
	testCwdTemp(t)

	// Write the temp state
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	testStateFileDefault(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-backup", "-",
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	if _, err := os.Stat(arguments.DefaultStateFilename + ".backup"); err == nil {
		t.Fatal("backup path should not exist")
	}

	testStateOutput(t, arguments.DefaultStateFilename, strings.TrimSpace(`
test_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`))
}

func TestUntaint_badState(t *testing.T) {
	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-state", "i-should-not-exist-ever",
		"foo.name",
	}
	code := c.Run(args)
	output := done(t)
	if code != 1 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}
}

func TestUntaint_defaultState(t *testing.T) {
	// Get a temp cwd
	testCwdTemp(t)

	// Write the temp state
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	testStateFileDefault(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	testStateOutput(t, arguments.DefaultStateFilename, strings.TrimSpace(`
test_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`))
}

func TestUntaint_defaultWorkspaceState(t *testing.T) {
	// Get a temp cwd
	testCwdTemp(t)

	// Write the temp state
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	testWorkspace := "development"
	path := testStateFileWorkspaceDefault(t, testWorkspace, state)

	view, done := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       view,
	}
	if err := meta.SetWorkspace(testWorkspace); err != nil {
		t.Fatal(err)
	}
	c := &UntaintCommand{
		Meta: meta,
	}

	args := []string{
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	testStateOutput(t, path, strings.TrimSpace(`
test_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`))
}

func TestUntaint_missing(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-state", statePath,
		"test_instance.bar",
	}
	code := c.Run(args)
	output := done(t)
	if code == 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.All())
	}
}

func TestUntaint_missingAllow(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-no-color",
		"-allow-missing",
		"-state", statePath,
		"test_instance.bar",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	// Check for the warning
	actual := strings.TrimSpace(output.Stdout())
	expected := strings.TrimSpace(`
Warning: No such resource instance

Resource instance test_instance.bar was not found, but this is not an error
because -allow-missing was set.

`)
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatalf("wrong output\n%s", diff)
	}
}

func TestUntaint_stateOut(t *testing.T) {
	// Get a temp cwd
	testCwdTemp(t)

	// Write the temp state
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	testStateFileDefault(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-state-out", "foo",
		"test_instance.foo",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: %d\n\n%s", code, output.Stderr())
	}

	testStateOutput(t, arguments.DefaultStateFilename, strings.TrimSpace(`
test_instance.foo: (tainted)
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`))
	testStateOutput(t, "foo", strings.TrimSpace(`
test_instance.foo:
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]
	`))
}

func TestUntaint_module(t *testing.T) {
	state := states.BuildState(func(s *states.SyncState) {
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "foo",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
		s.SetResourceInstanceCurrent(
			addrs.Resource{
				Mode: addrs.ManagedResourceMode,
				Type: "test_instance",
				Name: "blah",
			}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance.Child("child", addrs.NoKey)),
			&states.ResourceInstanceObjectSrc{
				AttrsJSON: []byte(`{"id":"bar"}`),
				Status:    states.ObjectTainted,
			},
			addrs.AbsProviderConfig{
				Provider: addrs.NewDefaultProvider("test"),
				Module:   addrs.RootModule,
			},
			addrs.NoKey,
		)
	})
	statePath := testStateFile(t, state)

	view, done := testView(t)
	c := &UntaintCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
	}

	args := []string{
		"-state", statePath,
		"module.child.test_instance.blah",
	}
	code := c.Run(args)
	output := done(t)
	if code != 0 {
		t.Fatalf("command exited with status code %d; want 0\n\n%s", code, output.Stderr())
	}

	testStateOutput(t, statePath, strings.TrimSpace(`
test_instance.foo: (tainted)
  ID = bar
  provider = provider["registry.opentofu.org/hashicorp/test"]

module.child:
  test_instance.blah:
    ID = bar
    provider = provider["registry.opentofu.org/hashicorp/test"]
	`))
}
