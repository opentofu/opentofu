// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/collections"
	"github.com/opentofu/opentofu/internal/linting"
)

func TestParseView(t *testing.T) {
	testCases := map[string]struct {
		args    []string
		want    *View
		wantErr string
	}{
		"nil": {
			nil,
			viewArgsWithDefaults(nil),
			"",
		},
		"empty": {
			[]string{},
			viewArgsWithDefaults(nil),
			"",
		},
		"no-color": {
			[]string{"-no-color"},
			viewArgsWithDefaults(func(v *View) {
				v.NoColor = true
			}),
			"",
		},
		"compact-warnings": {
			[]string{"-compact-warnings"},
			viewArgsWithDefaults(func(v *View) {
				v.CompactWarnings = true
			}),
			"",
		},
		"concise": {
			[]string{"-concise"},
			viewArgsWithDefaults(func(v *View) {
				v.Concise = true
			}),
			"",
		},
		"no-color and compact-warnings": {
			[]string{"-no-color", "-compact-warnings"},
			viewArgsWithDefaults(func(v *View) {
				v.NoColor = true
				v.CompactWarnings = true
			}),
			"",
		},
		"no-color and concise": {
			[]string{"-no-color", "-concise"},
			viewArgsWithDefaults(func(v *View) {
				v.NoColor = true
				v.Concise = true
			}),
			"",
		},
		"concise and compact-warnings": {
			[]string{"-concise", "-compact-warnings"},
			viewArgsWithDefaults(func(v *View) {
				v.Concise = true
				v.CompactWarnings = true
			}),
			"",
		},
		"all three": {
			[]string{"-no-color", "-compact-warnings", "-concise"},
			viewArgsWithDefaults(func(v *View) {
				v.NoColor = true
				v.CompactWarnings = true
				v.Concise = true
			}),
			"",
		},
		"all three, resulting in empty args": {
			[]string{"-no-color", "-compact-warnings", "-concise"},
			viewArgsWithDefaults(func(v *View) {
				v.NoColor = true
				v.CompactWarnings = true
				v.Concise = true
			}),
			"",
		},
		"turn off warning consolidation": {
			[]string{"-consolidate-warnings=false"},
			viewArgsWithDefaults(func(v *View) {
				v.ConsolidateWarnings = false
			}),
			"",
		},
		"show all deprecation warnings": {
			[]string{"-deprecation=module:all"},
			viewArgsWithDefaults(func(v *View) {
				v.ModuleDeprecationWarnLvl = DeprecationWarningLevelAll
			}),
			"",
		},
		"show only local deprecation warnings": {
			[]string{"-deprecation=module:local"},
			viewArgsWithDefaults(func(v *View) {
				v.ModuleDeprecationWarnLvl = DeprecationWarningLevelLocal
			}),
			"",
		},
		"show no deprecation warnings": {
			[]string{"-deprecation=module:none"},
			viewArgsWithDefaults(func(v *View) {
				v.ModuleDeprecationWarnLvl = DeprecationWarningLevelNone
			}),
			"",
		},
		"deprecation used with other yet non-existing namespaces is returning those in the unparsed args": {
			[]string{"-deprecation=othernamespace:arg", "-deprecation=module:none", "-deprecation=backend:arg"},
			viewArgsWithDefaults(func(v *View) {
				v.ModuleDeprecationWarnLvl = DeprecationWarningLevelNone
			}),
			"Expected -deprecation prefix \"module:\"",
		},
		"lint includes 'all' rule": {
			[]string{"-lint=all"},
			viewArgsWithDefaults(func(v *View) {
				v.LintInclude = collections.NewSet(linting.AllRulesGroupID)
			}),
			"",
		},
		"lint excludes 'all' and allows 'foo'": {
			[]string{"-lint=!all,foo"},
			viewArgsWithDefaults(func(v *View) {
				v.LintInclude = collections.NewSet(linting.MustParseRuleAddr("foo"))
				v.LintExclude = collections.NewSet[linting.RuleAddr](linting.AllRulesGroupID)
			}),
			"",
		},
		"lint with invalid rule name": {
			[]string{"-lint=#foo"},
			viewArgsWithDefaults(func(v *View) {
				v.LintInclude = collections.NewSet[linting.RuleAddr]()
				v.LintExclude = collections.NewSet[linting.RuleAddr]()
			}),
			"",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var cli CommandLine

			tc.want.ViewType = ViewHuman

			got := BindView(&cli, viewFlagNone|viewFlagLint)
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

func viewArgsWithDefaults(mutate func(v *View)) *View {
	ret := &View{
		NoColor:                  false,
		CompactWarnings:          false,
		ConsolidateWarnings:      true,
		ConsolidateErrors:        false,
		LintInclude:              make(collections.Set[linting.RuleAddr]),
		LintExclude:              make(collections.Set[linting.RuleAddr]),
		Concise:                  false,
		ModuleDeprecationWarnLvl: DeprecationWarningLevelAll,
		ShowSensitive:            false,
		ViewType:                 ViewHuman,
		InputEnabled:             false, // because tests are executed with "viewFlagNone" so -input is not registered
		JSONInto:                 nil,
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
