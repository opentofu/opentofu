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
	"github.com/hashicorp/go-version"
	"github.com/opentofu/opentofu/internal/command/workdir"
	"github.com/opentofu/opentofu/internal/modsdir"

	"github.com/opentofu/opentofu/internal/addrs"
	"github.com/opentofu/opentofu/internal/depsfile"
	"github.com/opentofu/opentofu/internal/getproviders"
)

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
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       view,
	}
	if err := meta.replaceLockedDependencies(context.Background(), locks); err != nil {
		t.Fatal(err)
	}

	version := "4.5.6"
	versionPrerelease := "foo"
	platform := getproviders.Platform{OS: "aros", Arch: "riscv64"}
	code := RunCommander(t, VersionCommander(version, versionPrerelease, platform), meta, []string{})
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
	meta := Meta{
		WorkingDir: workdir.NewDir("."),
		View:       view,
	}

	version := "4.5.6"
	versionPrerelease := "foo"
	platform := getproviders.Platform{OS: "aros", Arch: "riscv64"}

	code := RunCommander(t, VersionCommander(version, versionPrerelease, platform), meta, []string{"-v", "-version"})
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
	version := "4.5.6"
	platform := getproviders.Platform{OS: "aros", Arch: "riscv64"}
	code := RunCommander(t, VersionCommander(version, "", platform), meta, []string{"-json"})
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
	if err := meta.replaceLockedDependencies(context.Background(), locks); err != nil {
		t.Fatal(err)
	}
	version = "4.5.6"
	versionPrerelease := "foo"
	platform = getproviders.Platform{OS: "aros", Arch: "riscv64"}
	code = RunCommander(t, VersionCommander(version, versionPrerelease, platform), meta, []string{"-json"})
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

func TestModuleVersions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		records []modsdir.Record
		expect  map[string]string
	}{
		{
			name: "one module",
			records: []modsdir.Record{
				{
					Key:        "eks_cluster",
					SourceAddr: "registry.opentofu.org/terraform-aws-modules/eks/aws",
					Version:    version.Must(version.NewVersion("0.1.0")),
				},
			},
			expect: map[string]string{
				"registry.opentofu.org/terraform-aws-modules/eks/aws": "0.1.0",
			},
		},
		{
			name: "module and submodule",
			records: []modsdir.Record{
				{
					Key:        "eks_cluster",
					SourceAddr: "registry.opentofu.org/terraform-aws-modules/eks/aws",
					Version:    version.Must(version.NewVersion("0.1.0")),
				},
				{
					Key:        "eks_cluster.hello",
					SourceAddr: "./modules/hello",
				},
			},
			expect: map[string]string{
				"registry.opentofu.org/terraform-aws-modules/eks/aws":                "0.1.0",
				"registry.opentofu.org/terraform-aws-modules/eks/aws//modules/hello": "0.0.0",
			},
		},
		{
			name: "no parent",
			records: []modsdir.Record{
				{
					Key:        "eks_cluster.hello",
					SourceAddr: "./modules/hello",
				},
			},
			expect: map[string]string{
				"./modules/hello": "0.0.0",
			},
		},
		{
			name:    "no modules",
			records: []modsdir.Record{},
			expect:  map[string]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mv := readModuleVersions(tc.records)
			if diff := cmp.Diff(tc.expect, mv); diff != "" {
				t.Fatalf("wrong output\n%s", diff)
			}
		})
	}
}
