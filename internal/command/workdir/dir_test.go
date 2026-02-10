// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestWorkdirCreatedCorrectly(t *testing.T) {
	getOrDefault := func(in, def string) string {
		if in != "" {
			return in
		}
		return def
	}
	cases := map[string]struct {
		setup     func(t *testing.T, tempDir string) []string
		tfDataDir string

		wantNewArgs          []string
		wantDataDir          string
		wantMainDir          string
		wantOriginalDir      string
		wantWorkingDirSuffix string
	}{
		"without -chdir and without TF_DATA_DIR": {
			setup: func(t *testing.T, tempDir string) []string {
				return nil
			},
			wantDataDir: ".terraform",
		},
		"with relative -chdir and without TF_DATA_DIR": {
			setup: func(t *testing.T, tempDir string) []string {
				chdirModule := path.Join(tempDir, "root_module")
				if err := os.Mkdir(chdirModule, 0777); err != nil {
					t.Fatalf("failed to create %q: %s", chdirModule, err)
				}
				return []string{fmt.Sprintf("-chdir=%s", "root_module"), "-anotherflag=test"}
			},
			wantNewArgs:          []string{"-anotherflag=test"},
			wantDataDir:          ".terraform",
			wantWorkingDirSuffix: "root_module",
		},
		"with absolute -chdir and without TF_DATA_DIR": {
			setup: func(t *testing.T, tempDir string) []string {
				chdirModule := path.Join(tempDir, "root_module")
				if err := os.Mkdir(chdirModule, 0777); err != nil {
					t.Fatalf("failed to create %q: %s", chdirModule, err)
				}
				return []string{fmt.Sprintf("-chdir=%s", chdirModule), "-anotherflag=test"}
			},
			wantNewArgs:          []string{"-anotherflag=test"},
			wantDataDir:          ".terraform",
			wantWorkingDirSuffix: "root_module",
		},
		"without -chdir and with TF_DATA_DIR": {
			setup: func(t *testing.T, tempDir string) []string {
				t.Setenv("TF_DATA_DIR", "/just/a/random/path/since/it/is/not/checked")
				chdirModule := path.Join(tempDir, "root_module")
				if err := os.Mkdir(chdirModule, 0777); err != nil {
					t.Fatalf("failed to create %q: %s", chdirModule, err)
				}
				return []string{"-anotherflag=test"}
			},
			wantNewArgs: []string{"-anotherflag=test"},
			wantDataDir: filepath.Clean("/just/a/random/path/since/it/is/not/checked"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tempDir := t.TempDir()
			t.Chdir(tempDir)

			// execute the setup that gets back the args to be used when building the workdir
			tcArgs := tc.setup(t, tempDir)

			d, newArgs, err := NewWorkdir(tcArgs)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			t.Logf("got dir %#v", d)

			if diff := cmp.Diff(tc.wantNewArgs, newArgs); diff != "" {
				t.Fatalf("differences between expected and received args:\n%s", diff)
			}
			if got, want := d.dataDir, getOrDefault(tc.wantDataDir, tempDir); got != want {
				t.Errorf("expected dataDir %q but got %q", want, got)
			}
			if got, want := d.mainDir, "."; got != want {
				t.Errorf("expected mainDir %q but got %q", want, got)
			}
			if got, want := d.originalDir, tempDir; got != want {
				t.Errorf("expected originalDir %q but got %q", want, got)
			}
			// ensure that chdir has been applied or not correctly
			wd, err := os.Getwd()
			if err != nil {
				t.Fatalf("failed to get user's current dir: %s", err)
			}
			if got, want := wd, getOrDefault(tc.wantWorkingDirSuffix, tempDir); !strings.HasSuffix(got, want) {
				t.Errorf("current working directory is not the one expected. Wanted to end in %q but got only %q", want, got)
			}
		})
	}
}
