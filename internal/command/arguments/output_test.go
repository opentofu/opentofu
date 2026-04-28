// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseOutput_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *Output
		wantErrText string
	}{
		"defaults": {
			args: nil,
			want: outputArgsWithDefaults(func(a *Output) {
			}),
		},
		"json": {
			args: []string{"-json"},
			want: outputArgsWithDefaults(func(a *Output) {
				a.ViewOptions.ViewType = ViewJSON
			}),
		},
		"raw": {
			args: []string{"-raw", "foo"},
			want: outputArgsWithDefaults(func(a *Output) {
				a.Name = "foo"
				a.ViewOptions.ViewType = ViewRaw
			}),
		},
		"state": {
			args: []string{"-state=foobar.tfstate", "-raw", "foo"},
			want: outputArgsWithDefaults(func(a *Output) {
				a.Name = "foo"
				a.ViewOptions.ViewType = ViewRaw
				a.State.StatePath = "foobar.tfstate"
			}),
		},
		"unknown flag": {
			args:        []string{"-boop"},
			want:        outputArgsWithDefaults(func(a *Output) {}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -boop",
		},
		"json and raw specified": {
			args:        []string{"-json", "-raw"},
			want:        outputArgsWithDefaults(func(a *Output) {}),
			wantErrText: "Invalid output format: The -raw and -json options are mutually-exclusive.",
		},
		"raw with no name": {
			args: []string{"-raw"},
			want: outputArgsWithDefaults(func(a *Output) {
				a.ViewOptions.ViewType = ViewRaw
			}),
			wantErrText: "Output name required: You must give the name of a single output value when using the -raw option.",
		},
		"too many arguments": {
			args: []string{"-raw", "-state=foo.tfstate", "bar", "baz"},
			want: outputArgsWithDefaults(func(a *Output) {
				a.ViewOptions.ViewType = ViewRaw
				a.Name = "bar"
				a.State.StatePath = "foo.tfstate"
			}),
			wantErrText: "Unexpected argument: The output command expects exactly one argument with the name of an output variable or no arguments to show all outputs.",
		},
	}
	cmpOpts := cmpopts.IgnoreUnexported(ViewOptions{}, Vars{})
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseOutput(tc.args)
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

func outputArgsWithDefaults(mutate func(a *Output)) *Output {
	ret := &Output{
		Name:          "",
		ShowSensitive: false,
		ViewOptions: ViewOptions{
			ViewType: ViewHuman,
		},
		Vars:  &Vars{},
		State: &State{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
