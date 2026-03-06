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

func TestParseProvidersSchema_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *ProvidersSchema
		wantDiags   bool
		wantContain []string
	}{
		"valid json flag": {
			args: []string{"-json"},
			want: providersSchemaArgsWithDefaults(func(ps *ProvidersSchema) {
				ps.ViewOptions.ViewType = ViewJSON
			}),
		},
		"missing json flag": {
			args:      []string{},
			wantDiags: true,
			wantContain: []string{
				"Output only in json is allowed",
				"The `tofu providers schema` command requires the `-json` flag.",
			},
		},
		"one positional argument with json": {
			args:      []string{"-json", "foo"},
			wantDiags: true,
			wantContain: []string{
				"Too many command line arguments",
				"Expected at most zero positional arguments.",
			},
		},
		"multiple positional arguments with json": {
			args:      []string{"-json", "foo", "bar"},
			wantDiags: true,
			wantContain: []string{
				"Too many command line arguments",
				"Expected at most zero positional arguments.",
			},
		},
	}

	cmpOpts := cmp.Options{
		cmpopts.IgnoreUnexported(Vars{}, ViewOptions{}),
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProvidersSchema(tc.args)
			defer closer()

			if tc.wantDiags {
				if len(diags) == 0 {
					t.Fatal("expected diagnostics but got none")
				}
				for _, want := range tc.wantContain {
					if !strings.Contains(diags.Err().Error(), want) {
						t.Fatalf("wrong diags\n got: %s\nwant: %s", diags.Err().Error(), want)
					}
				}
				return
			}

			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got, cmpOpts); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func providersSchemaArgsWithDefaults(mutate func(ps *ProvidersSchema)) *ProvidersSchema {
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
