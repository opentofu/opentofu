// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/go-cmp/cmp"
	"github.com/opentofu/opentofu/internal/tfdiags"
)

func TestParseShow_valid(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *Show
	}{
		"no options at all": {
			nil,
			&Show{
				TargetType: ShowState,
				TargetArg:  "",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
		},
		"json with no other options": {
			[]string{"-json"},
			&Show{
				TargetType: ShowState,
				TargetArg:  "",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
		},
		"latest state snapshot": {
			[]string{"-state"},
			&Show{
				TargetType: ShowState,
				TargetArg:  "",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
		},
		"latest state snapshot, JSON": {
			[]string{"-state", "-json"},
			&Show{
				TargetType: ShowState,
				TargetArg:  "",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
		},
		"saved plan file": {
			[]string{"-plan=tfplan"},
			&Show{
				TargetType: ShowPlan,
				TargetArg:  "tfplan",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
		},
		"saved plan file, JSON": {
			[]string{"-plan=tfplan", "-json"},
			&Show{
				TargetType: ShowPlan,
				TargetArg:  "tfplan",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
		},
		"legacy positional argument": {
			[]string{"foo"},
			&Show{
				TargetType: ShowUnknownType, // caller must inspect "foo" to decide the type
				TargetArg:  "foo",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
		},
		"json with legacy positional argument": {
			[]string{"-json", "foo"},
			&Show{
				TargetType: ShowUnknownType, // caller must inspect "foo" to decide the type
				TargetArg:  "foo",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
		},
		"configuration with json": {
			[]string{"-config", "-json"},
			&Show{
				TargetType: ShowConfig,
				TargetArg:  "",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
		},
		"module with json": {
			[]string{"-module=foo", "-json"},
			&Show{
				TargetType: ShowModule,
				TargetArg:  "foo",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, _, diags := ParseShow(tc.args)
			got.Vars = nil
			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
		})
	}
}

func TestParseShow_invalid(t *testing.T) {
	testCases := map[string]struct {
		args      []string
		want      *Show
		wantDiags tfdiags.Diagnostics
	}{
		"unknown option": {
			[]string{"-boop"},
			&Show{
				TargetType: ShowState,
				View:       &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Failed to parse command-line options",
					"flag provided but not defined: -boop",
				),
			},
		},
		"positional arguments with state target selection": {
			[]string{"-state", "bar"},
			&Show{
				View: &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Unexpected command line arguments",
					"This command does not expect any positional arguments when using a target-selection option.",
				),
			},
		},
		"positional arguments with planfile target selection": {
			[]string{"-plan=foo", "bar"},
			&Show{
				View: &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Unexpected command line arguments",
					"This command does not expect any positional arguments when using a target-selection option.",
				),
			},
		},
		"conflicting target selection options": {
			[]string{"-state", "-plan=foo"},
			&Show{
				TargetType: ShowPlan,
				TargetArg:  "foo",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Conflicting object types to show",
					"The -state, -plan=FILENAME, -config, and -module=DIR options are mutually-exclusive, to specify which kind of object to show.",
				),
			},
		},
		"too many arguments in legacy mode": {
			[]string{"bar", "baz"},
			&Show{
				View: &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Too many command line arguments",
					"Expected at most one positional argument for the legacy positional argument mode.",
				),
			},
		},
		"too many arguments in legacy mode, json": {
			[]string{"-json", "bar", "baz"},
			&Show{
				View: &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Too many command line arguments",
					"Expected at most one positional argument for the legacy positional argument mode.",
				),
			},
		},
		"configuration without json": {
			[]string{"-config"},
			&Show{
				View: &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"JSON output required for configuration",
					"The -config option requires -json to be specified.",
				),
			},
		},
		"configuration with state": {
			[]string{"-config", "-state", "-json"},
			&Show{
				TargetType: ShowConfig,
				TargetArg:  "",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Conflicting object types to show",
					"The -state, -plan=FILENAME, -config, and -module=DIR options are mutually-exclusive, to specify which kind of object to show.",
				),
			},
		},
		"configuration with plan": {
			[]string{"-config", "-plan=tfplan", "-json"},
			&Show{
				TargetType: ShowConfig,
				TargetArg:  "",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Conflicting object types to show",
					"The -state, -plan=FILENAME, -config, and -module=DIR options are mutually-exclusive, to specify which kind of object to show.",
				),
			},
		},
		"module without json": {
			[]string{"-module=foo"},
			&Show{
				View: &View{ConsolidateWarnings: true, ViewType: ViewHuman},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"JSON output required for module",
					"The -module=DIR option requires -json to be specified.",
				),
			},
		},
		"module with state": {
			[]string{"-module=foo", "-state", "-json"},
			&Show{
				TargetType: ShowModule,
				TargetArg:  "foo",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Conflicting object types to show",
					"The -state, -plan=FILENAME, -config, and -module=DIR options are mutually-exclusive, to specify which kind of object to show.",
				),
			},
		},
		"module with plan": {
			[]string{"-module=foo", "-plan=tfplan", "-json"},
			&Show{
				TargetType: ShowModule,
				TargetArg:  "foo",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Conflicting object types to show",
					"The -state, -plan=FILENAME, -config, and -module=DIR options are mutually-exclusive, to specify which kind of object to show.",
				),
			},
		},
		"module with config": {
			[]string{"-module=foo", "-config", "-json"},
			&Show{
				TargetType: ShowModule,
				TargetArg:  "foo",
				View:       &View{ConsolidateWarnings: true, ViewType: ViewJSON},
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Conflicting object types to show",
					"The -state, -plan=FILENAME, -config, and -module=DIR options are mutually-exclusive, to specify which kind of object to show.",
				),
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, _, gotDiags := ParseShow(tc.args)
			got.Vars = nil
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("unexpected result\n%s", diff)
			}
			if !reflect.DeepEqual(gotDiags, tc.wantDiags) {
				t.Errorf("wrong result\ngot: %s\nwant: %s", spew.Sdump(gotDiags), spew.Sdump(tc.wantDiags))
			}
		})
	}
}
