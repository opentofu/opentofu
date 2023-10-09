// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package statemgr

import (
	"fmt"

	"github.com/hashicorp/go-uuid"
)

// NewLineage generates a new lineage identifier string. A lineage identifier
// is an opaque string that is intended to be unique in space and time, chosen
// when state is recorded at a location for the first time and then preserved
// afterwards to allow OpenTofu to recognize when one state snapshot is a
// predecessor or successor of another.
func NewLineage() string {
	lineage, err := uuid.GenerateUUID()
	if err != nil {
		panic(fmt.Errorf("Failed to generate lineage: %w", err))
	}
	return lineage
}
