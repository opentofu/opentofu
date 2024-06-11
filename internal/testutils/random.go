// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package testutils

import (
	"math/rand"
	"strings"
	"time"
)

// PseudoRandomID generates a random, ASCII-character identifier of the given length. This function is for test purposes
// only and should not be used for real identifiers as they are not guaranteed to be truly random or globally unique.
func PseudoRandomID(length uint) string {
	random := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec // Disabling gosec linting because this ID is for testing only.
	runes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	var builder strings.Builder
	for i := uint(0); i < length; i++ {
		builder.WriteRune(runes[random.Intn(len(runes))])
	}
	return builder.String()
}
