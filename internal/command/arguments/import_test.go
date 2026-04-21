// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opentofu/opentofu/internal/command/flags"
	"github.com/opentofu/opentofu/internal/command/workdir"
)

func TestParseImport_basicValidation(t *testing.T) {
	wd := workdir.NewDir(".")
	testCases := map[string]struct {
		args        []string
		want        *Import
		wantErrText string
	}{
		"defaults": {
			args: []string{"addr", "id"},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
			}),
		},
		"parallelism flag": {
			args: []string{"-parallelism=5", "addr", "id"},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.Parallelism = 5
			}),
		},
		"config flag": {
			args: []string{"-config=/path/to/config", "addr", "id"},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.ConfigPath = "/path/to/config"
			}),
		},
		"ignore-remote-version flag": {
			args: []string{"-ignore-remote-version", "addr", "id"},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.Backend.IgnoreRemoteVersion = true
			}),
		},
		"state flags": {
			args: []string{
				"-lock=false",
				"-lock-timeout=10s",
				"-state=foo.tfstate",
				"-state-out=bar.tfstate",
				"-backup=baz.tfstate",
				"addr",
				"id",
			},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.State.Lock = false
				imp.State.LockTimeout = 10 * time.Second
				imp.State.StatePath = "foo.tfstate"
				imp.State.StateOutPath = "bar.tfstate"
				imp.State.BackupPath = "baz.tfstate"
			}),
		},
		"var flags": {
			args: []string{"-var=foo=bar", "-var-file=vars.tfvars", "addr", "id"},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
			}),
		},
		"input flag": {
			args: []string{"-input=false", "addr", "id"},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.ViewOptions.InputEnabled = false
			}),
		},
		"json flag": {
			args: []string{"-json", "addr", "id"},
			want: importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.ViewOptions.ViewType = ViewJSON
				imp.ViewOptions.InputEnabled = false
			}),
		},
		"no arguments": {
			args:        []string{},
			want:        importArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments: The import command expects two arguments",
		},
		"one argument": {
			args:        []string{"addr"},
			want:        importArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments: The import command expects two arguments",
		},
		"too many arguments": {
			args:        []string{"addr", "id", "extra"},
			want:        importArgsWithDefaults(nil),
			wantErrText: "Invalid number of arguments: The import command expects two arguments",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseImport(tc.args, wd)
			defer closer()

			if tc.wantErrText != "" && len(diags) == 0 {
				t.Errorf("test wanted error but got nothing")
			} else if tc.wantErrText == "" && len(diags) > 0 {
				t.Errorf("test didn't expect errors but got some: %s", diags.ErrWithWarnings())
			} else if tc.wantErrText != "" && len(diags) > 0 {
				errStr := diags.ErrWithWarnings().Error()
				if !strings.Contains(errStr, tc.wantErrText) {
					t.Errorf("the returned diagnostics does not contain the expected error message.\ndiags:\n%s\nwanted: %s\n", errStr, tc.wantErrText)
				}
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func TestParseImport_vars(t *testing.T) {
	wd := workdir.NewDir(".")
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      []string{"addr", "id"},
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar", "addr", "id"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars", "addr", "id"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2", "addr", "id"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseImport(tc.args, wd)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if got.Vars.Empty() != tc.wantEmpty {
				t.Errorf("Vars.Empty() = %v, want %v", got.Vars.Empty(), tc.wantEmpty)
			}
			if len(got.Vars.All()) != tc.wantCount {
				t.Errorf("len(Vars.All()) = %d, want %d", len(got.Vars.All()), tc.wantCount)
			}
		})
	}
}

func importArgsWithDefaults(mutate func(imp *Import)) *Import {
	v := flags.NewRawFlags("-var")
	vf := flags.NewRawFlags("-var-file")
	ret := &Import{
		ResourceAddress: "",
		ResourceID:      "",
		ConfigPath:      ".",
		Parallelism:     DefaultParallelism,
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: true,
		},
		Vars: &Vars{
			vars:     &v,
			varFiles: &vf,
		},
		State:   NewStateFlags(),
		Backend: &Backend{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
