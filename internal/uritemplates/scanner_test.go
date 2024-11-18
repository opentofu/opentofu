// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package uritemplates

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestScanWithURITemplateSplit(t *testing.T) {
	tests := []struct {
		input      string
		wantTokens []string
		wantErr    string
	}{
		{
			``,
			nil,
			``,
		},
		{
			`foo`,
			[]string{`foo`},
			``,
		},
		{
			`foo{bar}`,
			[]string{`foo`, `{bar}`},
			``,
		},
		{
			`foo{bar}baz`,
			[]string{`foo`, `{bar}`, `baz`},
			``,
		},
		{
			`{bar}baz`,
			[]string{`{bar}`, `baz`},
			``,
		},
		{
			`{bar}`,
			[]string{`{bar}`},
			``,
		},
		{
			`{oops`,
			nil,
			`unclosed URI template expression`,
		},
		{
			`whoopsy{daisy`,
			[]string{"whoopsy"},
			`unclosed URI template expression`,
		},
		{
			`uh{oh}this{isnt valid`,
			[]string{"uh", "{oh}", "this"},
			`unclosed URI template expression`,
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			sc := newScanner(test.input)

			var gotTokens []string
			for sc.Scan() {
				gotTokens = append(gotTokens, sc.Text())
			}
			gotErr := sc.Err()

			if test.wantErr != "" {
				if gotErr == nil {
					t.Errorf("unexpected success\n  want error: %s", test.wantErr)
				} else if gotErr.Error() != test.wantErr {
					t.Errorf("wrong error\n  got:  %s\nwant: %s", gotErr.Error(), test.wantErr)
				}
			} else if gotErr != nil {
				t.Errorf("unexpected error: %s", gotErr)
			}

			if diff := cmp.Diff(test.wantTokens, gotTokens); diff != "" {
				t.Error("wrong tokens\n" + diff)
			}
		})
	}
}
