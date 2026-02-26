// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/cli"
	"github.com/opentofu/opentofu/internal/command/workdir"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
)

func TestVersionCommand_implements(t *testing.T) {
	var _ cli.Command = &VersionCommand{}
}

func TestVersion(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	// We'll create a fixed dependency lock file in our working directory
	// so we can verify that the version command shows the information
	// from it.
	locks := depsfile.NewLocks()
	locks.SetProvider(
		addrs.NewDefaultProvider("test2"),
		getproviders.MustParseVersion("1.2.3"),
		nil,
		nil,
	)
	locks.SetProvider(
		addrs.NewDefaultProvider("test1"),
		getproviders.MustParseVersion("7.8.9-beta.2"),
		nil,
		nil,
	)

	view, done := testView(t)
	c := &VersionCommand{
		Meta: Meta{
			WorkingDir: workdir.NewDir("."),
			View:       view,
		},
		Version:           "4.5.6",
		VersionPrerelease: "foo",
		Platform:          getproviders.Platform{OS: "aros", Arch: "riscv64"},
	}
	if err := c.replaceLockedDependencies(context.Background(), locks); err != nil {
		t.Fatal(err)
	}
	code := c.Run([]string{})
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: \n%s", output.Stderr())
	}

	actual := strings.TrimSpace(output.Stdout())
	expected := "OpenTofu v4.5.6-foo\non aros_riscv64\n+ provider registry.opentofu.org/hashicorp/test1 v7.8.9-beta.2\n+ provider registry.opentofu.org/hashicorp/test2 v1.2.3"
	if actual != expected {
		t.Fatalf("wrong output\ngot:\n%s\nwant:\n%s", actual, expected)
	}

}

func TestVersion_flags(t *testing.T) {
	view, done := testView(t)
	m := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       view,
	}

	// `tofu version`
	c := &VersionCommand{
		Meta:              m,
		Version:           "4.5.6",
		VersionPrerelease: "foo",
		Platform:          getproviders.Platform{OS: "aros", Arch: "riscv64"},
	}

	code := c.Run([]string{"-v", "-version"})
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: \n%s", output.Stderr())
	}

	actual := strings.TrimSpace(output.Stdout())
	expected := "OpenTofu v4.5.6-foo\non aros_riscv64"
	if actual != expected {
		t.Fatalf("wrong output\ngot: %#v\nwant: %#v", actual, expected)
	}
}

func TestVersion_json(t *testing.T) {
	td := t.TempDir()
	t.Chdir(td)

	view, done := testView(t)
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       view,
	}

	// `tofu version -json` without prerelease
	c := &VersionCommand{
		Meta:     meta,
		Version:  "4.5.6",
		Platform: getproviders.Platform{OS: "aros", Arch: "riscv64"},
	}
	code := c.Run([]string{"-json"})
	output := done(t)
	if code != 0 {
		t.Fatalf("bad: \n%s", output.Stderr())
	}

	actual := strings.TrimSpace(output.Stdout())
	expected := strings.TrimSpace(`
{
  "terraform_version": "4.5.6",
  "platform": "aros_riscv64",
  "provider_selections": {}
}
`)
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatalf("wrong output\n%s", diff)
	}

	// reset view
	view, done = testView(t)
	meta.View = view

	// Now we'll create a fixed dependency lock file in our working directory
	// so we can verify that the version command shows the information
	// from it.
	locks := depsfile.NewLocks()
	locks.SetProvider(
		addrs.NewDefaultProvider("test2"),
		getproviders.MustParseVersion("1.2.3"),
		nil,
		nil,
	)
	locks.SetProvider(
		addrs.NewDefaultProvider("test1"),
		getproviders.MustParseVersion("7.8.9-beta.2"),
		nil,
		nil,
	)

	// `tofu version -json` with prerelease and provider dependencies
	c = &VersionCommand{
		Meta:              meta,
		Version:           "4.5.6",
		VersionPrerelease: "foo",
		Platform:          getproviders.Platform{OS: "aros", Arch: "riscv64"},
	}
	if err := c.replaceLockedDependencies(context.Background(), locks); err != nil {
		t.Fatal(err)
	}
	code = c.Run([]string{"-json"})
	output = done(t)
	if code != 0 {
		t.Fatalf("bad: \n%s", output.Stderr())
	}

	actual = strings.TrimSpace(output.Stdout())
	expected = strings.TrimSpace(`
{
  "terraform_version": "4.5.6-foo",
  "platform": "aros_riscv64",
  "provider_selections": {
    "registry.opentofu.org/hashicorp/test1": "7.8.9-beta.2",
    "registry.opentofu.org/hashicorp/test2": "1.2.3"
  }
}
`)
	if diff := cmp.Diff(expected, actual); diff != "" {
		t.Fatalf("wrong output\n%s", diff)
	}
}
