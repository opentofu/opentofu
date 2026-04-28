// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opentofu/opentofu/internal/addrs"
)

func TestParseTaint_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *Taint
		wantErrText string
		forTaintCmd bool
	}{
		"no arguments": {
			args:        nil,
			want:        taintArgsWithDefaults(nil),
			wantErrText: "The taint command expects exactly one argument.",
			forTaintCmd: true,
		},
		"too many arguments": {
			args:        []string{"test_instance.foo", "test_instance.bar"},
			want:        taintArgsWithDefaults(nil),
			wantErrText: "The taint command expects exactly one argument.",
			forTaintCmd: true,
		},
		"valid resource address": {
			args: []string{"test_instance.foo"},
			want: taintArgsWithDefaults(func(v *Taint) {
				v.TargetAddress = addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test_instance",
					Name: "foo",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
			}),
			forTaintCmd: true,
		},
		"valid resource instance address": {
			args: []string{"test_instance.bar[0]"},
			want: taintArgsWithDefaults(func(v *Taint) {
				v.TargetAddress = addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test_instance",
					Name: "bar",
				}.Instance(addrs.IntKey(0)).Absolute(addrs.RootModuleInstance)
			}),
			forTaintCmd: true,
		},
		"data source error for taint cmd": {
			args: []string{"data.test_data.foo"},
			want: taintArgsWithDefaults(func(v *Taint) {
				v.TargetAddress = addrs.Resource{
					Mode: addrs.DataResourceMode,
					Type: "test_data",
					Name: "foo",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
			}),
			wantErrText: "Resource instance data.test_data.foo cannot be tainted",
			forTaintCmd: true,
		},
		"data source error for untaint cmd": {
			args: []string{"data.test_data.foo"},
			want: taintArgsWithDefaults(func(v *Taint) {
				v.TargetAddress = addrs.Resource{
					Mode: addrs.DataResourceMode,
					Type: "test_data",
					Name: "foo",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
			}),
			forTaintCmd: false, // for untaint we don't return the same error as we do for taint cmd
		},
		"invalid resource address for taint cmd": {
			args:        []string{"invalid-address"},
			want:        taintArgsWithDefaults(nil),
			wantErrText: "Resource specification must include a resource type and name.",
			forTaintCmd: true,
		},
		"allow-missing flag": {
			args: []string{"-allow-missing", "test_instance.foo"},
			want: taintArgsWithDefaults(func(v *Taint) {
				v.AllowMissing = true
				v.TargetAddress = addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test_instance",
					Name: "foo",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
			}),
		},
		"unknown flag": {
			args:        []string{"-unknown-flag", "test_instance.foo"},
			want:        taintArgsWithDefaults(func(v *Taint) {}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -unknown-flag",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}, State{}, Backend{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseTaint(tc.forTaintCmd, tc.args)
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

func TestParseTaint_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      []string{"test_instance.foo"},
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar", "test_instance.foo"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars", "test_instance.foo"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2", "test_instance.foo"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		for _, isTaint := range []bool{true, false} {
			tcName := fmt.Sprintf("%s_%t", name, isTaint)
			t.Run(tcName, func(t *testing.T) {
				got, closer, diags := ParseTaint(false, tc.args)
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
}

func taintArgsWithDefaults(mutate func(v *Taint)) *Taint {
	ret := &Taint{
		TargetAddress: addrs.AbsResourceInstance{},
		AllowMissing:  false,
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: false,
		},
		Vars: &Vars{},
		State: &State{
			Lock: true,
		},
		Backend: &Backend{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
