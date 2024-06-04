// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package collections_test

import (
	"testing"

	"github.com/opentofu/opentofu/internal/collections"
)

type hasTestCase struct {
	name             string
	set              collections.Set[string]
	testValueResults map[string]bool
}

func TestSet_NewSet(t *testing.T) {
	testCases := []struct {
		name        string
		constructed collections.Set[int]
		expected    collections.Set[int]
	}{
		{
			name:        "empty",
			constructed: collections.NewSet[int](),
			expected:    collections.Set[int]{},
		}, {
			name:        "items",
			constructed: collections.NewSet[int](1, 54, 284),
			expected:    collections.Set[int]{1: {}, 54: {}, 284: {}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.constructed) != len(tc.expected) {
				t.Fatal("Set length mismatch")
			}

			for k := range tc.expected {
				if _, ok := tc.constructed[k]; !ok {
					t.Fatalf("Expected to find key %v in constructed set", k)
				}
			}
		})
	}
}

func TestSet_has(t *testing.T) {
	testCases := []hasTestCase{
		{
			name: "string",
			set: collections.Set[string]{
				"a": {},
				"b": {},
				"c": {},
			},
			testValueResults: map[string]bool{
				"a": true,
				"b": true,
				"c": true,
				"d": false,
				"e": false,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			for value, has := range testCase.testValueResults {
				t.Run(value, func(t *testing.T) {
					if has {
						if !testCase.set.Has(value) {
							t.Fatalf("Set does not have expected value of %s", value)
						}
					} else {
						if testCase.set.Has(value) {
							t.Fatalf("Set has unexpected value of %s", value)
						}
					}
				})
			}
		})
	}
}

func TestSet_string(t *testing.T) {
	testSet := collections.Set[string]{
		"a": {},
		"b": {},
		"c": {},
	}

	if str := testSet.String(); str != "a, b, c" {
		t.Fatalf("Incorrect string concatenation: %s", str)
	}
}
