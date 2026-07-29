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

func TestParseMetadataFunctions_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args        []string
		want        *MetadataFunctions
		wantErrText string
	}{
		"defaults": {
			args:        nil,
			want:        metadataFunctionsArgsWithDefaults(nil),
			wantErrText: "The `tofu metadata functions` command requires the `-json` flag.",
		},
		"json flag": {
			args: []string{"-json"},
			want: metadataFunctionsArgsWithDefaults(func(args *MetadataFunctions) {
				// The -json flag is just to validate that the user understands that the output will be in json.
				// While parsing the arguments, we revert back to human format to be sure that the diagnostics
				// are printed in human format
				args.View.ViewType = ViewHuman
			}),
		},
		"invalid flag": {
			args:        []string{"-foo"},
			want:        metadataFunctionsArgsWithDefaults(func(args *MetadataFunctions) {}),
			wantErrText: "flag provided but not defined: -foo",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseMetadataFunctions(tc.args)
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
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func metadataFunctionsArgsWithDefaults(mutate func(version *MetadataFunctions)) *MetadataFunctions {
	ret := &MetadataFunctions{
		View: &View{
			ConsolidateWarnings: true,
			ViewType:            ViewHuman,
			InputEnabled:        false,
		},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
