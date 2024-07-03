// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tofu

import (
	"fmt"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/opentofu/opentofu/internal/lang/marks"
)

func TestMarksEqual(t *testing.T) {
	for i, tc := range []struct {
		a, b  []cty.PathValueMarks
		equal bool
	}{
		{
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			true,
		},
		{
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "A"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			false,
		},
		{
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "b"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "c"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "b"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "c"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			true,
		},
		{
			[]cty.PathValueMarks{
				cty.PathValueMarks{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
				cty.PathValueMarks{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "c"}},
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
			},
			[]cty.PathValueMarks{
				cty.PathValueMarks{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "c"}},
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
				cty.PathValueMarks{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
			},
			true,
		},
		{
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "b"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			false,
		},
		{
			nil,
			nil,
			true,
		},
		{
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			nil,
			false,
		},
		{
			nil,
			[]cty.PathValueMarks{
				cty.PathValueMarks{Path: cty.Path{cty.GetAttrStep{Name: "a"}}, Marks: cty.NewValueMarks(marks.Sensitive)},
			},
			false,
		},
	} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if marksEqual(tc.a, tc.b) != tc.equal {
				t.Fatalf("marksEqual(\n%#v,\n%#v,\n) != %t\n", tc.a, tc.b, tc.equal)
			}
		})
	}
}

func TestCombinePathValueMarks(t *testing.T) {
	paths := map[string]cty.PathValueMarks{
		"a.b": {
			Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
			Marks: cty.NewValueMarks(marks.Sensitive),
		},
		"a.c": {
			Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "c"}},
			Marks: cty.NewValueMarks(marks.Sensitive),
		},
		"[0]": {
			Path:  cty.Path{cty.IndexStep{Key: cty.NumberIntVal(0)}},
			Marks: cty.NewValueMarks("a"),
		},
		"a.b<alt>": {
			Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
			Marks: cty.NewValueMarks("a"),
		},
	}

	tests := []struct {
		name string
		LHS  []cty.PathValueMarks
		RHS  []cty.PathValueMarks
		Want []cty.PathValueMarks
	}{
		{
			name: "no marks",
			LHS:  []cty.PathValueMarks{},
			RHS:  []cty.PathValueMarks{},
			Want: []cty.PathValueMarks{},
		},
		{
			name: "one mark",
			LHS:  []cty.PathValueMarks{paths["a.b"]},
			RHS:  []cty.PathValueMarks{},
			Want: []cty.PathValueMarks{paths["a.b"]},
		},
		{
			name: "one overlapping mark",
			LHS:  []cty.PathValueMarks{paths["a.b"]},
			RHS:  []cty.PathValueMarks{paths["a.b"]},
			Want: []cty.PathValueMarks{paths["a.b"]},
		},
		{
			name: "one non-overlapping mark",
			LHS:  []cty.PathValueMarks{paths["a.b"]},
			RHS:  []cty.PathValueMarks{paths["a.c"]},
			Want: []cty.PathValueMarks{paths["a.b"], paths["a.c"]},
		},
		{
			name: "one overlapping and two non-overlapping marks",
			LHS:  []cty.PathValueMarks{paths["a.b"], paths["a.c"], paths["[0]"]},
			RHS:  []cty.PathValueMarks{paths["a.c"]},
			Want: []cty.PathValueMarks{paths["a.b"], paths["a.c"], paths["[0]"]},
		},
		{
			name: "one overlapping mark with different values",
			LHS: []cty.PathValueMarks{
				{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks(marks.Sensitive),
				},
			},
			RHS: []cty.PathValueMarks{
				{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks("OTHERMARK"),
				},
			},
			Want: []cty.PathValueMarks{
				{
					Path:  cty.Path{cty.GetAttrStep{Name: "a"}, cty.GetAttrStep{Name: "b"}},
					Marks: cty.NewValueMarks(marks.Sensitive, "OTHERMARK"),
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := combinePathValueMarks(test.LHS, test.RHS)
			if len(got) != len(test.Want) {
				t.Fatalf("incorrect result length\ngot:  %#v\nwant: %#v", got, test.Want)
			}

			for i, want := range test.Want {
				if !got[i].Equal(want) {
					t.Errorf("incorrect result\nindex: %d\ngot:  %#v\nwant: %#v", i, got[i], want)
				}
			}
		})
	}
}
