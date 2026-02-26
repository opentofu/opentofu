// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package arguments

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestParseFmt_basicValidation(t *testing.T) {
	testCases := map[string]struct {
		args []string
		want *Fmt
	}{
		"defaults": {
			nil,
			fmtArgsWithDefaults(nil),
		},
		"list": {
			[]string{"-list=false"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.List = false
			}),
		},
		"write": {
			[]string{"-write=false"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Write = false
			}),
		},
		"diff": {
			[]string{"-diff"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Diff = true
			}),
		},
		"diff with value": {
			[]string{"-diff=true"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Diff = true
			}),
		},
		"check": {
			[]string{"-check"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Check = true
			}),
		},
		"recursive": {
			[]string{"-recursive"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Recursive = true
			}),
		},
		"file args": {
			[]string{"foo", "bar"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Paths = []string{"foo", "bar"}
			}),
		},
		"args with stdin in front": {
			[]string{"-", "bar"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Paths = nil
				v.List = false
				v.Write = false
			}),
		},
		"args with stdin not on the first index": {
			[]string{"foo", "-", "bar"},
			fmtArgsWithDefaults(func(v *Fmt) {
				v.Paths = []string{"foo", "-", "bar"}
			}),
		},
	}

	cmpOpts := cmpopts.IgnoreUnexported(ViewOptions{})

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			got, closer, diags := ParseFmt(tc.args)
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

func fmtArgsWithDefaults(mutate func(v *Fmt)) *Fmt {
	ret := &Fmt{
		Paths:     []string{"."},
		List:      true,
		Write:     true,
		Diff:      false,
		Check:     false,
		Recursive: false,
		ViewOptions: ViewOptions{
			jsonFlag:     false,
			jsonIntoFlag: "",
			ViewType:     ViewHuman,
			InputEnabled: false,
			JSONInto:     nil,
		},
	}
	if mutate != nil {
		mutate(ret)
	}
	return ret
}
