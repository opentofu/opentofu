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

func TestSet_intersection(t *testing.T) {
	cases := map[string]struct {
		given, second, wanted collections.Set[any]
	}{
		"string - same len": {
			given:  collections.NewSet[any]("a", "b", "c"),
			second: collections.NewSet[any]("c", "d", "e"),
			wanted: collections.NewSet[any]("c"),
		},
		"string - given is greater in size": {
			given:  collections.NewSet[any]("a", "b", "c", "d"),
			second: collections.NewSet[any]("c", "d", "e"),
			wanted: collections.NewSet[any]("c", "d"),
		},
		"string - second is greater in size": {
			given:  collections.NewSet[any]("a", "b", "c"),
			second: collections.NewSet[any]("b", "c", "d", "e"),
			wanted: collections.NewSet[any]("b", "c"),
		},
		"string - no elements in common and given is greater in size": {
			given:  collections.NewSet[any]("a", "b", "c"),
			second: collections.NewSet[any]("d", "e"),
			wanted: collections.NewSet[any](),
		},
		"string - no elements in common and second is greater in size": {
			given:  collections.NewSet[any]("a", "b"),
			second: collections.NewSet[any]("c", "d", "e"),
			wanted: collections.NewSet[any](),
		},
		"int - same len": {
			given:  collections.NewSet[any](1, 2, 3),
			second: collections.NewSet[any](3, 4, 5),
			wanted: collections.NewSet[any](3),
		},
		"int - given is greater in size": {
			given:  collections.NewSet[any](1, 2, 3, 4),
			second: collections.NewSet[any](3, 4, 5),
			wanted: collections.NewSet[any](3, 4),
		},
		"int - second is greater in size": {
			given:  collections.NewSet[any](1, 2, 3),
			second: collections.NewSet[any](3, 4, 5, 6),
			wanted: collections.NewSet[any](3),
		},
		"int - no elements in common and given is greater in size": {
			given:  collections.NewSet[any](1, 2, 3),
			second: collections.NewSet[any](4, 5),
			wanted: collections.NewSet[any](),
		},
		"int - no elements in common and second is greater in size": {
			given:  collections.NewSet[any](1, 2),
			second: collections.NewSet[any](3, 4, 5),
			wanted: collections.NewSet[any](),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			givenString := tc.given.String()
			secondString := tc.second.String()
			wantedString := tc.wanted.String()

			got := tc.given.Intersection(tc.second)

			gotString := got.String()
			givenAfterString := tc.given.String()
			secondAfterString := tc.second.String()

			if wantedString != gotString {
				t.Errorf("unexpected returned result: %s; wanted: %s", gotString, wantedString)
			}
			if givenString != givenAfterString {
				t.Errorf("given shouldn't be changed during the operation but seems that it was. initial: %s; after: %s", givenString, givenAfterString)
			}
			if secondString != secondAfterString {
				t.Errorf("second shouldn't be changed during the operation but seems that it was. initial: %s; after: %s", secondString, secondAfterString)
			}
		})
	}
}
