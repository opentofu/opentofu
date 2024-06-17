// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"hash/crc64"
	"math/rand"
	"strings"
	"testing"
	"time"
)

var randomSources = map[string]*rand.Rand{}

type CharacterSpace string

const (
	CharacterSpaceAlphaNumericUpperLower CharacterSpace = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	CharacterSpaceAlphaNumericLower      CharacterSpace = "abcdefghijklmnopqrstuvwxyz0123456789"
	CharacterSpaceAlphaNumericUpper      CharacterSpace = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	CharacterSpaceAlphaUpperLower        CharacterSpace = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	CharacterSpaceAlphaLower             CharacterSpace = "abcdefghijklmnopqrstuvwxyz"
	CharacterSpaceAlphaUpper             CharacterSpace = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// DeterministicRandomID generates a pseudo-random identifier for the given test, using its name as a seed for
// randomness. This function guarantees that when queried in order, the values are always the same as long as the name
// of the test doesn't change.
func DeterministicRandomID(t *testing.T, length uint, characterSpace CharacterSpace) string {
	var random *rand.Rand
	name := t.Name()
	var ok bool
	random, ok = randomSources[name]
	if !ok {
		seed := crc64.Checksum([]byte(name), crc64.MakeTable(crc64.ECMA))
		random = rand.New(rand.NewSource(int64(seed)))
		randomSources[name] = random
		t.Cleanup(func() {
			delete(randomSources, name)
		})
	}
	return RandomIDFromSource(random, length, characterSpace)
}

// RandomID returns a non-deterministic, pseudo-random identifier.
func RandomID(length uint, characterSpace CharacterSpace) string {
	return RandomIDFromSource(rand.New(rand.NewSource(time.Now().UnixNano())), length, characterSpace) //nolint:gosec // Disabling gosec linting because this ID is for testing only.
}

// RandomIDPrefix returns a random identifier with a given prefix. The prefix length does not count towards the
// length.
func RandomIDPrefix(prefix string, length uint, characterSpace CharacterSpace) string {
	return prefix + RandomID(length, characterSpace)
}

// RandomIDFromSource generates a random ID with the specified length based on the provided random parameter.
func RandomIDFromSource(random *rand.Rand, length uint, characterSpace CharacterSpace) string {
	runes := []rune(characterSpace)
	var builder strings.Builder
	for i := uint(0); i < length; i++ {
		builder.WriteRune(runes[random.Intn(len(runes))])
	}
	return builder.String()
}
