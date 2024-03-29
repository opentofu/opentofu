// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package pbkdf2

import "testing"

// testRandomSource is a predictable reader that outputs the test name as a source of randomness.
type testRandomSource struct {
	t *testing.T
}

func (t testRandomSource) Read(target []byte) (int, error) {
	name := t.t.Name()
	for i := 0; i < len(target); i++ {
		target[i] = name[i%len(name)]
	}
	return len(target), nil
}
