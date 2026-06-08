// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package workdir

import (
	"strings"
	"testing"
)

func TestExtractChdirOption(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantDir       string
		wantArgs      []string
		wantErrSubstr string
	}{
		{
			name:     "no args",
			args:     []string{},
			wantDir:  "",
			wantArgs: []string{},
		},
		{
			name:     "no chdir flag",
			args:     []string{"init"},
			wantDir:  "",
			wantArgs: []string{"init"},
		},
		// -chdir=<dir> original syntax
		{
			name:     "single dash equals",
			args:     []string{"-chdir=mydir", "init"},
			wantDir:  "mydir",
			wantArgs: []string{"init"},
		},
		{
			name:     "single dash equals with absolute path",
			args:     []string{"-chdir=/some/path", "plan"},
			wantDir:  "/some/path",
			wantArgs: []string{"plan"},
		},
		// --chdir=<dir>
		{
			name:     "double dash equals",
			args:     []string{"--chdir=mydir", "init"},
			wantDir:  "mydir",
			wantArgs: []string{"init"},
		},
		// -chdir <dir> space separated
		{
			name:     "single dash space separated",
			args:     []string{"-chdir", "mydir", "init"},
			wantDir:  "mydir",
			wantArgs: []string{"init"},
		},
		// --chdir <dir> space separated
		{
			name:     "double dash space separated",
			args:     []string{"--chdir", "mydir", "init"},
			wantDir:  "mydir",
			wantArgs: []string{"init"},
		},
		// Other flags before chdir are preserved
		{
			name:     "other flags preserved",
			args:     []string{"-chdir=mydir", "-flag", "init"},
			wantDir:  "mydir",
			wantArgs: []string{"-flag", "init"},
		},
		// chdir after subcommand should be ignored
		{
			name:     "chdir after subcommand ignored",
			args:     []string{"init", "-chdir=mydir"},
			wantDir:  "",
			wantArgs: []string{"init", "-chdir=mydir"},
		},
		// Error: equals sign but no value
		{
			name:          "single dash equals no value",
			args:          []string{"-chdir=", "init"},
			wantErrSubstr: "must include a directory path",
		},
		{
			name:          "double dash equals no value",
			args:          []string{"--chdir=", "init"},
			wantErrSubstr: "must include a directory path",
		},
		// Error: space separated but no following arg
		{
			name:          "single dash no following arg",
			args:          []string{"-chdir"},
			wantErrSubstr: "must include a directory path",
		},
		{
			name:          "double dash no following arg",
			args:          []string{"--chdir"},
			wantErrSubstr: "must include a directory path",
		},
		// Error: space separated but next arg is a flag
		{
			name:          "chdir followed by another flag",
			args:          []string{"-chdir", "-other", "init"},
			wantErrSubstr: "must include a directory path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDir, gotArgs, err := extractChdirOption(tt.args)

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErrSubstr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if gotDir != tt.wantDir {
				t.Errorf("dir: got %q, want %q", gotDir, tt.wantDir)
			}
			if !slicesEqual(gotArgs, tt.wantArgs) {
				t.Errorf("args: got %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
