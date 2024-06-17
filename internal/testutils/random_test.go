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
		testutils.CharacterSpaceAlphaNumeric,
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
		testutils.CharacterSpaceAlphaNumeric,
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
	id := testutils.RandomIDPrefix(testPrefix, idLength, testutils.CharacterSpaceAlphaNumeric)
	if len(id) != idLength+len(testPrefix) {
		t.Fatalf("Incorrect random ID length: %s", id)
	}
	if !strings.HasPrefix(id, testPrefix) {
		t.Fatalf("Missing prefix: %s", id)
	}
}

func TestRandomID(t *testing.T) {
	const idLength = 12
	id := testutils.RandomID(idLength, testutils.CharacterSpaceAlphaNumeric)
	if len(id) != idLength {
		t.Fatalf("Incorrect random ID length: %s", id)
	}
}
