// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cliconfig

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestContainerCLIConfigFilePaths(t *testing.T) {
	tests := []struct {
		goos          string
		home          string
		xdgConfigHome string
		xdgRuntimeDir string
		uid           int

		want []string // Must be written with forward slashes for test portability
	}{
		// Linux tests
		{
			goos: "linux",
			home: "/home/foo",
			uid:  1,

			want: []string{
				"/run/containers/1/auth.json",
				"/home/foo/.config/containers/auth.json",
			},
		},
		{
			goos:          "linux",
			home:          "/home/foo",
			xdgConfigHome: "/home/foo/not-config",
			uid:           1,

			want: []string{
				"/run/containers/1/auth.json",
				"/home/foo/not-config/containers/auth.json",
			},
		},
		{
			goos:          "linux",
			home:          "/home/foo",
			xdgRuntimeDir: "/home/foo/run",
			uid:           1,

			want: []string{
				"/home/foo/run/containers/auth.json",
				"/home/foo/.config/containers/auth.json",
			},
		},

		// Windows tests
		// (There isn't really any Windows-specific behavior in this function,
		// so this is largely the same as the Darwin tests below but with some
		// Windows-shaped input paths and the special case that os.Getuid
		// always returns -1.)
		{
			goos: "windows",
			home: `c:\Users\foo`,
			uid:  -1,

			want: []string{
				// Generating the same path twice matches Podman's behavior.
				"c:/Users/foo/.config/containers/auth.json",
				"c:/Users/foo/.config/containers/auth.json",
			},
		},
		{
			goos:          "windows",
			home:          `c:\Users\foo`,
			xdgConfigHome: `\\something-shared\foo`,
			uid:           -1,

			want: []string{
				"c:/Users/foo/.config/containers/auth.json",
				"//something-shared/foo/containers/auth.json",
			},
		},
		{
			goos:          "windows",
			home:          `c:\Users\foo`,
			xdgRuntimeDir: `\\something-shared\foo`,
			uid:           -1,

			want: []string{
				// XDG_RUNTIME_DIR is ignored by Podman everywhere except Linux
				"c:/Users/foo/.config/containers/auth.json",
				"c:/Users/foo/.config/containers/auth.json",
			},
		},

		// Darwin tests
		// (These effectively cover for everything else that isn't Linux
		// or Windows too, since this just exercises the fallback paths.)
		{
			goos: "darwin",
			home: "/home/foo",
			uid:  1,

			want: []string{
				// Returning the same path twice matches Podman's behavior in this case
				"/home/foo/.config/containers/auth.json",
				"/home/foo/.config/containers/auth.json",
			},
		},
		{
			goos:          "darwin",
			home:          "/home/foo",
			xdgConfigHome: "/home/foo/not-config",
			uid:           1,

			want: []string{
				// Effectively prioritizing the _default_ XDG_CONFIG_HOME over the
				// overridden one seems to match Podman's behavior in this case.
				"/home/foo/.config/containers/auth.json",
				"/home/foo/not-config/containers/auth.json",
			},
		},
		{
			goos:          "darwin",
			home:          "/home/foo",
			xdgRuntimeDir: "/home/foo/run",
			uid:           1,

			want: []string{
				// Returning the same path twice matches Podman's behavior in this case.
				// Podman only considers XDG_RUNTIME_DIR on Linux systems.
				"/home/foo/.config/containers/auth.json",
				"/home/foo/.config/containers/auth.json",
			},
		},
	}

	for _, test := range tests {
		var testNameBuilder strings.Builder
		fmt.Fprintf(&testNameBuilder, "on %s with homedir %s", test.goos, test.home)
		if test.xdgConfigHome != "" {
			fmt.Fprintf(&testNameBuilder, ", XDG_CONFIG_HOME=%s", test.xdgConfigHome)
		}
		if test.xdgRuntimeDir != "" {
			fmt.Fprintf(&testNameBuilder, ", XDG_RUNTIME_DIR=%s", test.xdgRuntimeDir)
		}
		fmt.Fprintf(&testNameBuilder, ", and uid=%d", test.uid)
		testName := testNameBuilder.String()

		t.Run(testName, func(t *testing.T) {
			t.Log(testName) // easier to read without the test name escaping

			got := containerCLIConfigFilePaths(
				test.goos,
				test.home,
				test.xdgConfigHome,
				test.xdgRuntimeDir,
				test.uid,
			)

			// The function returns paths using the syntax of the OS where
			// this test is running, so we'll transform our results to
			// always use forward-slashes, just so this test can be
			// portable. Unfortunately we can't use filepath.ToSlash
			// for this because it's written to do absolutely nothing
			// unless it's running on Windows.
			for i := range got {
				got[i] = strings.ReplaceAll(got[i], "\\", "/")
			}
			t.Logf("\ngot  %#v\nwant %#v", got, test.want)

			if diff := cmp.Diff(test.want, got); diff != "" {
				t.Error("wrong result\n" + diff)
			}
		})
	}
}
