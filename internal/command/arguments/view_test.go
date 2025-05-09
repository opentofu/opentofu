// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tofu"
)

func TestParseView(t *testing.T) {
	testCases := map[string]struct {
		args     []string
		want     *View
		wantArgs []string
	}{
		"nil": {
			nil,
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: true, Concise: false},
			nil,
		},
		"empty": {
			[]string{},
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: true, Concise: false},
			[]string{},
		},
		"none matching": {
			[]string{"-foo", "bar", "-baz"},
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: true, Concise: false},
			[]string{"-foo", "bar", "-baz"},
		},
		"no-color": {
			[]string{"-foo", "-no-color", "-baz"},
			&View{NoColor: true, CompactWarnings: false, ConsolidateWarnings: true, Concise: false},
			[]string{"-foo", "-baz"},
		},
		"compact-warnings": {
			[]string{"-foo", "-compact-warnings", "-baz"},
			&View{NoColor: false, CompactWarnings: true, ConsolidateWarnings: true, Concise: false},
			[]string{"-foo", "-baz"},
		},
		"concise": {
			[]string{"-foo", "-concise", "-baz"},
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: true, Concise: true},
			[]string{"-foo", "-baz"},
		},
		"no-color and compact-warnings": {
			[]string{"-foo", "-no-color", "-compact-warnings", "-baz"},
			&View{NoColor: true, CompactWarnings: true, ConsolidateWarnings: true, Concise: false},
			[]string{"-foo", "-baz"},
		},
		"no-color and concise": {
			[]string{"-foo", "-no-color", "-concise", "-baz"},
			&View{NoColor: true, CompactWarnings: false, ConsolidateWarnings: true, Concise: true},
			[]string{"-foo", "-baz"},
		},
		"concise and compact-warnings": {
			[]string{"-foo", "-concise", "-compact-warnings", "-baz"},
			&View{NoColor: false, CompactWarnings: true, ConsolidateWarnings: true, Concise: true},
			[]string{"-foo", "-baz"},
		},
		"all three": {
			[]string{"-foo", "-no-color", "-compact-warnings", "-concise", "-baz"},
			&View{NoColor: true, CompactWarnings: true, ConsolidateWarnings: true, Concise: true},
			[]string{"-foo", "-baz"},
		},
		"all three, resulting in empty args": {
			[]string{"-no-color", "-compact-warnings", "-concise"},
			&View{NoColor: true, CompactWarnings: true, ConsolidateWarnings: true, Concise: true},
			[]string{},
		},
		"turn off warning consolidation": {
			[]string{"-consolidate-warnings=false"},
			&View{NoColor: false, CompactWarnings: false, ConsolidateWarnings: false, Concise: false},
			[]string{},
		},
		"show all deprecation warnings": {
			[]string{"-deprecation=module:all"},
			&View{ModuleDeprecationWarnLvl: tofu.DeprecationWarningLevelAll, ConsolidateWarnings: true},
			[]string{},
		},
		"show only local deprecation warnings": {
			[]string{"-deprecation=module:local"},
			&View{ModuleDeprecationWarnLvl: tofu.DeprecationWarningLevelLocal, ConsolidateWarnings: true},
			[]string{},
		},
		"show no deprecation warnings": {
			[]string{"-deprecation=module:none"},
			&View{ModuleDeprecationWarnLvl: tofu.DeprecationWarningLevelNone, ConsolidateWarnings: true},
			[]string{},
		},
		"deprecation used with other yet non-existing namespaces is returning those in the unparsed args": {
			[]string{"-deprecation=othernamespace:arg", "-deprecation=module:none", "-deprecation=backend:arg"},
			&View{ModuleDeprecationWarnLvl: tofu.DeprecationWarningLevelNone, ConsolidateWarnings: true},
			[]string{"-deprecation=othernamespace:arg", "-deprecation=backend:arg"},
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, gotArgs := ParseView(tc.args)
			if *got != *tc.want {
				t.Errorf("unexpected result\n got: %#v\nwant: %#v", got, tc.want)
			}
			if !cmp.Equal(gotArgs, tc.wantArgs) {
				t.Errorf("unexpected args\n got: %#v\nwant: %#v", gotArgs, tc.wantArgs)
			}
		})
	}
}
