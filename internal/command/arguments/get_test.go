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

func TestParseGet_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *Get
	}{
		"defaults": {
			nil,
			getArgsWithDefaults(nil),
		},
		"update flag": {
			[]string{"-update"},
			getArgsWithDefaults(func(get *Get) {
				get.Update = true
			}),
		},
		"custom test-directory": {
			[]string{"-test-directory=integration"},
			getArgsWithDefaults(func(get *Get) {
				get.TestsDirectory = "integration"
			}),
		},
		"multiple flags combined": {
			[]string{"-update", "-test-directory=e2e"},
			getArgsWithDefaults(func(get *Get) {
				get.Update = true
				get.TestsDirectory = "e2e"
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseGet(tc.args)
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

func TestParseGet_vars(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		wantCount int
		wantEmpty bool
	}{
		"no vars": {
			args:      nil,
			wantCount: 0,
			wantEmpty: true,
		},
		"single var": {
			args:      []string{"-var", "foo=bar"},
			wantCount: 1,
			wantEmpty: false,
		},
		"single var-file": {
			args:      []string{"-var-file", "terraform.tfvars"},
			wantCount: 1,
			wantEmpty: false,
		},
		"multiple vars mixed": {
			args:      []string{"-var", "a=1", "-var-file", "f.tfvars", "-var", "b=2"},
			wantCount: 3,
			wantEmpty: false,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseGet(tc.args)
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

func TestParseGet_tooManyArguments(t *testing.T) {
	testCases := map[string]struct {
		args []string
	}{
		"one positional argument": {
			args: []string{"mydir"},
		},
		"multiple positional arguments": {
			args: []string{"dir1", "dir2"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, closer, diags := ParseGet(tc.args)
			defer closer()

			if len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if got, want := diags.Err().Error(), "Unexpected argument"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
			if got, want := diags.Err().Error(), "Too many command line arguments"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func getArgsWithDefaults(mutate func(get *Get)) *Get {
	ret := &Get{
		Update:         false,
		TestsDirectory: "tests",
		ViewOptions: ViewOptions{
			ViewType:     ViewHuman,
			InputEnabled: false,
		},
		Vars: &Vars{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
