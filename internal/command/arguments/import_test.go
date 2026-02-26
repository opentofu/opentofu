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
		args []string
		want *Import
	}{
		"defaults": {
			[]string{"addr", "id"},
			importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
			}),
		},
		"parallelism flag": {
			[]string{"-parallelism=5", "addr", "id"},
			importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.Parallelism = 5
			}),
		},
		"config flag": {
			[]string{"-config=/path/to/config", "addr", "id"},
			importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.ConfigPath = "/path/to/config"
			}),
		},
		"ignore-remote-version flag": {
			[]string{"-ignore-remote-version", "addr", "id"},
			importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.Backend.IgnoreRemoteVersion = true
			}),
		},
		"state flags": {
			[]string{
				"-lock=false",
				"-lock-timeout=10s",
				"-state=foo.tfstate",
				"-state-out=bar.tfstate",
				"-backup=baz.tfstate",
				"addr",
				"id",
			},
			importArgsWithDefaults(func(imp *Import) {
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
			[]string{"-var=foo=bar", "-var-file=vars.tfvars", "addr", "id"},
			importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
			}),
		},
		"input flag": {
			[]string{"-input=false", "addr", "id"},
			importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.ViewOptions.InputEnabled = false
			}),
		},
		"json flag": {
			[]string{"-json", "addr", "id"},
			importArgsWithDefaults(func(imp *Import) {
				imp.ResourceAddress = "addr"
				imp.ResourceID = "id"
				imp.ViewOptions.ViewType = ViewJSON
				imp.ViewOptions.InputEnabled = false
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseImport(tc.args, wd)
			defer closer()

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func TestParseImport_argsValidation(t *testing.T) {
	wd := workdir.NewDir(".")
	testCases := map[string]struct {
		args        []string
		wantError   string
		wantDetails string
	}{
		"no arguments": {
			args:        []string{},
			wantError:   "Invalid number of arguments",
			wantDetails: "The import command expects two arguments",
		},
		"one argument": {
			args:        []string{"addr"},
			wantError:   "Invalid number of arguments",
			wantDetails: "The import command expects two arguments",
		},
		"too many arguments": {
			args:        []string{"addr", "id", "extra"},
			wantError:   "Invalid number of arguments",
			wantDetails: "The import command expects two arguments",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, closer, diags := ParseImport(tc.args, wd)
			defer closer()

			if len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			err := diags.Err()
			if err == nil {
				t.Fatal("expected error but got nil")
			}

			if got := err.Error(); !strings.Contains(got, tc.wantError) {
				t.Errorf("wrong error message\n got: %s\nwant: %s", got, tc.wantError)
			}

			if len(diags) > 0 {
				detail := diags[0].Description().Detail
				if detail != tc.wantDetails {
					t.Errorf("wrong diagnostic detail\n got: %s\nwant: %s", detail, tc.wantDetails)
				}
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
		State: &State{
			Lock:      true,
			StatePath: "",
		},
		Backend: &Backend{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
