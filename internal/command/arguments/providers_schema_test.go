// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
				// even though the -json flag is given, that is only to force the user to handle the successfull output
				// of this command as json
				ps.View.ViewType = ViewHuman
			}),
		},
		"missing json flag": {
			args:      []string{},
			wantDiags: true,
			want:      providersSchemaArgsWithDefaults(nil),
			wantContain: []string{
				"The `tofu providers schema` command requires the `-json` flag.",
			},
		},
		"one positional argument with json": {
			args: []string{"-json", "foo"},
			want: providersSchemaArgsWithDefaults(func(ps *ProvidersSchema) {
				// even though the -json flag is given, that is only to force the user to handle the successfull output
				// of this command as json
				ps.View.ViewType = ViewHuman
			}),
			wantDiags: true,
			wantContain: []string{
				"Too many command line arguments. Did you mean to use -chdir",
			},
		},
		"multiple positional arguments with json": {
			args: []string{"-json", "foo", "bar"},
			want: providersSchemaArgsWithDefaults(func(ps *ProvidersSchema) {
				// even though the -json flag is given, that is only to force the user to handle the successfull output
				// of this command as json
				ps.View.ViewType = ViewHuman
			}),
			wantDiags: true,
			wantContain: []string{
				"Too many command line arguments. Did you mean to use -chdir",
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseProvidersSchema(tc.args)
			defer closer()

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}

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
		})
	}
}

func providersSchemaArgsWithDefaults(mutate func(ps *ProvidersSchema)) *ProvidersSchema {
	ret := &ProvidersSchema{
		View: &View{
			ConsolidateWarnings: true,
			ViewType:            ViewHuman,
			InputEnabled:        false,
		},
		Vars: &Vars{},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
