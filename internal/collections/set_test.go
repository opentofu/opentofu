package collections_test

import (
	"github.com/opentofu/opentofu/internal/collections"
	"strings"
	"testing"
)

type hasTestCase struct {
	name             string
	set              collections.Set[string]
	testValueResults map[string]bool
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

	if str := testSet.String(); !strings.Contains(str, "a, b, c") {
		t.Fatalf("Incorrect string concatenation: %s", str)
	}
}
