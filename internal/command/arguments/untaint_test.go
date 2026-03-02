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
	"github.com/opentofu/opentofu/internal/addrs"
)

func TestParseUntaint_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *Untaint
		wantErrText string
	}{
		"no arguments": {
			args:        nil,
			want:        untaintArgsWithDefaults(nil),
			wantErrText: "The untaint command expects exactly one argument.",
		},
		"too many arguments": {
			args:        []string{"test_instance.foo", "test_instance.bar"},
			want:        untaintArgsWithDefaults(nil),
			wantErrText: "The untaint command expects exactly one argument.",
		},
		"valid resource address": {
			args: []string{"test_instance.foo"},
			want: untaintArgsWithDefaults(func(v *Untaint) {
				v.TargetAddress = addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test_instance",
					Name: "foo",
				}.Instance(addrs.NoKey).Absolute(addrs.RootModuleInstance)
			}),
		},
		"valid resource instance address": {
			args: []string{"test_instance.bar[0]"},
			want: untaintArgsWithDefaults(func(v *Untaint) {
				v.TargetAddress = addrs.Resource{
					Mode: addrs.ManagedResourceMode,
					Type: "test_instance",
					Name: "bar",
				}.Instance(addrs.IntKey(0)).Absolute(addrs.RootModuleInstance)
			}),
		},
		"invalid resource address": {
			args:        []string{"invalid-address"},
			want:        untaintArgsWithDefaults(nil),
			wantErrText: "Resource specification must include a resource type and name.",
		},
		"allow-missing flag": {
			args: []string{"-allow-missing", "test_instance.foo"},
			want: untaintArgsWithDefaults(func(v *Untaint) {
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
			want:        untaintArgsWithDefaults(func(v *Untaint) {}),
			wantErrText: "Failed to parse command-line flags: flag provided but not defined: -unknown-flag",
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}, State{}, Backend{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseUntaint(tc.args)
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

func TestParseUntaint_vars(t *testing.T) {
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
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseUntaint(tc.args)
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

func untaintArgsWithDefaults(mutate func(v *Untaint)) *Untaint {
	ret := &Untaint{
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
