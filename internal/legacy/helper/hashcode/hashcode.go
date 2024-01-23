// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package hashcode

import (
	"bytes"
	"fmt"
	"hash/crc32"
)

// String hashes a string to a unique hashcode.
// Returns a non-negative integer representing the hashcode of the string.
func String(s string) int {
	// crc32 returns an uint32, so we need to massage it into an int.
	crc := crc32.ChecksumIEEE([]byte(s))
	// We need to first squash the result to 32 bits, embracing the overflow
	// to ensure that there is no difference between 32 and 64-bit
	// platforms.
	squashed := int32(crc)
	// convert into a generic int that is sized as per the architecture
	systemSized := int(squashed)

	// If the integer is negative, we return the absolute value of the
	// integer. This is because we want to return a non-negative integer
	if systemSized >= 0 {
		return systemSized
	}
	if -systemSized >= 0 {
		return -systemSized
	}
	// systemSized == MinInt
	return 0
}

// Strings hashes a list of strings to a unique hashcode.
func Strings(strings []string) string {
	var buf bytes.Buffer

	for _, s := range strings {
		buf.WriteString(fmt.Sprintf("%s-", s))
	}

	return fmt.Sprintf("%d", String(buf.String()))
}
