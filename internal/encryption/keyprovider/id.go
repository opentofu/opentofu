// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package keyprovider

import "fmt"

// ID is a type alias to make passing the wrong ID into a key provider harder.
type ID string

// Validate validates the key provider ID for correctness.
func (id ID) Validate() error {
	if id == "" {
		return fmt.Errorf("empty key provider ID (key provider IDs must match %s)", idRe.String())
	}
	if !idRe.MatchString(string(id)) {
		return fmt.Errorf("invalid key provider ID: %s (must match %s)", id, idRe.String())
	}
	return nil
}
