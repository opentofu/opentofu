// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
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
				ViewType:   ViewHuman,
			},
		},
		"json with no other options": {
			[]string{"-json"},
			&Show{
				TargetType: ShowState,
				TargetArg:  "",
				ViewType:   ViewJSON,
			},
		},
		"latest state snapshot": {
			[]string{"-state"},
			&Show{
				TargetType: ShowState,
				TargetArg:  "",
				ViewType:   ViewHuman,
			},
		},
		"latest state snapshot, JSON": {
			[]string{"-state", "-json"},
			&Show{
				TargetType: ShowState,
				TargetArg:  "",
				ViewType:   ViewJSON,
			},
		},
		"saved plan file": {
			[]string{"-plan=tfplan"},
			&Show{
				TargetType: ShowPlan,
				TargetArg:  "tfplan",
				ViewType:   ViewHuman,
			},
		},
		"saved plan file, JSON": {
			[]string{"-plan=tfplan", "-json"},
			&Show{
				TargetType: ShowPlan,
				TargetArg:  "tfplan",
				ViewType:   ViewJSON,
			},
		},
		"legacy positional argument": {
			[]string{"foo"},
			&Show{
				TargetType: ShowUnknownType, // caller must inspect "foo" to decide the type
				TargetArg:  "foo",
				ViewType:   ViewHuman,
			},
		},
		"json with legacy positional argument": {
			[]string{"-json", "foo"},
			&Show{
				TargetType: ShowUnknownType, // caller must inspect "foo" to decide the type
				TargetArg:  "foo",
				ViewType:   ViewJSON,
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, diags := ParseShow(tc.args)
			got.Vars = nil
			if len(diags) > 0 {
				t.Fatalf("unexpected diags: %v", diags)
			}
			if *got != *tc.want {
				t.Fatalf("unexpected result\n got: %#v\nwant: %#v", got, tc.want)
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
				ViewType:   ViewHuman,
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
				ViewType: ViewHuman,
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
				ViewType: ViewHuman,
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
				ViewType:   ViewHuman,
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Conflicting object types to show",
					"The -state and -plan=FILENAME options are mutually-exclusive, to specify which kind of object to show.",
				),
			},
		},
		"too many arguments in legacy mode": {
			[]string{"bar", "baz"},
			&Show{
				ViewType: ViewHuman,
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
				ViewType: ViewJSON,
			},
			tfdiags.Diagnostics{
				tfdiags.Sourceless(
					tfdiags.Error,
					"Too many command line arguments",
					"Expected at most one positional argument for the legacy positional argument mode.",
				),
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, gotDiags := ParseShow(tc.args)
			got.Vars = nil
			if *got != *tc.want {
				t.Fatalf("unexpected result\n got: %#v\nwant: %#v", got, tc.want)
			}
			if !reflect.DeepEqual(gotDiags, tc.wantDiags) {
				t.Errorf("wrong result\ngot: %s\nwant: %s", spew.Sdump(gotDiags), spew.Sdump(tc.wantDiags))
			}
		})
	}
}
