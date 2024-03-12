// Copyright (c) The OpenTofu Authors
// SPDX-License-Identifier: MPL-2.0
// Copyright (c) 2023 HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package method

import (
	"fmt"
)

// ID is a type alias to make passing the wrong ID into a method ID harder.
type ID string

// Validate validates the key provider ID for correctness.
func (i ID) Validate() error {
	if i == "" {
		return fmt.Errorf("empty key provider ID (key provider IDs must match %s)", idRe.String())
	}
	if !idRe.MatchString(string(i)) {
		return fmt.Errorf("invalid key provider ID: %s (must match %s)", i, idRe.String())
	}
	return nil
}
