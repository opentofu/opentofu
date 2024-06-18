// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils_test

import (
	"strings"
	"testing"

	"github.com/opentofu/opentofu/internal/testutils"
)

func TestDeterministicRandomID(t *testing.T) {
	const idLength = 12
	if t.Name() != "TestDeterministicRandomID" {
		t.Fatalf(
			"The test name has changed, please update the test as it is used for seeding the random number " +
				"generator.",
		)
	}
	if id := testutils.DeterministicRandomID(
		t,
		idLength,
		testutils.CharacterRangeAlphaNumeric,
	); id != "MFXow4tIaSnd" {
		t.Fatalf(
			"Incorrect first pseudo-random ID returned: %s (the returned ID depends on the test name, make "+
				"sure to verify and update if you changed the test name)",
			id,
		)
	}
	if id := testutils.DeterministicRandomID(
		t,
		idLength,
		testutils.CharacterRangeAlphaNumeric,
	); id != "9LSAyPisw01j" {
		t.Fatalf(
			"Incorrect second pseudo-random ID returned: %s (the returned ID depends on the test name, make "+
				"sure to verify and update if you changed the test name)",
			id,
		)
	}
}

func TestRandomIDPrefix(t *testing.T) {
	const testPrefix = "test-"
	const idLength = 12
	id := testutils.RandomIDPrefix(testPrefix, idLength, testutils.CharacterRangeAlphaNumeric)
	if len(id) != idLength+len(testPrefix) {
		t.Fatalf("Incorrect random ID length: %s", id)
	}
	if !strings.HasPrefix(id, testPrefix) {
		t.Fatalf("Missing prefix: %s", id)
	}
}

func TestRandomID(t *testing.T) {
	const idLength = 12
	id := testutils.RandomID(idLength, testutils.CharacterRangeAlphaNumeric)
	if len(id) != idLength {
		t.Fatalf("Incorrect random ID length: %s", id)
	}
}

func TestDeterministicRandomInt(t *testing.T) {
	if t.Name() != "TestDeterministicRandomInt" {
		t.Fatalf(
			"The test name has changed, please update the test as it is used for seeding the random number " +
				"generator.",
		)
	}
	if i := testutils.DeterministicRandomInt(
		t,
		1,
		42,
	); i != 31 {
		t.Fatalf(
			"Incorrect first pseudo-random int returned: %d (the returned int depends on the test name, make "+
				"sure to verify and update if you changed the test name)",
			i,
		)
	}
	if i := testutils.DeterministicRandomInt(
		t,
		1,
		42,
	); i != 29 {
		t.Fatalf(
			"Incorrect second pseudo-random int returned: %d (the returned int depends on the test name, make "+
				"sure to verify and update if you changed the test name)",
			i,
		)
	}
}

func TestRandomInt(t *testing.T) {
	i := testutils.RandomInt(1, 42)
	if i < 1 || i > 42 {
		t.Fatalf("Invalid random integer returned %d (out of range)", i)
	}
}
