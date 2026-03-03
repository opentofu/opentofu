// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseProvidersSchema_jsonValidation(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *ProvidersSchema
	}{
		"valid json": {
			[]string{"-json"},
			getProvidersSchemaArgsWithDefaults(func(ps *ProvidersSchema) {
				ps.ViewOptions.ViewType = ViewJSON
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(Vars{}, ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProvidersSchema(tc.args)
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

func TestParseProvidersSchema_missingJsonCheck(t *testing.T) {
	testCases := map[string]struct {
		args []string
	}{
		"missing json": {
			args: []string{}, // No JSON flag
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, closer, diags := ParseProvidersSchema(tc.args)
			defer closer()

			if len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if got, want := diags.Err().Error(), "Output only in json is allowed"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
			if got, want := diags.Err().Error(), "The `tofu providers schema` command requires the `-json` flag."; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestParseProvidersSchema_tooManyArguments(t *testing.T) {
	testCases := map[string]struct {
		args []string
	}{
		"one positional argument with json": {
			args: []string{"-json", "foo"},
		},
		"multiple positional arguments with json": {
			args: []string{"-json", "foo", "bar"},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			_, closer, diags := ParseProvidersSchema(tc.args)
			defer closer()

			if len(diags) == 0 {
				t.Fatal("expected diagnostics but got none")
			}
			if got, want := diags.Err().Error(), "Too many command line arguments"; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
			if got, want := diags.Err().Error(), "Expected at most zero positional arguments."; !strings.Contains(got, want) {
				t.Fatalf("wrong diags\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func getProvidersSchemaArgsWithDefaults(mutate func(ps *ProvidersSchema)) *ProvidersSchema {
	ret := &ProvidersSchema{
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
