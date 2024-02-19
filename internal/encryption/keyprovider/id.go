// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

// ID is a type alias to make passing the wrong ID into a key provider harder.
type ID string

// Validate validates the key provider ID for correctness.
func (i ID) Validate() error {
	// TODO implement format checking
	return nil
}
