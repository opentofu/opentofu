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

func TestParseView(t *testing.T) {
	testCases := map[string]struct {
		args    []string
		want    *View
		wantErr string
	}{
		"nil": {
			nil,
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: true, Concise: false},
			"",
		},
		"empty": {
			[]string{},
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: true, Concise: false},
			"",
		},
		"no-color": {
			[]string{"-no-color"},
			&View{NoColor: true, CompactWarnings: false, ConsolidateWarnings: true, Concise: false},
			"",
		},
		"compact-warnings": {
			[]string{"-compact-warnings"},
			&View{NoColor: false, CompactWarnings: true, ConsolidateWarnings: true, Concise: false},
			"",
		},
		"concise": {
			[]string{"-concise"},
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: true, Concise: true},
			"",
		},
		"no-color and compact-warnings": {
			[]string{"-no-color", "-compact-warnings"},
			&View{NoColor: true, CompactWarnings: true, ConsolidateWarnings: true, Concise: false},
			"",
		},
		"no-color and concise": {
			[]string{"-no-color", "-concise"},
			&View{NoColor: true, CompactWarnings: false, ConsolidateWarnings: true, Concise: true},
			"",
		},
		"concise and compact-warnings": {
			[]string{"-concise", "-compact-warnings"},
			&View{NoColor: false, CompactWarnings: true, ConsolidateWarnings: true, Concise: true},
			"",
		},
		"all three": {
			[]string{"-no-color", "-compact-warnings", "-concise"},
			&View{NoColor: true, CompactWarnings: true, ConsolidateWarnings: true, Concise: true},
			"",
		},
		"all three, resulting in empty args": {
			[]string{"-no-color", "-compact-warnings", "-concise"},
			&View{NoColor: true, CompactWarnings: true, ConsolidateWarnings: true, Concise: true},
			"",
		},
		"turn off warning consolidation": {
			[]string{"-consolidate-warnings=false"},
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: false, Concise: false},
			"",
		},
		"show all deprecation warnings": {
			[]string{"-deprecation=module:all"},
			&View{ModuleDeprecationWarnLvl: DeprecationWarningLevelAll, ConsolidateWarnings: true},
			"",
		},
		"show only local deprecation warnings": {
			[]string{"-deprecation=module:local"},
			&View{ModuleDeprecationWarnLvl: DeprecationWarningLevelLocal, ConsolidateWarnings: true},
			"",
		},
		"show no deprecation warnings": {
			[]string{"-deprecation=module:none"},
			&View{ModuleDeprecationWarnLvl: DeprecationWarningLevelNone, ConsolidateWarnings: true},
			"",
		},
		"deprecation used with other yet non-existing namespaces is returning those in the unparsed args": {
			[]string{"-deprecation=othernamespace:arg", "-deprecation=module:none", "-deprecation=backend:arg"},
			&View{ModuleDeprecationWarnLvl: DeprecationWarningLevelNone, ConsolidateWarnings: true},
			"Expected -deprecation prefix \"module:\"",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var cli CommandLine

			tc.want.ViewType = ViewHuman

			got := BindView(&cli, viewFlagNone)
			_, diags := cli.parseWithHooks("view", tc.args)

			if tc.wantErr == "" && len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			} else if tc.wantErr != "" {
				if len(diags) == 0 {
					t.Fatalf("expected diags but got none")
				} else if got := diags.Err().Error(); !strings.Contains(got, tc.wantErr) {
					t.Fatalf("wrong diags\n got: %s\nwant: %s", got, tc.wantErr)
				}
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}
