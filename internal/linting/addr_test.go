// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package linting

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseAddress(t *testing.T) {
	cases := []struct {
		in                           string
		expectedAddr                 RuleAddr
		expectedStringRepresentation string
		expectedErrMsg               string
	}{
		{
			in: "all",
			expectedAddr: RuleAddr{
				Namespace: "",
				Name:      "all",
			},
			expectedStringRepresentation: "all",
		},
		{
			in: "foo",
			expectedAddr: RuleAddr{
				Namespace: "",
				Name:      "foo",
			},
			expectedStringRepresentation: "foo",
		},
		{
			in: "core:foo",
			expectedAddr: RuleAddr{
				Namespace: "core",
				Name:      "foo",
			},
			expectedStringRepresentation: "core:foo",
		},
		{
			in: "baz:foo",
			expectedAddr: RuleAddr{
				Namespace: "baz",
				Name:      "foo",
			},
			expectedStringRepresentation: "baz:foo",
		},
		{
			in:             "my_namespace:foo:bar",
			expectedAddr:   RuleAddr{},
			expectedErrMsg: `invalid lint rule id "my_namespace:foo:bar". It does not match the required format or character restriction`,
		},
		{
			in:             "",
			expectedAddr:   RuleAddr{},
			expectedErrMsg: `invalid lint rule id "". It does not match the required format or character restriction`,
		},
		{
			in:             "baz:",
			expectedAddr:   RuleAddr{},
			expectedErrMsg: `invalid lint rule id "baz:". It does not match the required format or character restriction`,
		},
		{
			in:             ":foo",
			expectedAddr:   RuleAddr{},
			expectedErrMsg: `invalid lint rule id ":foo". It does not match the required format or character restriction`,
		},
		{
			in: "023a:bar",
			expectedAddr: RuleAddr{
				Namespace: "023a",
				Name:      "bar",
			},
			expectedStringRepresentation: "023a:bar",
		},
		{
			in:             "_foo:bar",
			expectedAddr:   RuleAddr{},
			expectedErrMsg: `invalid lint rule id "_foo:bar". It does not match the required format or character restriction`,
		},
		{
			in: "fo_o:bar",
			expectedAddr: RuleAddr{
				Namespace: "fo_o",
				Name:      "bar",
			},
			expectedStringRepresentation: "fo_o:bar",
		},
		{
			in: "foo_:bar",
			expectedAddr: RuleAddr{
				Namespace: "foo_",
				Name:      "bar",
			},
			expectedStringRepresentation: "foo_:bar",
		},
		{
			in:             "-foo:bar",
			expectedErrMsg: `invalid lint rule id "-foo:bar". It does not match the required format or character restriction`,
		},
		{
			in: "fo-o:bar",
			expectedAddr: RuleAddr{
				Namespace: "fo-o",
				Name:      "bar",
			},
			expectedStringRepresentation: "fo-o:bar",
		},
		{
			in: "foo-:bar",
			expectedAddr: RuleAddr{
				Namespace: "foo-",
				Name:      "bar",
			},
			expectedStringRepresentation: "foo-:bar",
		},
		{
			in:             "foo:_bar",
			expectedErrMsg: `invalid lint rule id "foo:_bar". It does not match the required format or character restriction`,
		},
		{
			in: "foo:ba_r",
			expectedAddr: RuleAddr{
				Namespace: "foo",
				Name:      "ba_r",
			},
			expectedStringRepresentation: "foo:ba_r",
		},
		{
			in: "foo:bar_",
			expectedAddr: RuleAddr{
				Namespace: "foo",
				Name:      "bar_",
			},
			expectedStringRepresentation: "foo:bar_",
		},
		{
			in:             "foo:-bar",
			expectedErrMsg: `invalid lint rule id "foo:-bar". It does not match the required format or character restriction`,
		},
		{
			in: "foo:ba-r",
			expectedAddr: RuleAddr{
				Namespace: "foo",
				Name:      "ba-r",
			},
			expectedStringRepresentation: "foo:ba-r",
		},
		{
			in: "foo:bar-",
			expectedAddr: RuleAddr{
				Namespace: "foo",
				Name:      "bar-",
			},
			expectedStringRepresentation: "foo:bar-",
		},
		{
			in: "foo/buz:bar",
			expectedAddr: RuleAddr{
				Namespace: "foo/buz",
				Name:      "bar",
			},
			expectedStringRepresentation: "foo/buz:bar",
		},
		{
			in: "foo/:bar",
			expectedAddr: RuleAddr{
				Namespace: "foo/",
				Name:      "bar",
			},
			expectedStringRepresentation: "foo/:bar",
		},
		{
			in:             "/foo:bar",
			expectedErrMsg: `invalid lint rule id "/foo:bar". It does not match the required format or character restriction`,
		},
		{
			in:             "foo:",
			expectedErrMsg: `invalid lint rule id "foo:". It does not match the required format or character restriction`,
		},
		{
			in:             "!foo:bar",
			expectedErrMsg: `invalid lint rule id "!foo:bar". It does not match the required format or character restriction`,
		},
		{
			in:             "|foo:bar",
			expectedErrMsg: `invalid lint rule id "|foo:bar". It does not match the required format or character restriction`,
		},
		{
			in:             "foo:  ",
			expectedErrMsg: `invalid lint rule id "foo:  ". It does not match the required format or character restriction`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			res, err := ParseRuleAddr(tc.in)
			gotErrMsg := ""
			if err != nil {
				gotErrMsg = err.Error()
			}
			if diff := cmp.Diff(tc.expectedErrMsg, gotErrMsg); diff != "" {
				t.Errorf("invalid error (-want,+got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.expectedAddr, res); diff != "" {
				t.Errorf("invalid resulted rule address (-want,+got):\n%s", diff)
			}
			if gotErrMsg == "" { // we check this only when there is no error
				gotStr := res.String()
				if gotStr != tc.expectedStringRepresentation {
					t.Errorf("invalid string representation returned for %q. wanted %q but got %q", tc.in, tc.expectedStringRepresentation, gotStr)
				}
			}
		})
	}
}
